package core_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/internal/cache"
	"github.com/ezcdlabs/clarity/internal/core"
)

// fakeViewProducer is a stand-in for *core.Lens in CachedLens tests. It
// exposes a programmable channel so the test can decide when (and what)
// the "fresh" path emits — keeping the cached half deterministic.
type fakeViewProducer struct {
	out chan core.View
}

func newFakeViewProducer() *fakeViewProducer {
	return &fakeViewProducer{out: make(chan core.View, 4)}
}

func (f *fakeViewProducer) Views(ctx context.Context) <-chan core.View {
	go func() {
		<-ctx.Done()
		// Drain-then-close so a producer that already wrote into the buffer
		// before cancellation still gets seen by the consumer.
		select {
		case <-f.out:
		default:
		}
		close(f.out)
	}()
	return f.out
}

func (f *fakeViewProducer) emit(v core.View) { f.out <- v }

// TestCachedLens_NoCache_EmitsFreshOnly: with no cache file on disk,
// CachedLens has nothing to short-circuit on so it must immediately
// forward the inner producer's first View — Stale=false — as the only
// emission. Anything else (a synthetic empty stale view, a silent
// dropped emit) would either mislead the renderer or stall startup.
func TestCachedLens_NoCache_EmitsFreshOnly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cf := cache.New(filepath.Join(t.TempDir(), "snapshot.json.gz"))
	inner := newFakeViewProducer()
	cl := core.NewCachedLens(inner, cf, core.DefaultLeadTimeMode)
	out := cl.Views(ctx)

	go inner.emit(core.View{Snapshot: core.Snapshot{RepoName: "fresh"}})

	got := readView(t, out)
	if got.Stale {
		t.Errorf("expected first emission to be fresh (Stale=false), got Stale=true")
	}
	if got.Snapshot.RepoName != "fresh" {
		t.Errorf("expected fresh snapshot, got %+v", got.Snapshot)
	}
}

// TestCachedLens_WithCache_EmitsStaleFirstThenFresh is the load-bearing
// SWR test: a populated cache file produces a Stale view immediately,
// and the next fresh emission from the inner producer follows with
// Stale=false. Order matters — the renderer relies on the stale view
// being the first paint.
func TestCachedLens_WithCache_EmitsStaleFirstThenFresh(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cf := cache.New(filepath.Join(t.TempDir(), "snapshot.json.gz"))
	// Pre-seed the cache with a Snapshot — that's exactly what
	// CachedLens.Write does on every fresh emission, so seeding directly
	// emulates a previous run.
	cached := core.Snapshot{RepoName: "cached"}
	data, err := json.Marshal(cached)
	if err != nil {
		t.Fatalf("marshal cached snapshot: %v", err)
	}
	if err := cf.Write(data); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	inner := newFakeViewProducer()
	cl := core.NewCachedLens(inner, cf, core.DefaultLeadTimeMode)
	out := cl.Views(ctx)

	first := readView(t, out)
	if !first.Stale {
		t.Errorf("expected first emission to be Stale=true (from cache), got Stale=false")
	}
	if first.Snapshot.RepoName != "cached" {
		t.Errorf("expected cached snapshot, got %+v", first.Snapshot)
	}

	go inner.emit(core.View{Snapshot: core.Snapshot{RepoName: "fresh"}})

	second := readView(t, out)
	if second.Stale {
		t.Errorf("expected second emission to be Stale=false, got Stale=true")
	}
	if second.Snapshot.RepoName != "fresh" {
		t.Errorf("expected fresh snapshot, got %+v", second.Snapshot)
	}
}

// TestCachedLens_WritesFreshToCache: each fresh View's Snapshot must be
// persisted so the next process startup has a warm cache. Verified by
// reading the cache file back after the consumer has seen one fresh
// emission.
func TestCachedLens_WritesFreshToCache(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cf := cache.New(filepath.Join(t.TempDir(), "snapshot.json.gz"))
	inner := newFakeViewProducer()
	cl := core.NewCachedLens(inner, cf, core.DefaultLeadTimeMode)
	out := cl.Views(ctx)

	go inner.emit(core.View{Snapshot: core.Snapshot{RepoName: "fresh"}})
	readView(t, out) // wait for the fresh emission to be observed downstream

	data, exists, err := cf.Read()
	if err != nil {
		t.Fatalf("Read cache: %v", err)
	}
	if !exists {
		t.Fatal("expected cache file to exist after fresh emission")
	}
	var got core.Snapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal cache: %v", err)
	}
	if got.RepoName != "fresh" {
		t.Errorf("expected cache to contain fresh snapshot, got %+v", got)
	}
}

func readView(t *testing.T, ch <-chan core.View) core.View {
	t.Helper()
	select {
	case v, ok := <-ch:
		if !ok {
			t.Fatal("channel closed unexpectedly while waiting for a view")
		}
		return v
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for view")
		return core.View{}
	}
}
