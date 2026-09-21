package clarityrefs_test

import (
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// A working copy can share its objects with a mirror elsewhere on disk
// through .git/objects/info/alternates. CI caches are built this way, and
// Blacksmith's checkout leaves the workspace attached to its mirror unless
// explicitly told to dissociate — which it is not by default.
//
// Every object is readable by git and none of them live in the workspace, so
// anything resolving objects out of .git/objects directly sees an empty
// store. That surfaced identically to the partial-clone failure — "object not
// found" against a ref that was present — which is why reads go through git
// rather than chasing one clone shape at a time.

// TestWriteEvent_UnderSharedObjectStore is the regression test for reporting
// from a workspace attached to a mirror.
//
// The pre-existing event is what makes it bite: the write has to read the
// current ref back before appending, and that read is what failed.
func TestWriteEvent_UnderSharedObjectStore(t *testing.T) {
	remote := gittest.NewRemote(t)

	seed := remote.NewClone(t)
	first := clarityrefs.Event{Stage: "ci", Status: "started", Time: time.Unix(1744120134, 0)}
	if err := clarityrefs.WriteEvent(seed.Path, "origin", fakeSHA, first); err != nil {
		t.Fatalf("seeding the remote events ref failed: %v", err)
	}

	shared := remote.NewSharedObjectClone(t)
	second := clarityrefs.Event{Stage: "ci", Status: "passed", Time: time.Unix(1744120200, 0)}
	if err := clarityrefs.WriteEvent(shared.Path, "origin", fakeSHA, second); err != nil {
		t.Fatalf("WriteEvent from a shared object store failed: %v", err)
	}

	verify := remote.NewClone(t)
	if err := fetchEventsRef(verify.Path); err != nil {
		t.Fatalf("fetch for verification failed: %v", err)
	}
	events, err := clarityrefs.ReadEvents(verify.Path, fakeSHA)
	if err != nil {
		t.Fatalf("ReadEvents failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected both events on the remote, got %d: %+v", len(events), events)
	}
	if events[0].Status != "started" || events[1].Status != "passed" {
		t.Errorf("expected started then passed, got %q then %q", events[0].Status, events[1].Status)
	}
}

// TestReadAllEvents_UnderSharedObjectStore covers the read side, used by the
// TUI and --plain.
func TestReadAllEvents_UnderSharedObjectStore(t *testing.T) {
	remote := gittest.NewRemote(t)

	seed := remote.NewClone(t)
	ev := clarityrefs.Event{Stage: "deploy", Status: "passed", Time: time.Unix(1744120134, 0)}
	if err := clarityrefs.WriteEvent(seed.Path, "origin", fakeSHA, ev); err != nil {
		t.Fatalf("seeding failed: %v", err)
	}

	shared := remote.NewSharedObjectClone(t)
	if err := clarityrefs.FetchEventsRef(shared.Path, "origin"); err != nil {
		t.Fatalf("FetchEventsRef failed: %v", err)
	}
	all, err := clarityrefs.ReadAllEvents(shared.Path)
	if err != nil {
		t.Fatalf("ReadAllEvents in a shared object store failed: %v", err)
	}
	if len(all[fakeSHA]) != 1 {
		t.Fatalf("expected 1 event for %s, got %d", fakeSHA, len(all[fakeSHA]))
	}
}
