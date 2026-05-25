package refsource_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/adapters/refsource"
	"github.com/ezcdlabs/clarity/internal/clock"
	"github.com/ezcdlabs/clarity/internal/core"
	"github.com/ezcdlabs/clarity/internal/gittest"
	"github.com/ezcdlabs/clarity/internal/refs"
)

// Compile-time check: the adapter satisfies the core.Source port. Drifting
// away from the interface fails the build before any test runs.
var _ core.Source = (*refsource.Source)(nil)

const testInterval = 5 * time.Second

// --- Source.Watch behaviour (migrated from internal/watcher/watcher_test.go) -

func waitForSnapshot(t *testing.T, ch <-chan core.Snapshot) core.Snapshot {
	t.Helper()
	select {
	case snap := <-ch:
		return snap
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for snapshot")
		return core.Snapshot{}
	}
}

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

func startSource(t *testing.T, repoPath string, fake *clock.Fake) (<-chan core.Snapshot, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	src, err := refsource.New(refsource.Options{
		RepoPath: repoPath,
		Remote:   "origin",
		Branch:   "main",
		Interval: testInterval,
		Limit:    50,
		Clock:    fake,
	})
	if err != nil {
		t.Fatalf("refsource.New: %v", err)
	}
	return src.Watch(ctx), cancel
}

func TestSource_Watch_EmitsInitialSnapshot(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	fake := clock.NewFake()
	ch, _ := startSource(t, clone.Path, fake)

	snap := waitForSnapshot(t, ch)
	if len(snap.Commits) == 0 {
		t.Fatal("expected at least the seed commit in the initial snapshot")
	}
}

// TestSource_PropagatesRepoName fixes the contract that the Source stamps
// the RepoName onto every Snapshot it emits, so downstream Renderers can
// look it up via View.Snapshot.RepoName instead of carrying it through
// their constructor. Drift on this would silently strip the header label.
func TestSource_PropagatesRepoName(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	src, err := refsource.New(refsource.Options{
		RepoPath: clone.Path,
		Remote:   "origin",
		Branch:   "main",
		RepoName: "myrepo",
		Interval: testInterval,
		Limit:    50,
		Clock:    clock.NewFake(),
	})
	if err != nil {
		t.Fatalf("refsource.New: %v", err)
	}
	snap := waitForSnapshot(t, src.Watch(ctx))
	if snap.RepoName != "myrepo" {
		t.Errorf("expected RepoName=%q on emitted Snapshot, got %q", "myrepo", snap.RepoName)
	}
}

func TestSource_Watch_BranchChange_EmitsNewSnapshot(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	other := remote.NewClone(t)

	fake := clock.NewFake()
	ch, _ := startSource(t, clone.Path, fake)

	first := waitForSnapshot(t, ch)
	firstTop := first.Commits[0].SHA

	waitForTimer(t, fake)

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

func TestSource_Watch_EventsRefChange_EmitsNewSnapshot(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "a")
	clone.CommitAll("a commit")
	clone.Push("main")
	committedSHA := headSHA(t, clone.Path)

	fake := clock.NewFake()
	ch, _ := startSource(t, clone.Path, fake)

	waitForSnapshot(t, ch)
	waitForTimer(t, fake)

	ev := clarityrefs.Event{Stage: "ci", Status: "passed", Time: time.Unix(1744120134, 0)}
	if err := clarityrefs.WriteEvent(clone.Path, "origin", committedSHA, ev); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}

	fake.Advance(testInterval)

	snap := waitForSnapshot(t, ch)
	var found bool
	for _, c := range snap.Commits {
		if c.SHA == committedSHA && len(c.Events) >= 1 && c.Events[0].Stage == "ci" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected the new event to appear on the committed SHA in the next snapshot:\n%+v", snap)
	}
}

func TestSource_Watch_NoChange_DoesNotEmit(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	fake := clock.NewFake()
	ch, _ := startSource(t, clone.Path, fake)

	waitForSnapshot(t, ch)
	waitForTimer(t, fake)

	fake.Advance(testInterval)
	waitForTimer(t, fake)

	select {
	case snap := <-ch:
		t.Fatalf("expected no new snapshot when refs unchanged, got: %+v", snap)
	default:
	}
}

func TestSource_Watch_ContextCancel_ClosesChannel(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	fake := clock.NewFake()
	ch, cancel := startSource(t, clone.Path, fake)

	waitForSnapshot(t, ch)
	waitForTimer(t, fake)

	cancel()

	deadline := time.After(3 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("channel did not close after context cancel")
		}
	}
}

// --- New() setup behaviour --------------------------------------------------

// TestNew_RegistersClarityFetchRefspec verifies the plan's promise that
// refsource owns its own setup: a caller no longer has to remember to call
// refs.EnsureClarityFetchRefspec before instantiating a Source. The gate
// inside EnsureClarityFetchRefspec means we need a remote that *does*
// publish the events ref before the refspec gets added, so seed one event
// first and confirm the refspec lands after New().
func TestNew_RegistersClarityFetchRefspec(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	// Seed an event so the remote actually publishes refs/clarity/events —
	// that's the condition under which EnsureClarityFetchRefspec is allowed
	// to register the refspec.
	clone.WriteFile("seed.txt", "seed")
	clone.CommitAll("seed commit")
	clone.Push("main")
	sha := headSHA(t, clone.Path)
	if err := clarityrefs.WriteEvent(clone.Path, "origin", sha,
		clarityrefs.Event{Stage: "ci", Status: "passed", Time: time.Unix(1, 0)}); err != nil {
		t.Fatalf("seed WriteEvent: %v", err)
	}

	if _, err := refsource.New(refsource.Options{
		RepoPath: clone.Path,
		Remote:   "origin",
		Branch:   "main",
		Interval: testInterval,
		Limit:    50,
		Clock:    clock.NewFake(),
	}); err != nil {
		t.Fatalf("refsource.New: %v", err)
	}

	cmd := exec.Command("git", "config", "--get-all", "remote.origin.fetch")
	cmd.Dir = clone.Path
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git config --get-all: %v", err)
	}
	if !strings.Contains(string(out), refs.ClarityFetchRefspec) {
		t.Errorf("expected remote.origin.fetch to include %q, got:\n%s",
			refs.ClarityFetchRefspec, out)
	}
}
