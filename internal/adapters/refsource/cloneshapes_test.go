package refsource_test

import (
	"context"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/adapters/refsource"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// The watcher does its own fetch of the events ref, so it has to be as
// independent of the clone's shape as the reporting path is. These go through
// Source.Watch rather than calling BuildSnapshot directly, because the fetch
// under test only happens inside Watch — a test that fetches for itself would
// pass with the watcher's own fetch reverted, which is exactly the gap that
// let the first version of this fix ship untested.

// TestWatch_BloblessPartialClone covers a working copy cloned with
// --filter=blob:none, where a fetch inherits the filter and leaves the event
// JSON blobs on the server unless clarity asks for them explicitly.
func TestWatch_BloblessPartialClone(t *testing.T) {
	remote := gittest.NewRemote(t)
	seedEvent(t, remote)

	partial := remote.NewBloblessClone(t)
	assertWatchSeesTheEvent(t, partial.Path)
}

// TestWatch_SharedObjectStore covers a working copy whose objects live in a
// mirror reached through .git/objects/info/alternates.
func TestWatch_SharedObjectStore(t *testing.T) {
	remote := gittest.NewRemote(t)
	seedEvent(t, remote)

	shared := remote.NewSharedObjectClone(t)
	assertWatchSeesTheEvent(t, shared.Path)
}

func seedEvent(t *testing.T, remote *gittest.Remote) {
	t.Helper()
	seed := remote.NewClone(t)
	seed.WriteFile("f.txt", "hello")
	seed.CommitAll("feat: something")
	seed.Push("main")

	head := seed.LogBranch("main")[0].Hash
	ev := clarityrefs.Event{Stage: "ci", Status: "passed", Time: time.Unix(1744120134, 0)}
	if err := clarityrefs.WriteEvent(seed.Path, "origin", head, ev); err != nil {
		t.Fatalf("seeding the remote events ref failed: %v", err)
	}
}

func assertWatchSeesTheEvent(t *testing.T, repoPath string) {
	t.Helper()

	src, err := refsource.New(refsource.Options{RepoPath: repoPath, Branch: "main"})
	if err != nil {
		t.Fatalf("refsource.New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	select {
	case snap, ok := <-src.Watch(ctx):
		if !ok {
			t.Fatal("watch channel closed without emitting a snapshot")
		}
		if len(snap.Commits) == 0 {
			t.Fatal("snapshot has no commits")
		}
		// The top commit is the one the event was written against, so an
		// events ref that failed to arrive shows up as an empty Events map
		// rather than as an error.
		top := snap.Commits[0]
		if len(top.Events) == 0 {
			t.Fatalf("no events on %s: the events ref did not arrive intact", top.SHA[:8])
		}
		if top.Events[0].Status != "passed" {
			t.Errorf("expected the seeded passed event, got %q", top.Events[0].Status)
		}
	case <-ctx.Done():
		t.Fatal("watch produced no snapshot before the deadline")
	}
}
