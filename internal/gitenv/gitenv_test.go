package gitenv_test

import (
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/internal/gitenv"
)

// TestClean_StripsGitContextVars verifies that the three variables git sets
// when invoking external commands are removed from the returned environment.
func TestClean_StripsGitContextVars(t *testing.T) {
	t.Setenv("GIT_DIR", "/bad/path")
	t.Setenv("GIT_WORK_TREE", "/bad/tree")
	t.Setenv("GIT_PREFIX", "bad/")

	env := gitenv.Clean()

	for _, e := range env {
		key, _, _ := strings.Cut(e, "=")
		switch key {
		case "GIT_DIR":
			t.Error("Clean() should have stripped GIT_DIR")
		case "GIT_WORK_TREE":
			t.Error("Clean() should have stripped GIT_WORK_TREE")
		case "GIT_PREFIX":
			t.Error("Clean() should have stripped GIT_PREFIX")
		}
	}
}

// TestClean_PreservesOtherVars verifies that unrelated environment variables
// are not removed.
func TestClean_PreservesOtherVars(t *testing.T) {
	const canary = "CLARITY_TEST_CANARY"
	const value = "canary-value"
	t.Setenv(canary, value)

	env := gitenv.Clean()

	for _, e := range env {
		if e == canary+"="+value {
			return
		}
	}
	t.Errorf("Clean() removed %s=%s but it should have been preserved", canary, value)
}
