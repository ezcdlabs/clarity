package watcher_test

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/gittest"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

func fetchAll(t *testing.T, repoPath string) {
	t.Helper()
	cmd := exec.Command("git", "fetch", "origin",
		"+refs/heads/main:refs/remotes/origin/main",
		"+refs/clarity/events:refs/clarity/events",
	)
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil &&
		!strings.Contains(string(out), "couldn't find remote ref") {
		t.Fatalf("fetch all: %v\n%s", err, out)
	}
}

func TestBuildSnapshot_EmptyRepo_ReturnsInitialCommitOnly(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	fetchAll(t, clone.Path)

	snap, err := watcher.BuildSnapshot(clone.Path, "main", 50)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if len(snap.Commits) != 1 {
		t.Fatalf("expected 1 commit (the seed), got %d", len(snap.Commits))
	}
	if snap.Commits[0].Subject != "initial commit" {
		t.Errorf("expected initial commit at top, got %q", snap.Commits[0].Subject)
	}
}

func TestBuildSnapshot_ReturnsCommitsNewestFirst(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "a")
	clone.CommitAll("first")
	clone.WriteFile("b.txt", "b")
	clone.CommitAll("second")
	clone.WriteFile("c.txt", "c")
	clone.CommitAll("third")
	clone.Push("main")

	fetchAll(t, clone.Path)
	snap, err := watcher.BuildSnapshot(clone.Path, "main", 50)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if len(snap.Commits) < 3 {
		t.Fatalf("expected at least 3 commits, got %d", len(snap.Commits))
	}
	want := []string{"third", "second", "first"}
	for i, w := range want {
		if snap.Commits[i].Subject != w {
			t.Errorf("commit[%d]: expected %q, got %q", i, w, snap.Commits[i].Subject)
		}
	}
}

func TestBuildSnapshot_RespectsLimit(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	for i := 0; i < 5; i++ {
		clone.WriteFile("f.txt", strings.Repeat("x", i+1))
		clone.CommitAll("commit")
	}
	clone.Push("main")
	fetchAll(t, clone.Path)

	snap, err := watcher.BuildSnapshot(clone.Path, "main", 3)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if len(snap.Commits) != 3 {
		t.Fatalf("expected limit=3 to return 3 commits, got %d", len(snap.Commits))
	}
}

func TestBuildSnapshot_JoinsEventsToCommitsBySHA(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "a")
	clone.CommitAll("a commit")
	clone.Push("main")

	// Find HEAD SHA via git rev-parse.
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = clone.Path
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	headSHA := strings.TrimSpace(string(out))

	ev := clarityrefs.Event{
		Stage: "build", Status: "passed",
		Time: time.Unix(1744120134, 0),
	}
	if err := clarityrefs.WriteEvent(clone.Path, "origin", headSHA, ev); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}

	fetchAll(t, clone.Path)
	snap, err := watcher.BuildSnapshot(clone.Path, "main", 50)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}

	var found bool
	for _, c := range snap.Commits {
		if c.SHA == headSHA {
			found = true
			if len(c.Events) != 1 {
				t.Errorf("expected 1 event for HEAD, got %d", len(c.Events))
			} else if c.Events[0].Stage != "build" || c.Events[0].Status != "passed" {
				t.Errorf("unexpected event payload: %+v", c.Events[0])
			}
		}
	}
	if !found {
		t.Errorf("HEAD commit %s not in snapshot", headSHA)
	}
}

func TestBuildSnapshot_CommitsWithoutEvents_HaveEmptyList(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "a")
	clone.CommitAll("no events")
	clone.Push("main")
	fetchAll(t, clone.Path)

	snap, err := watcher.BuildSnapshot(clone.Path, "main", 50)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	for _, c := range snap.Commits {
		if c.Events == nil {
			continue // nil is fine
		}
		if len(c.Events) != 0 {
			t.Errorf("expected no events for %s, got %d", c.SHA, len(c.Events))
		}
	}
}
