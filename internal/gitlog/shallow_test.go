package gitlog_test

import (
	"testing"

	"github.com/ezcdlabs/clarity/internal/gitlog"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// TestWalk_ShallowClone covers the shape actions/checkout produces by
// default: `fetch-depth: 1`, so the repository holds one commit whose parents
// are deliberately absent and the boundary is recorded in .git/shallow.
//
// git honours that graft and stops. go-git does not read .git/shallow, so it
// followed the parent pointer into an object that was never downloaded and
// failed the whole walk with "object not found" — which surfaced to the user
// as `git clarity --plain` dying with "context deadline exceeded", on the
// single most common CI checkout there is.
func TestWalk_ShallowClone(t *testing.T) {
	remote := gittest.NewRemote(t)
	seed := remote.NewClone(t)
	for _, msg := range []string{"feat: one", "feat: two", "feat: three"} {
		seed.WriteFile("f.txt", msg)
		seed.CommitAll(msg)
	}
	seed.Push("main")

	shallow := remote.NewShallowClone(t)

	commits, more, err := gitlog.Walk(shallow.Path, "main", 50)
	if err != nil {
		t.Fatalf("Walk on a shallow clone failed: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected the one commit the clone holds, got %d", len(commits))
	}
	if commits[0].Subject != "feat: three" {
		t.Errorf("expected the tip commit, got %q", commits[0].Subject)
	}
	// The history genuinely continues on the remote, but this clone cannot
	// see it. Claiming "more" would be guessing at something it does not know.
	if more {
		t.Error("a shallow boundary is not the same as a truncated walk; more should be false")
	}
	if commits[0].Author == "" || commits[0].Time.IsZero() {
		t.Errorf("commit metadata is incomplete: %+v", commits[0])
	}
}
