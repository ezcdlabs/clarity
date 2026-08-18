package clarityrefs

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/internal/gittest"
)

// remoteRefLockRejection is the verbatim push failure from a GitHub Actions
// deploy job, reproduced character-for-character so the test is driven by the
// real wording rather than a guess at it.
const remoteRefLockRejection = "exit status 1\n" +
	"To https://github.com/Rafiki-Works/app-v2\n" +
	" ! [remote rejected] refs/clarity/events -> refs/clarity/events " +
	"(cannot lock ref 'refs/clarity/events': is at a9f4824bf9165cde14f8ffd67bfd1c7129321039 " +
	"but expected 1668810d1e2f4448d940486d2718decdeeece9b4)\n" +
	"error: failed to push some refs to 'https://github.com/Rafiki-Works/app-v2'"

// TestWriteEvent_RetriesWhenRemoteRefusesTheRefLock is the end-to-end
// regression test for a dropped deploy event reported from CI.
//
// Two jobs pushed refs/clarity/events at the same instant. Neither client
// could tell it was behind — both had fetched the same tip — so the loser's
// compare-and-swap was refused by the server rather than declined locally,
// and the rejection came back worded as a lock failure. The retry loop only
// recognised the client-side wording ("[rejected]"), so a routine lost race
// was returned as a hard error and the event was never recorded.
//
// The race is injected at the push seam, because provoking a server-side lock
// failure on demand means winning a sub-millisecond window on a real
// forge. What is NOT faked is the recovery: the winner's event is pushed for
// real inside the seam, so the loser genuinely comes back to a moved ref and
// has to re-fetch and replay its commit on top. Asserting both events survive
// is what distinguishes a real retry from a swallowed error.
func TestWriteEvent_RetriesWhenRemoteRefusesTheRefLock(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef01234567"

	remote := gittest.NewRemote(t)
	loser := remote.NewClone(t)
	winner := remote.NewClone(t)

	// Seed the ref so both writers are updating an existing ref — the ref
	// lock can only be refused for a ref that is already there.
	seed := Event{Stage: "ci", Status: "passed", Time: time.Unix(1744120000, 0)}
	if err := WriteEvent(winner.Path, "origin", sha, seed); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	if err := fetchEventsRef(loser.Path, "origin"); err != nil {
		t.Fatalf("loser fetch: %v", err)
	}

	original := pushEvents
	t.Cleanup(func() { pushEvents = original })

	var raced atomic.Bool
	pushEvents = func(repoPath, rem string) error {
		// Only the loser's first push loses the race; every push after it,
		// including the winner's nested one below, goes through for real.
		if !raced.CompareAndSwap(false, true) {
			return original(repoPath, rem)
		}
		won := Event{Stage: "deploy", Status: "started", Time: time.Unix(1744120100, 0)}
		if err := WriteEvent(winner.Path, "origin", sha, won); err != nil {
			t.Errorf("winner write: %v", err)
		}
		return fmt.Errorf("%s", remoteRefLockRejection)
	}

	lost := Event{Stage: "deploy", Status: "passed", Time: time.Unix(1744120200, 0)}
	if err := WriteEvent(loser.Path, "origin", sha, lost); err != nil {
		t.Fatalf("a lost push race must be retried, not returned: %v", err)
	}

	reader := remote.NewClone(t)
	if err := fetchEventsRef(reader.Path, "origin"); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	got, err := ReadEvents(reader.Path, sha)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("all three events must survive the race, got %d: %+v", len(got), got)
	}
	// The loser must have replayed onto the winner's commit, not clobbered it.
	for _, want := range []Event{seed, {Stage: "deploy", Status: "started"}, {Stage: "deploy", Status: "passed"}} {
		found := false
		for _, g := range got {
			if g.Stage == want.Stage && g.Status == want.Status {
				found = true
			}
		}
		if !found {
			t.Errorf("event %s/%s was lost", want.Stage, want.Status)
		}
	}
}
