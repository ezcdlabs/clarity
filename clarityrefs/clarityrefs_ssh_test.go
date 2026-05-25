//go:build ssh

// SSH-backed integration tests for clarityrefs. These exercise the write
// path against a real SSH git server (an alpine+openssh container with
// receive.fsckObjects=true, mirroring GitHub's enforcement) — the exact
// behaviours that the file-backed local tests can't catch:
//
//   - Concurrent push / FF-retry over real network round-trips, not
//     local-fs writes.
//   - GitHub's receive.fsckObjects rejection of the flat-tree shape
//     (the "fullPathname" class of bug). Only triggers against a remote
//     git server, never against a local-file remote.
//
// Gated by the `ssh` build tag; default `go test ./...` doesn't run them
// (and doesn't even compile testcontainers-go's transitive dependencies).
// Run with `go test -tags ssh ./clarityrefs/`.
package clarityrefs_test

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
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// TestSSHWriteEvent_LandsOnEventsRef is the basic end-to-end: write one event
// and read it back from a fresh clone. Exercises the optimistic push loop and
// nested-tree builder against a real SSH+fsckObjects remote.
func TestSSHWriteEvent_LandsOnEventsRef(t *testing.T) {
	remote := gittest.NewSSHRemote(t)
	writer := remote.NewClone(t)

	uniqueSHA := fmt.Sprintf("%040x", time.Now().UnixNano())[:40]
	ev := clarityrefs.Event{
		Stage:  "ci",
		Status: "passed",
		Time:   time.Now(),
	}
	if err := clarityrefs.WriteEvent(writer.Path, "origin", uniqueSHA, ev); err != nil {
		t.Fatalf("WriteEvent over SSH failed: %v", err)
	}

	reader := remote.NewClone(t)
	fetchEventsRefSSH(t, reader.Path)
	got, err := clarityrefs.ReadEvents(reader.Path, uniqueSHA)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Stage != "ci" || got[0].Status != "passed" {
		t.Errorf("unexpected event: %+v", got[0])
	}
}

// TestSSHConcurrentWriters_BothLand exercises the FF-retry path against a real
// network remote. Two clones write to the same SHA simultaneously; both events
// must be present in the final state.
func TestSSHConcurrentWriters_BothLand(t *testing.T) {
	remote := gittest.NewSSHRemote(t)
	clone1 := remote.NewClone(t)
	clone2 := remote.NewClone(t)

	uniqueSHA := fmt.Sprintf("%040x", time.Now().UnixNano())[:40]

	var wg sync.WaitGroup
	var err1, err2 error
	wg.Add(2)
	go func() {
		defer wg.Done()
		ev := clarityrefs.Event{Stage: "ci", Status: "passed", Time: time.Now()}
		err1 = clarityrefs.WriteEvent(clone1.Path, "origin", uniqueSHA, ev)
	}()
	go func() {
		defer wg.Done()
		ev := clarityrefs.Event{Stage: "deploy", Status: "passed", Time: time.Now()}
		err2 = clarityrefs.WriteEvent(clone2.Path, "origin", uniqueSHA, ev)
	}()
	wg.Wait()
	if err1 != nil {
		t.Errorf("clone1 WriteEvent: %v", err1)
	}
	if err2 != nil {
		t.Errorf("clone2 WriteEvent: %v", err2)
	}

	reader := remote.NewClone(t)
	fetchEventsRefSSH(t, reader.Path)
	got, err := clarityrefs.ReadEvents(reader.Path, uniqueSHA)
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
// bug would silently pass against any local-file remote — only real git
// servers with receive.fsckObjects=true catch it.
func TestSSHFsckObjects_RejectsInvalidTree(t *testing.T) {
	remote := gittest.NewSSHRemote(t)
	clone := remote.NewClone(t)

	blobHash := strings.TrimSpace(runOutputStdinSSH(t, clone.Path, "test data\n",
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
	treeHash := strings.TrimSpace(runOutputStdinSSH(t, clone.Path, string(rawTree),
		"git", "hash-object", "--literally", "-t", "tree", "-w", "--stdin"))

	parentHash := strings.TrimSpace(runOutputSSH(t, clone.Path, "git", "rev-parse", "origin/main"))
	env := append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
	)
	commitCmd := exec.Command("git", "commit-tree", treeHash, "-p", parentHash, "-m", "bad tree")
	commitCmd.Dir = clone.Path
	commitCmd.Env = env
	commitOut, err := commitCmd.Output()
	if err != nil {
		t.Fatalf("commit-tree: %v", err)
	}
	badCommit := strings.TrimSpace(string(commitOut))

	pushCmd := exec.Command("git", "push", "origin", badCommit+":refs/heads/probe-fsck")
	pushCmd.Dir = clone.Path
	out, err := pushCmd.CombinedOutput()
	t.Logf("push output:\n%s", out)
	if err == nil {
		t.Fatal("expected push to be rejected by receive.fsckObjects, but it succeeded")
	}
	if !strings.Contains(string(out), "fullPathname") && !strings.Contains(string(out), "fsck") {
		t.Fatalf("push failed but not with an fsck error; got:\n%s", out)
	}
}

// --- helpers (SSH-suffixed so they don't collide with the local-backend
//   helpers in clarityrefs_test.go's package) -----------------------------

// fetchEventsRefSSH pulls the events ref so reads see the latest server state.
// Mirrors the helper that used to live in test/integration/clarityrefs_test.go.
func fetchEventsRefSSH(t *testing.T, repoPath string) {
	t.Helper()
	cmd := exec.Command("git", "fetch", "origin", "+refs/clarity/events:refs/clarity/events")
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil &&
		!strings.Contains(string(out), "couldn't find remote ref") {
		t.Fatalf("fetch events ref: %v\n%s", err, out)
	}
}

func runOutputSSH(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return string(out)
}

func runOutputStdinSSH(t *testing.T, dir, stdin string, args ...string) string {
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
