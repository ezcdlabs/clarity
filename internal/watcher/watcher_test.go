package watcher_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/clock"
	"github.com/ezcdlabs/clarity/internal/gittest"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

const testInterval = 5 * time.Second

// waitForSnapshot reads one snapshot off ch with a generous timeout.
func waitForSnapshot(t *testing.T, ch <-chan watcher.Snapshot) watcher.Snapshot {
	t.Helper()
	select {
	case snap := <-ch:
		return snap
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for snapshot")
		return watcher.Snapshot{}
	}
}

// waitForTimer blocks until the watcher has registered another fake timer
// (i.e. has gone back to sleep on Clock.After).
func waitForTimer(t *testing.T, fake *clock.Fake) {
	t.Helper()
	select {
	case <-fake.TimerAdded():
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for fake.TimerAdded()")
	}
}

func headSHA(t *testing.T, repoPath string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func startWatcher(t *testing.T, repoPath string, fake *clock.Fake) (<-chan watcher.Snapshot, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch := watcher.Watch(ctx, watcher.Options{
		RepoPath: repoPath,
		Remote:   "origin",
		Branch:   "main",
		Interval: testInterval,
		Limit:    50,
		Clock:    fake,
	})
	return ch, cancel
}

func TestWatch_EmitsInitialSnapshot(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	fake := clock.NewFake()
	ch, _ := startWatcher(t, clone.Path, fake)

	snap := waitForSnapshot(t, ch)
	if len(snap.Commits) == 0 {
		t.Fatal("expected at least the seed commit in the initial snapshot")
	}
}

func TestWatch_BranchChange_EmitsNewSnapshot(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	other := remote.NewClone(t)

	fake := clock.NewFake()
	ch, _ := startWatcher(t, clone.Path, fake)

	first := waitForSnapshot(t, ch)
	firstTop := first.Commits[0].SHA

	waitForTimer(t, fake)

	// Push a new commit from another clone — this is what changes origin/main.
	other.WriteFile("new.txt", "new")
	other.CommitAll("new commit on main")
	other.Push("main")

	fake.Advance(testInterval)

	second := waitForSnapshot(t, ch)
	if len(second.Commits) == 0 {
		t.Fatal("expected commits in second snapshot")
	}
	if second.Commits[0].SHA == firstTop {
		t.Fatalf("expected branch tip to advance, but second top is still %s", firstTop)
	}
	if second.Commits[0].Subject != "new commit on main" {
		t.Errorf("expected new commit at top, got %q", second.Commits[0].Subject)
	}
}

func TestWatch_EventsRefChange_EmitsNewSnapshot(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "a")
	clone.CommitAll("a commit")
	clone.Push("main")
	committedSHA := headSHA(t, clone.Path)

	fake := clock.NewFake()
	ch, _ := startWatcher(t, clone.Path, fake)

	waitForSnapshot(t, ch)
	waitForTimer(t, fake)

	// Write an event to the remote events ref — branch tip is unchanged.
	ev := clarityrefs.Event{Stage: "build", Status: "passed", Time: time.Unix(1744120134, 0)}
	if err := clarityrefs.WriteEvent(clone.Path, "origin", committedSHA, ev); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}

	fake.Advance(testInterval)

	snap := waitForSnapshot(t, ch)
	var found bool
	for _, c := range snap.Commits {
		if c.SHA == committedSHA && len(c.Events) >= 1 && c.Events[0].Stage == "build" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected the new event to appear on the committed SHA in the next snapshot:\n%+v", snap)
	}
}

func TestWatch_NoChange_DoesNotEmit(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	fake := clock.NewFake()
	ch, _ := startWatcher(t, clone.Path, fake)

	waitForSnapshot(t, ch)
	waitForTimer(t, fake)

	// Don't mutate anything.
	fake.Advance(testInterval)

	// The watcher should poll, see no change, and register another timer
	// without emitting. Wait for that next timer to confirm a full no-op cycle.
	waitForTimer(t, fake)

	select {
	case snap := <-ch:
		t.Fatalf("expected no new snapshot when refs unchanged, got: %+v", snap)
	default:
	}
}

func TestWatch_ContextCancel_ClosesChannel(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	fake := clock.NewFake()
	ch, cancel := startWatcher(t, clone.Path, fake)

	waitForSnapshot(t, ch)
	waitForTimer(t, fake)

	cancel()

	// Drain anything that was already in flight, then expect the close.
	deadline := time.After(3 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // closed — pass
			}
			// drained one stale snapshot, keep going
		case <-deadline:
			t.Fatal("channel did not close after context cancel")
		}
	}
}
