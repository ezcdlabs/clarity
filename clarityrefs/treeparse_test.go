package clarityrefs_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// The events ref is parsed out of `git ls-tree -r -z`, so the parser has to
// survive what a tree can legally contain. Every event clarity itself writes
// has a tame path, which is exactly why these need constructing by hand:
// nothing in the ordinary write path would ever produce them, and the parser
// would quietly mis-handle them until something else did.

// TestReadEvents_PathNeedingNULFraming covers a file name containing a
// newline. Without -z, git terminates entries with a newline and quotes such
// paths; splitting on newlines would tear the entry in half and the parse
// would either fail or silently drop events.
func TestReadEvents_PathNeedingNULFraming(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	ev := clarityrefs.Event{Stage: "ci", Status: "passed", Time: time.Unix(1744120134, 0)}
	if err := clarityrefs.WriteEvent(clone.Path, "origin", fakeSHA, ev); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if err := fetchEventsRef(clone.Path); err != nil {
		t.Fatalf("fetch: %v", err)
	}

	// A sibling file whose name holds a newline, a quote and non-ASCII.
	awkward := "events/" + fakeSHA + "/wei\nrd \"na\"me ünï.json"
	addFileToEventsRef(t, clone.Path, awkward, `{"stage":"ci","status":"failed","ts":1744120200}`)

	events, err := clarityrefs.ReadEvents(clone.Path, fakeSHA)
	if err != nil {
		t.Fatalf("ReadEvents with an awkward path failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected both events, got %d — the entry was mis-framed", len(events))
	}
	if events[1].Status != "failed" {
		t.Errorf("expected the awkwardly-named event to be read, got %q", events[1].Status)
	}
}

// TestReadEvents_SkipsNonBlobEntries covers a gitlink (a submodule entry) in
// the tree, named so that it passes the .json filter and actually reaches the
// parser. cat-file happily returns the *commit* it points at, so without the
// blob check that commit object is handed to the event decoder and the whole
// read fails on a repo that is otherwise perfectly fine.
func TestReadEvents_SkipsNonBlobEntries(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	ev := clarityrefs.Event{Stage: "ci", Status: "passed", Time: time.Unix(1744120134, 0)}
	if err := clarityrefs.WriteEvent(clone.Path, "origin", fakeSHA, ev); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if err := fetchEventsRef(clone.Path); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	addGitlinkToEventsRef(t, clone.Path, "events/"+fakeSHA+"/submodule.json")

	events, err := clarityrefs.ReadEvents(clone.Path, fakeSHA)
	if err != nil {
		t.Fatalf("a gitlink in the tree must be skipped, not fail the read: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected the one real event, got %d", len(events))
	}
}

// addFileToEventsRef adds one blob to the local events ref, via plumbing so
// the path can be anything a tree allows rather than anything clarity writes.
func addFileToEventsRef(t *testing.T, repoPath, path, content string) {
	t.Helper()
	blob := strings.TrimSpace(gitOutStdin(t, repoPath, content, "hash-object", "-w", "--stdin"))
	updateEventsTree(t, repoPath, "100644 blob "+blob+"\t"+path)
}

// addGitlinkToEventsRef adds a commit entry (mode 160000) to the events ref.
func addGitlinkToEventsRef(t *testing.T, repoPath, path string) {
	t.Helper()
	head := strings.TrimSpace(gitOut(t, repoPath, nil, "rev-parse", "HEAD"))
	updateEventsTree(t, repoPath, "160000 commit "+head+"\t"+path)
}

// updateEventsTree rewrites the events ref with one extra tree entry.
func updateEventsTree(t *testing.T, repoPath, entry string) {
	t.Helper()
	idx := filepath.Join(t.TempDir(), "index")
	env := []string{"GIT_INDEX_FILE=" + idx}

	gitOut(t, repoPath, env, "read-tree", clarityrefs.EventsRef)
	gitOutStdin2(t, repoPath, env, entry+"\x00", "update-index", "-z", "--index-info")
	tree := strings.TrimSpace(gitOut(t, repoPath, env, "write-tree"))
	commit := strings.TrimSpace(gitOutStdin(t, repoPath, "test", "commit-tree", tree))
	gitOut(t, repoPath, nil, "update-ref", clarityrefs.EventsRef, commit)
}

func gitOut(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	return gitOutStdin2(t, dir, env, "", args...)
}

func gitOutStdin(t *testing.T, dir, stdin string, args ...string) string {
	t.Helper()
	return gitOutStdin2(t, dir, nil, stdin, args...)
}

func gitOutStdin2(t *testing.T, dir string, env []string, stdin string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
