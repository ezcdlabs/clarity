package clarityrefs_test

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// GIT_NO_LAZY_FETCH=1 tells git not to reach out to a promisor remote for an
// object it does not hold. CI setups set it to keep builds off the network
// mid-run, and clarity inherits it like any other variable.
//
// It is what makes --no-filter load-bearing rather than merely efficient.
// Reading through git means a lazily-held blob would normally be fetched on
// access, so with lazy fetching available the flag only saves round trips —
// but with it refused, the blobs have to already be present, and the only
// thing that puts them there is clarity asking for them when it fetches.

// TestWriteEvent_BloblessCloneWithoutLazyFetching pins that behaviour rather
// than pinning the flag: it asserts the outcome --no-filter exists to
// produce, so removing the flag fails here and not only in the shim test
// that checks the argument is sent.
func TestWriteEvent_BloblessCloneWithoutLazyFetching(t *testing.T) {
	remote := gittest.NewRemote(t)
	seed := remote.NewClone(t)
	first := clarityrefs.Event{Stage: "ci", Status: "started", Time: time.Unix(1744120134, 0)}
	if err := clarityrefs.WriteEvent(seed.Path, "origin", fakeSHA, first); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	partial := remote.NewBloblessClone(t)

	// Set after the clone: git refuses to make a partial clone at all when
	// lazy fetching is disabled, so the variable has to arrive once the
	// repository exists — which is also how a runner presents it.
	t.Setenv("GIT_NO_LAZY_FETCH", "1")

	second := clarityrefs.Event{Stage: "ci", Status: "passed", Time: time.Unix(1744120200, 0)}
	if err := clarityrefs.WriteEvent(partial.Path, "origin", fakeSHA, second); err != nil {
		t.Fatalf("WriteEvent in a blobless clone with lazy fetching refused: %v\n"+
			"the fetch has to bring the blobs itself; nothing else can", err)
	}
}

// TestReadEvents_ReportsGitsOwnDiagnosis covers the error path that state
// produces when the blobs were *not* asked for up front. The read cannot
// recover, so what it says has to be useful.
//
// git answers in one of two shapes depending on its version, and both are
// legitimate: it either reports the object as "missing" — which clarity
// turns into a message naming the object and the ref — or ends the batch
// stream mid-object and explains itself on stderr, where "lazy fetching
// disabled" is the line that matters. The assertion is that the failure
// identifies itself either way, rather than surfacing as a bare parse error
// like "object 1 of 1: EOF".
func TestReadEvents_ReportsGitsOwnDiagnosis(t *testing.T) {
	remote := gittest.NewRemote(t)
	seed := remote.NewClone(t)
	ev := clarityrefs.Event{Stage: "ci", Status: "passed", Time: time.Unix(1744120134, 0)}
	if err := clarityrefs.WriteEvent(seed.Path, "origin", fakeSHA, ev); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	partial := remote.NewBloblessClone(t)
	// Fetch the way a filter-inheriting fetch would: ref and trees, no blobs.
	cmd := exec.Command("git", "fetch", "origin",
		"+"+clarityrefs.EventsRef+":"+clarityrefs.EventsRef)
	cmd.Dir = partial.Path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("filtered fetch: %v\n%s", err, out)
	}

	t.Setenv("GIT_NO_LAZY_FETCH", "1")

	_, err := clarityrefs.ReadAllEvents(partial.Path)
	if err == nil {
		t.Fatal("expected a read failure when the blobs are absent and cannot be fetched")
	}
	msg := err.Error()
	identifies := strings.Contains(msg, clarityrefs.EventsRef) ||
		strings.Contains(msg, "lazy fetching")
	if !identifies {
		t.Errorf("error should say what could not be read, naming the ref or "+
			"carrying git's own explanation; got: %v", err)
	}
}
