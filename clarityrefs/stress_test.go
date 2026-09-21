package clarityrefs_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// TestReadAllEvents_LargeRef guards the subprocess read path at a size where
// its failure modes actually appear.
//
// `git cat-file --batch` is fed every hash on stdin while its stdout is read
// concurrently. If those were serialised — write it all, then read — git would
// block writing into a full stdout pipe while clarity was still blocking on
// stdin, and the whole read would deadlock. A handful of events fits in the
// pipe buffer and proves nothing; a few thousand does not.
func TestReadAllEvents_LargeRef(t *testing.T) {
	if testing.Short() {
		t.Skip("large ref stress test")
	}
	const n = 3000

	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	batch := map[string][]clarityrefs.Event{}
	for i := 0; i < n; i++ {
		sha := fmt.Sprintf("%040x", i)
		batch[sha] = []clarityrefs.Event{{
			Stage:  "ci",
			Status: "passed",
			Time:   time.Unix(1744120134+int64(i), 0),
			CI: map[string]string{
				// Padding, so blobs are not all a couple of hundred bytes and
				// the stream has to be framed correctly across many reads.
				"system": "github-actions",
				"run_id": fmt.Sprintf("%0256d", i),
			},
		}}
	}
	if err := clarityrefs.WriteEvents(clone.Path, "origin", batch); err != nil {
		t.Fatalf("WriteEvents: %v", err)
	}

	if err := fetchEventsRef(clone.Path); err != nil {
		t.Fatalf("fetch: %v", err)
	}

	done := make(chan struct{})
	var got map[string][]clarityrefs.Event
	var readErr error
	start := time.Now()
	go func() {
		defer close(done)
		got, readErr = clarityrefs.ReadAllEvents(clone.Path)
	}()

	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("ReadAllEvents did not return: the cat-file pipe deadlocked")
	}

	t.Logf("read %d events in %s", n, time.Since(start).Round(time.Millisecond))
	if readErr != nil {
		t.Fatalf("ReadAllEvents: %v", readErr)
	}
	if len(got) != n {
		t.Fatalf("expected %d commits with events, got %d", n, len(got))
	}
	// Spot-check that framing did not slip by one object somewhere in the
	// middle: a misframed stream yields plausible-looking garbage, not an error.
	mid := fmt.Sprintf("%040x", n/2)
	if evs := got[mid]; len(evs) != 1 || evs[0].CI["run_id"] != fmt.Sprintf("%0256d", n/2) {
		t.Fatalf("content mismatch at %s: %+v", mid, evs)
	}
}

// TestReadAllEvents_EmptyAndAwkwardBlobs covers the framing edges: a zero-byte
// object, and content whose bytes could be mistaken for the batch protocol's
// own framing.
func TestReadAllEvents_EmptyAndAwkwardBlobs(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	tricky := clarityrefs.Event{
		Stage:  "ci",
		Status: "passed",
		Time:   time.Unix(1744120134, 0),
		CI: map[string]string{
			"newlines": "line\nline\n",
			"nul_like": "0000000000000000000000000000000000000000 blob 12",
			"unicode":  "✓ ünïcøde 日本語",
		},
	}
	if err := clarityrefs.WriteEvent(clone.Path, "origin", fakeSHA, tricky); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}

	if err := fetchEventsRef(clone.Path); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	got, err := clarityrefs.ReadEvents(clone.Path, fakeSHA)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	for k, want := range tricky.CI {
		if got[0].CI[k] != want {
			t.Errorf("CI[%q] = %q, want %q", k, got[0].CI[k], want)
		}
	}
}
