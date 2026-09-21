package gitlog_test

import (
	"testing"

	"github.com/ezcdlabs/clarity/internal/gitlog"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// TestWalk_UnderSharedObjectStore covers the commit walk, which is the other
// half of rendering a snapshot.
//
// Reading the events ref through git fixed reporting, but the walk still went
// through go-git, whose alternates support cannot follow a path outside the
// repository directory — so the TUI kept failing, and worse: the watcher
// swallowed the error and retried until the deadline, leaving the user with
// only "context deadline exceeded". The repository is opened with an
// alternates filesystem rooted at / so those paths resolve.
func TestWalk_UnderSharedObjectStore(t *testing.T) {
	remote := gittest.NewRemote(t)
	shared := remote.NewSharedObjectClone(t)

	commits, _, err := gitlog.Walk(shared.Path, "main", 10)
	if err != nil {
		t.Fatalf("Walk in a shared object store failed: %v", err)
	}
	if len(commits) == 0 {
		t.Fatal("expected at least the initial commit, got none")
	}
}
