package cache_test

import (
	"path/filepath"
	"testing"

	"github.com/ezcdlabs/clarity/internal/cache"
)

// TestFile_Read_Missing returns (nil, false, nil). The "doesn't exist
// yet" branch must never error — first-run callers rely on this to
// distinguish "no cache" from "broken cache".
func TestFile_Read_Missing(t *testing.T) {
	f := cache.New(filepath.Join(t.TempDir(), "missing.bin.gz"))
	data, exists, err := f.Read()
	if err != nil {
		t.Fatalf("expected no error on missing file, got %v", err)
	}
	if exists {
		t.Errorf("expected exists=false, got true")
	}
	if data != nil {
		t.Errorf("expected nil data, got %d bytes", len(data))
	}
}

// TestFile_RoundTrip is the load-bearing happy-path: Write some bytes,
// Read them back, get the same bytes. Exercises the gzip round-trip end
// to end (callers see plain bytes; the on-disk layer compresses).
func TestFile_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "round-trip.bin.gz")
	f := cache.New(path)
	want := []byte(`{"hello":"world"}`)

	if err := f.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, exists, err := f.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !exists {
		t.Fatal("expected exists=true after Write")
	}
	if string(got) != string(want) {
		t.Errorf("round-trip mismatch:\n  want: %s\n  got:  %s", want, got)
	}
}

// TestFile_Write_IsAtomic protects against partial writes on crash:
// Write must materialise the final file in one rename, never leaving
// behind a half-written byte stream that a concurrent Read would see.
// We assert this indirectly by writing twice and confirming the second
// write fully replaces the first (no leftover bytes from the longer
// first payload).
func TestFile_Write_IsAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atomic.bin.gz")
	f := cache.New(path)

	if err := f.Write([]byte("longer-original-payload")); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	if err := f.Write([]byte("short")); err != nil {
		t.Fatalf("second Write: %v", err)
	}

	got, _, err := f.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != "short" {
		t.Errorf("expected second write to fully replace first, got %q", got)
	}
}

// TestFile_Invalidate removes the file; subsequent Read reports missing.
// Useful for hand-clearing a corrupted cache via the helper rather than
// shelling out.
func TestFile_Invalidate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalidate.bin.gz")
	f := cache.New(path)
	if err := f.Write([]byte("data")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := f.Invalidate(); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}
	_, exists, err := f.Read()
	if err != nil {
		t.Fatalf("Read after Invalidate: %v", err)
	}
	if exists {
		t.Errorf("expected exists=false after Invalidate")
	}
}

// TestFile_Invalidate_Missing is a no-op — Invalidate must be safe to
// call when there's nothing to invalidate. Callers that "clear on
// startup if config changed" shouldn't have to first check existence.
func TestFile_Invalidate_Missing(t *testing.T) {
	f := cache.New(filepath.Join(t.TempDir(), "missing.bin.gz"))
	if err := f.Invalidate(); err != nil {
		t.Errorf("expected no error invalidating a missing file, got %v", err)
	}
}
