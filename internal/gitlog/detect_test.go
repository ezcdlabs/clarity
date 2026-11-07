package gitlog_test

import (
	"os/exec"
	"testing"

	"github.com/ezcdlabs/clarity/internal/gitlog"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// TestDetectTrunk pins the runtime trunk-detection chain that replaces the
// blind "main" default when .ezcd.json doesn't pin a branch: origin/HEAD
// (the remote's default branch) first, then the conventional names by
// existence. Every candidate must actually resolve to a commit — a stale
// origin/HEAD falls through rather than being trusted. "" means nothing
// conventional resolved, and the caller keeps the old default so the
// pre-flight error (with its hint) stays the ending for that repo.
//
// The current branch is deliberately NOT in this chain: silently watching
// whatever happens to be checked out would point the dashboard — and the
// `check` push gate — at a feature branch, a wrong answer that looks right.
func TestDetectTrunk(t *testing.T) {
	t.Run("origin HEAD wins", func(t *testing.T) {
		remote := gittest.NewRemoteOnBranch(t, "master")
		clone := remote.NewClone(t)
		if got := gitlog.DetectTrunk(clone.Path); got != "master" {
			t.Errorf("DetectTrunk = %q, want master", got)
		}
	})

	t.Run("origin HEAD outranks a local main", func(t *testing.T) {
		remote := gittest.NewRemoteOnBranch(t, "master")
		clone := remote.NewClone(t)
		clone.CheckoutNew("main") // a stray local main must not shadow the remote's trunk
		if got := gitlog.DetectTrunk(clone.Path); got != "master" {
			t.Errorf("DetectTrunk = %q, want master", got)
		}
	})

	t.Run("no origin HEAD, main resolves", func(t *testing.T) {
		repo := gittest.NewLocalRepo(t, "main")
		repo.WriteFile("f", "")
		repo.CommitAll("init")
		if got := gitlog.DetectTrunk(repo.Path); got != "main" {
			t.Errorf("DetectTrunk = %q, want main", got)
		}
	})

	t.Run("no origin HEAD, only master resolves", func(t *testing.T) {
		repo := gittest.NewLocalRepo(t, "master")
		repo.WriteFile("f", "")
		repo.CommitAll("init")
		if got := gitlog.DetectTrunk(repo.Path); got != "master" {
			t.Errorf("DetectTrunk = %q, want master", got)
		}
	})

	t.Run("stale origin HEAD falls through to a name that resolves", func(t *testing.T) {
		remote := gittest.NewRemoteOnBranch(t, "master")
		clone := remote.NewClone(t)
		runGit(t, clone.Path, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/gone")
		if got := gitlog.DetectTrunk(clone.Path); got != "master" {
			t.Errorf("DetectTrunk = %q, want master", got)
		}
	})

	t.Run("nothing conventional resolves", func(t *testing.T) {
		repo := gittest.NewLocalRepo(t, "develop")
		repo.WriteFile("f", "")
		repo.CommitAll("init")
		if got := gitlog.DetectTrunk(repo.Path); got != "" {
			t.Errorf("DetectTrunk = %q, want empty — the current branch must not be guessed at runtime", got)
		}
	})

	t.Run("not a repository", func(t *testing.T) {
		if got := gitlog.DetectTrunk(t.TempDir()); got != "" {
			t.Errorf("DetectTrunk = %q, want empty", got)
		}
	})
}
