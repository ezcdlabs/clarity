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

// TestClean_PinsMessageLocale verifies that git is asked for untranslated
// messages.
//
// Clarity classifies several git outcomes by matching git's own English text
// — "couldn't find remote ref" to recognise a repo that has never reported,
// "unknown option" to fall back on old git, plus the push-rejection and
// packfile matchers. Those strings are gettext-translated, so under a French
// or Japanese locale every one of them silently stops matching, and the
// first-ever report in a repo turns into a hard failure.
func TestClean_PinsMessageLocale(t *testing.T) {
	t.Setenv("LC_ALL", "fr_FR.UTF-8")
	t.Setenv("LANG", "fr_FR.UTF-8")
	t.Setenv("LANGUAGE", "fr")
	t.Setenv("LC_MESSAGES", "fr_FR.UTF-8")

	env := gitenv.Clean()

	// Last value wins in exec, so assert on the effective value rather than
	// on mere presence.
	effective := map[string]string{}
	for _, e := range env {
		if k, v, ok := strings.Cut(e, "="); ok {
			effective[k] = v
		}
	}
	if got := effective["LC_ALL"]; got != "C" {
		t.Errorf("LC_ALL = %q, want %q", got, "C")
	}
	// gettext consults LANGUAGE ahead of LC_ALL, so leaving it set would
	// re-translate the messages LC_ALL=C was meant to pin.
	if got, ok := effective["LANGUAGE"]; ok && got != "" {
		t.Errorf("LANGUAGE = %q, want it empty", got)
	}
	if got := effective["LC_MESSAGES"]; got != "" && got != "C" {
		t.Errorf("LC_MESSAGES = %q, want it unset or C", got)
	}
}
