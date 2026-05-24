//go:build ssh

// Tests for the SSH backend of gittest. Gated by the `ssh` build tag because
// they require Docker (testcontainers-go starts an alpine+openssh container
// per test) — `go test ./...` keeps running fast for the local backend, and
// CI / on-demand runs use `go test -tags ssh ./...` to exercise this suite.
package gittest_test

import (
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/internal/gittest"
)

// TestNewSSHRemote_StartsContainer_AndExposesURL is the simplest possible
// smoke test for the SSH backend: spin up the container, get back a Remote,
// and verify the URL looks ssh-shaped. Doesn't yet validate that anything
// is actually cloneable — TestSSHRemote_NewClone_PullsSeedCommit covers that.
func TestNewSSHRemote_StartsContainer_AndExposesURL(t *testing.T) {
	remote := gittest.NewSSHRemote(t)

	url := remote.URL()
	if !strings.HasPrefix(url, "ssh://git@") {
		t.Errorf("expected ssh:// URL with git@ user, got %q", url)
	}
	if !strings.HasSuffix(url, "/home/git/repo.git") {
		t.Errorf("expected URL to end with /home/git/repo.git (server's bare repo path), got %q", url)
	}
}

// TestSSHRemote_NewClone_PullsSeedCommit is the end-to-end validation that
// the GIT_SSH_COMMAND override actually works: clone the SSH-hosted bare
// repo via git over SSH, and confirm the seed "initial commit" from the
// server's Dockerfile-baked setup arrives. If this passes, key, port, and
// host-key-bypass plumbing are all correctly wired.
func TestSSHRemote_NewClone_PullsSeedCommit(t *testing.T) {
	remote := gittest.NewSSHRemote(t)
	clone := remote.NewClone(t)

	commits := clone.LogBranch("main")
	if len(commits) == 0 {
		t.Fatal("expected at least the seed commit in the clone, got nothing")
	}
	// The Dockerfile commits `initial commit` as the seed.
	found := false
	for _, c := range commits {
		if c.Message == "initial commit" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected seed `initial commit` in branch history, got %+v", commits)
	}
}

// TestSSHRemote_Push_RoundTrips validates the push direction. A clone
// commits a new file, pushes to main over SSH, and a SECOND independent
// clone fetches main and confirms the pushed commit arrives. This
// exercises the same code path clarityrefs's WriteEvent uses against the
// production SSH server, so it's the foundation the relocated SSH
// clarityrefs tests will sit on top of.
func TestSSHRemote_Push_RoundTrips(t *testing.T) {
	remote := gittest.NewSSHRemote(t)

	writer := remote.NewClone(t)
	writer.WriteFile("hello.txt", "from the writer clone")
	writer.CommitAll("writer commit over SSH")
	writer.Push("main")

	reader := remote.NewClone(t)
	commits := reader.LogBranch("main")
	found := false
	for _, c := range commits {
		if c.Message == "writer commit over SSH" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("reader clone did not see writer's commit; got %+v", commits)
	}
}
