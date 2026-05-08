//go:build integration

// Package integration contains tests that run clarity against a live SSH git
// server (the "server" Docker Compose service) configured exactly like
// production GitHub: receive.fsckObjects=true, restricted git-shell user,
// pubkey-only auth.
//
// Run from the repo root with:
//
//	docker compose -f test/integration/docker-compose.yml run --rm client
//
// Or for a manual shell inside the client container:
//
//	docker compose -f test/integration/docker-compose.yml run --rm client bash
package integration_test

import (
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
)

const fakeSHA = "0123456789abcdef0123456789abcdef01234567"

// serverHost returns the hostname of the SSH git server from the environment.
// The test is skipped if the variable is not set (i.e. running outside Docker).
func serverHost(t *testing.T) string {
	t.Helper()
	h := os.Getenv("CLARITY_SSH_SERVER")
	if h == "" {
		t.Skip("CLARITY_SSH_SERVER not set — run inside the Docker Compose client container")
	}
	return h
}

// cloneRepo clones git@host:/home/git/repo.git into a temp directory.
func cloneRepo(t *testing.T, host string) string {
	t.Helper()
	dir := t.TempDir()
	remoteURL := fmt.Sprintf("git@%s:/home/git/repo.git", host)
	run(t, "", "git", "clone", remoteURL, dir)
	run(t, dir, "git", "config", "user.email", "test@clarity")
	run(t, dir, "git", "config", "user.name", "Test")
	return dir
}

// fetchEventsRef pulls the events ref so reads see the latest server state.
func fetchEventsRef(t *testing.T, repoPath string) {
	t.Helper()
	cmd := exec.Command("git", "fetch", "origin", "+refs/clarity/events:refs/clarity/events")
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil &&
		!strings.Contains(string(out), "couldn't find remote ref") {
		t.Fatalf("fetch events ref: %v\n%s", err, out)
	}
}

// TestSSHWriteEvent_LandsOnEventsRef is the basic end-to-end: write one event
// and read it back from a fresh clone. Exercises the optimistic push loop and
// nested-tree builder against a real SSH+fsckObjects remote.
func TestSSHWriteEvent_LandsOnEventsRef(t *testing.T) {
	host := serverHost(t)
	clone := cloneRepo(t, host)

	// Use a unique SHA suffix so multiple test runs against the same server
	// don't conflict.
	uniqueSHA := fmt.Sprintf("%040x", time.Now().UnixNano())[:40]
	ev := clarityrefs.Event{
		Stage:  "build",
		Status: "passed",
		Time:   time.Now(),
	}
	if err := clarityrefs.WriteEvent(clone, "origin", uniqueSHA, ev); err != nil {
		t.Fatalf("WriteEvent over SSH failed: %v", err)
	}

	reader := cloneRepo(t, host)
	fetchEventsRef(t, reader)
	got, err := clarityrefs.ReadEvents(reader, uniqueSHA)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Stage != "build" || got[0].Status != "passed" {
		t.Errorf("unexpected event: %+v", got[0])
	}
}

// TestSSHConcurrentWriters_BothLand exercises the FF-retry path against a real
// network remote. Two clones write to the same SHA simultaneously; both events
// must be present in the final state.
func TestSSHConcurrentWriters_BothLand(t *testing.T) {
	host := serverHost(t)
	clone1 := cloneRepo(t, host)
	clone2 := cloneRepo(t, host)

	uniqueSHA := fmt.Sprintf("%040x", time.Now().UnixNano())[:40]

	var wg sync.WaitGroup
	var err1, err2 error
	wg.Add(2)
	go func() {
		defer wg.Done()
		ev := clarityrefs.Event{Stage: "build", Status: "passed", Time: time.Now()}
		err1 = clarityrefs.WriteEvent(clone1, "origin", uniqueSHA, ev)
	}()
	go func() {
		defer wg.Done()
		ev := clarityrefs.Event{Stage: "deploy", Status: "passed", Time: time.Now()}
		err2 = clarityrefs.WriteEvent(clone2, "origin", uniqueSHA, ev)
	}()
	wg.Wait()
	if err1 != nil {
		t.Errorf("clone1 WriteEvent: %v", err1)
	}
	if err2 != nil {
		t.Errorf("clone2 WriteEvent: %v", err2)
	}

	reader := cloneRepo(t, host)
	fetchEventsRef(t, reader)
	got, err := clarityrefs.ReadEvents(reader, uniqueSHA)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 events after concurrent writes, got %d", len(got))
	}
}

// TestSSHFsckObjects_RejectsInvalidTree verifies the test server is correctly
// configured: a tree with a slash in an entry name (the old "fullPathname"
// bug shape) is rejected. Without this check, a regression to the flat-tree
// bug would silently pass against the test server.
func TestSSHFsckObjects_RejectsInvalidTree(t *testing.T) {
	host := serverHost(t)
	repoPath := cloneRepo(t, host)

	blobHash := strings.TrimSpace(runOutputStdin(t, repoPath, "test data\n",
		"git", "hash-object", "-w", "--stdin"))

	blobHashBytes, err := hex.DecodeString(blobHash)
	if err != nil {
		t.Fatalf("decode blob hash: %v", err)
	}
	// Raw git tree entry: "<mode> <name>\0<20-byte-binary-sha>"
	// "events/abc.json" with a slash is exactly what the broken flat-tree code
	// would produce.
	var rawTree []byte
	rawTree = append(rawTree, []byte("100644 events/abc.json\x00")...)
	rawTree = append(rawTree, blobHashBytes...)
	treeHash := strings.TrimSpace(runOutputStdin(t, repoPath, string(rawTree),
		"git", "hash-object", "--literally", "-t", "tree", "-w", "--stdin"))

	parentHash := strings.TrimSpace(runOutput(t, repoPath, "git", "rev-parse", "origin/main"))
	env := append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
	)
	commitCmd := exec.Command("git", "commit-tree", treeHash, "-p", parentHash, "-m", "bad tree")
	commitCmd.Dir = repoPath
	commitCmd.Env = env
	commitOut, err := commitCmd.Output()
	if err != nil {
		t.Fatalf("commit-tree: %v", err)
	}
	badCommit := strings.TrimSpace(string(commitOut))

	pushCmd := exec.Command("git", "push", "origin", badCommit+":refs/heads/probe-fsck")
	pushCmd.Dir = repoPath
	out, err := pushCmd.CombinedOutput()
	t.Logf("push output:\n%s", out)
	if err == nil {
		t.Fatal("expected push to be rejected by receive.fsckObjects, but it succeeded")
	}
	if !strings.Contains(string(out), "fullPathname") && !strings.Contains(string(out), "fsck") {
		t.Fatalf("push failed but not with an fsck error; got:\n%s", out)
	}
}

// --- helpers -----------------------------------------------------------------

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
}

func runOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return string(out)
}

func runOutputStdin(t *testing.T, dir, stdin string, args ...string) string {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return string(out)
}
