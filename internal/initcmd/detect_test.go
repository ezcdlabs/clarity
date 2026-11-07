package initcmd_test

import (
	"testing"

	"github.com/ezcdlabs/clarity/internal/gittest"
	"github.com/ezcdlabs/clarity/internal/initcmd"
)

// TestDetectTrunk pins where the branch prompt's suggestion comes from when
// no .ezcd.json names one yet — which is always the case during a first init,
// because init is what writes the file. The remote's default branch
// (origin/HEAD) outranks whatever happens to be checked out, since init can
// be run from a feature branch; a repo with no remote falls back to the
// current branch; and a directory git can't answer for yields "", leaving
// the caller to fall back to the config default.
func TestDetectTrunk(t *testing.T) {
	t.Run("remote default branch wins", func(t *testing.T) {
		remote := gittest.NewRemoteOnBranch(t, "master")
		clone := remote.NewClone(t)
		if got := initcmd.DetectTrunk(clone.Path); got != "master" {
			t.Errorf("DetectTrunk = %q, want master", got)
		}
	})

	t.Run("remote default outranks the checked-out branch", func(t *testing.T) {
		remote := gittest.NewRemoteOnBranch(t, "master")
		clone := remote.NewClone(t)
		clone.CheckoutNew("feature")
		if got := initcmd.DetectTrunk(clone.Path); got != "master" {
			t.Errorf("DetectTrunk = %q, want master", got)
		}
	})

	t.Run("no remote falls back to the current branch", func(t *testing.T) {
		repo := gittest.NewLocalRepo(t, "develop")
		if got := initcmd.DetectTrunk(repo.Path); got != "develop" {
			t.Errorf("DetectTrunk = %q, want develop", got)
		}
	})

	t.Run("not a repository yields empty", func(t *testing.T) {
		if got := initcmd.DetectTrunk(t.TempDir()); got != "" {
			t.Errorf("DetectTrunk = %q, want empty", got)
		}
	})
}
