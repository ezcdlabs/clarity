package gittest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/internal/gittest"
)

func commandFromString(s string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		parts := strings.SplitN(s, " ", -1)
		return exec.Command(parts[0], parts[1:]...)
	}
	return exec.Command("sh", "-c", s)
}

// TestNewRemote_IsBareRepo verifies that NewRemote produces a real bare git
// repository that git itself considers valid.
func TestNewRemote_IsBareRepo(t *testing.T) {
	remote := gittest.NewRemote(t)

	// A bare repo has HEAD but no working tree — check the canonical indicator.
	if _, err := os.Stat(filepath.Join(remote.Path, "HEAD")); err != nil {
		t.Fatalf("expected HEAD in bare repo at %s: %v", remote.Path, err)
	}
	if _, err := os.Stat(filepath.Join(remote.Path, ".git")); err == nil {
		t.Fatalf("bare repo should not have a .git directory")
	}
}

// TestNewRemote_HasInitialCommitOnMain verifies that the remote starts with an
// initial commit on main, so clones have a valid base to work from.
func TestNewRemote_HasInitialCommitOnMain(t *testing.T) {
	remote := gittest.NewRemote(t)

	commits := remote.LogBranch("main")
	if len(commits) == 0 {
		t.Fatal("expected at least one commit on main")
	}
}

// TestClone_CanCommitAndPushToRemote verifies the basic clone → commit → push
// round-trip works.
func TestClone_CanCommitAndPushToRemote(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	clone.WriteFile("hello.txt", "hello world")
	clone.CommitAll("add hello.txt")
	clone.Push("main")

	commits := remote.LogBranch("main")
	if commits[0].Message != "add hello.txt" {
		t.Fatalf("expected top commit message %q, got %q", "add hello.txt", commits[0].Message)
	}
}

// TestClone_MultipleClones_CanSeeEachOthersPushes verifies that two
// independent clones of the same remote can exchange commits via the remote —
// the multi-developer scenario the acceptance tests depend on.
func TestClone_MultipleClones_CanSeeEachOthersPushes(t *testing.T) {
	remote := gittest.NewRemote(t)
	alice := remote.NewClone(t)
	bob := remote.NewClone(t)

	alice.WriteFile("alice.txt", "alice was here")
	alice.CommitAll("alice's commit")
	alice.Push("main")

	bob.Fetch()
	commits := bob.LogBranch("origin/main")
	if commits[0].Message != "alice's commit" {
		t.Fatalf("bob should see alice's commit after fetch, got %q", commits[0].Message)
	}
}

// TestRemote_ListRefs verifies that refs pushed to the remote are visible.
func TestRemote_ListRefs(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	clone.WriteFile("file.txt", "content")
	clone.CommitAll("some commit")
	clone.PushRef("HEAD", "refs/custom/my-ref")

	refs := remote.ListRefs()
	found := false
	for _, r := range refs {
		if r == "refs/custom/my-ref" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected refs/custom/my-ref in remote refs, got: %v", refs)
	}
}

// TestRemote_DeleteRef verifies that a ref can be deleted from the remote.
func TestRemote_DeleteRef(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	clone.WriteFile("file.txt", "content")
	clone.CommitAll("some commit")
	clone.PushRef("HEAD", "refs/custom/my-ref")

	remote.DeleteRef("refs/custom/my-ref")

	refs := remote.ListRefs()
	for _, r := range refs {
		if r == "refs/custom/my-ref" {
			t.Fatal("expected refs/custom/my-ref to be deleted")
		}
	}
}

// TestClone_WriteFile_CreatesFileOnDisk verifies that WriteFile actually
// creates the file at the right path in the clone's working tree.
func TestClone_WriteFile_CreatesFileOnDisk(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	clone.WriteFile("subdir/file.txt", "some content")

	data, err := os.ReadFile(filepath.Join(clone.Path, "subdir", "file.txt"))
	if err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	if string(data) != "some content" {
		t.Fatalf("expected %q, got %q", "some content", string(data))
	}
}

// TestRemote_ReadFileAtRef verifies that files committed to a ref are readable.
func TestRemote_ReadFileAtRef(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	clone.WriteFile("events/abc123/test.json", `{"stage":"build","status":"passed"}`)
	clone.CommitAll("report: abc123 build passed")
	clone.PushRef("HEAD", "refs/clarity/events")

	content, ok := remote.ReadFileAtRef("refs/clarity/events", "events/abc123/test.json")
	if !ok {
		t.Fatal("expected file to exist at ref")
	}
	if content != `{"stage":"build","status":"passed"}` {
		t.Fatalf("unexpected content: %q", content)
	}
}

// TestRemote_ReadFileAtRef_MissingFile verifies that missing files return false.
func TestRemote_ReadFileAtRef_MissingFile(t *testing.T) {
	remote := gittest.NewRemote(t)

	_, ok := remote.ReadFileAtRef("refs/clarity/events", "events/nobody.json")
	if ok {
		t.Fatal("expected false for missing ref/file")
	}
}

// TestPassingTestCommand_Exits0 verifies the passing command actually exits 0.
func TestPassingTestCommand_Exits0(t *testing.T) {
	cmd := commandFromString(gittest.PassingTestCommand())
	if err := cmd.Run(); err != nil {
		t.Fatalf("PassingTestCommand should exit 0, got: %v", err)
	}
}

// TestFailingTestCommand_Exits1 verifies the failing command actually exits non-zero.
func TestFailingTestCommand_Exits1(t *testing.T) {
	cmd := commandFromString(gittest.FailingTestCommand())
	if err := cmd.Run(); err == nil {
		t.Fatal("FailingTestCommand should exit non-zero")
	}
}

// TestClone_LogBranch_ReturnsCommitsInOrder verifies that LogBranch returns
// commits newest-first.
func TestClone_LogBranch_ReturnsCommitsInOrder(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	clone.WriteFile("a.txt", "a")
	clone.CommitAll("first")
	clone.WriteFile("b.txt", "b")
	clone.CommitAll("second")
	clone.WriteFile("c.txt", "c")
	clone.CommitAll("third")

	commits := clone.LogBranch("main")
	if len(commits) < 3 {
		t.Fatalf("expected at least 3 commits, got %d", len(commits))
	}
	if commits[0].Message != "third" || commits[1].Message != "second" || commits[2].Message != "first" {
		t.Fatalf("unexpected commit order: %v", commits)
	}
}
