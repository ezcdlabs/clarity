package refs_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/internal/gittest"
	"github.com/ezcdlabs/clarity/internal/refs"
)

// seedEventsRef pushes an empty commit onto refs/clarity/events on the remote
// so EnsureClarityFetchRefspec's "is the events ref published?" gate returns
// true. Without this, the bootstrap correctly skips itself for tests that
// just want to exercise the add-the-refspec path.
func seedEventsRef(t *testing.T, c *gittest.Clone) {
	t.Helper()
	c.WriteFile(".clarity-seed", "")
	c.CommitAll("seed clarity events ref")
	c.PushRef("HEAD", "refs/clarity/events")
}

func gitConfigGetAll(t *testing.T, repoPath, key string) []string {
	t.Helper()
	cmd := exec.Command("git", "config", "--get-all", key)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		// git config exits 1 when the key is unset.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil
		}
		t.Fatalf("git config --get-all %s: %v", key, err)
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// Regression: running EnsureClarityFetchRefspec against a remote with no
// events ref yet (e.g. a fresh repo where nobody has called `git clarity
// report` ever) must not break a subsequent plain `git fetch`. The old
// behaviour added the refspec unconditionally, and `git fetch` aborts the
// whole command when any configured refspec fails with "couldn't find
// remote ref".
func TestEnsureClarityFetchRefspec_DoesNotBreakGitFetch_WhenNoEventsRef(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	if err := refs.EnsureClarityFetchRefspec(clone.Path, "origin"); err != nil {
		t.Fatalf("EnsureClarityFetchRefspec: %v", err)
	}

	cmd := exec.Command("git", "fetch", "origin")
	cmd.Dir = clone.Path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git fetch failed after bootstrap (the bug):\n%s", out)
	}
}

func TestEnsureClarityFetchRefspec_AddsWhenMissing(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	seedEventsRef(t, clone)

	if err := refs.EnsureClarityFetchRefspec(clone.Path, "origin"); err != nil {
		t.Fatalf("EnsureClarityFetchRefspec: %v", err)
	}

	got := gitConfigGetAll(t, clone.Path, "remote.origin.fetch")
	want := "+refs/clarity/events:refs/clarity/events"
	found := false
	for _, line := range got {
		if line == want {
			found = true
		}
	}
	if !found {
		t.Errorf("expected %q in remote.origin.fetch, got: %v", want, got)
	}
}

func TestEnsureClarityFetchRefspec_NoOpWhenAlreadyPresent(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	seedEventsRef(t, clone)

	// First call adds the refspec.
	if err := refs.EnsureClarityFetchRefspec(clone.Path, "origin"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	// Second call should be a no-op.
	if err := refs.EnsureClarityFetchRefspec(clone.Path, "origin"); err != nil {
		t.Fatalf("second call: %v", err)
	}

	got := gitConfigGetAll(t, clone.Path, "remote.origin.fetch")
	want := "+refs/clarity/events:refs/clarity/events"
	count := 0
	for _, line := range got {
		if line == want {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 %q in remote.origin.fetch, got %d (all: %v)", want, count, got)
	}
}

func TestEnsureClarityFetchRefspec_PreservesExistingRefspecs(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	seedEventsRef(t, clone)

	// The clone already has a default fetch refspec from `git clone`. Verify
	// EnsureClarityFetchRefspec doesn't clobber it.
	before := gitConfigGetAll(t, clone.Path, "remote.origin.fetch")
	if len(before) == 0 {
		t.Fatal("expected default fetch refspec to be present after clone")
	}

	if err := refs.EnsureClarityFetchRefspec(clone.Path, "origin"); err != nil {
		t.Fatalf("EnsureClarityFetchRefspec: %v", err)
	}

	after := gitConfigGetAll(t, clone.Path, "remote.origin.fetch")
	for _, original := range before {
		found := false
		for _, line := range after {
			if line == original {
				found = true
			}
		}
		if !found {
			t.Errorf("original refspec %q was lost; after = %v", original, after)
		}
	}
}
