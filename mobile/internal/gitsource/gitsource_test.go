package gitsource_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/gittest"
	"github.com/ezcdlabs/clarity/mobile/internal/gitsource"
	"github.com/go-git/go-git/v5/storage/memory"
)

// seed builds a remote with n commits on main and a deploy event on the tip.
func seed(t *testing.T, n int) *gittest.Remote {
	t.Helper()
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	for i := 0; i < n; i++ {
		// Distinct content per commit: git refuses an empty one, so identical
		// bodies would silently produce a one-commit history.
		clone.WriteFile("f.txt", fmt.Sprintf("rev %d", i))
		clone.CommitAll(fmt.Sprintf("feat: change %d", i))
	}
	clone.Push("main")

	head := clone.LogBranch("main")[0].Hash
	ev := clarityrefs.Event{Stage: "deploy", Status: "passed", Time: time.Unix(1744120134, 0)}
	if err := clarityrefs.WriteEvent(clone.Path, "origin", head, ev); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	return remote
}

func open(t *testing.T, remote *gittest.Remote) *gitsource.Repo {
	t.Helper()
	r, err := gitsource.Open(memory.NewStorage(), remote.URL(), "main")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return r
}

// TestSync_FetchesWithoutACheckout is the whole premise: a client with no
// working tree reads a repository it fetched itself.
func TestSync_FetchesWithoutACheckout(t *testing.T) {
	remote := seed(t, 5)
	r := open(t, remote)

	if err := r.Sync(context.Background(), gitsource.SyncOptions{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	snap, err := r.Snapshot(50)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snap.Commits) == 0 {
		t.Fatal("no commits read")
	}
	// The event must have been joined to its commit, which is the point of
	// fetching the events ref at all.
	var withEvents int
	for _, c := range snap.Commits {
		if len(c.Events) > 0 {
			withEvents++
		}
	}
	if withEvents != 1 {
		t.Errorf("expected exactly one commit carrying events, got %d", withEvents)
	}
}

// TestSync_ShallowDepthStopsAtTheGraft covers the failure that made the CLI
// abandon go-git for reading.
//
// A shallow fetch leaves the oldest commit claiming parents that were never
// downloaded. go-git does not read that boundary: Log walks through it and
// fails the entire iteration with "object not found". The difference here is
// that the client chose the depth, and go-git records the boundary even
// though it ignores it — so the walk can stop where the history actually
// stops.
func TestSync_ShallowDepthStopsAtTheGraft(t *testing.T) {
	remote := seed(t, 20)
	r := open(t, remote)

	if err := r.Sync(context.Background(), gitsource.SyncOptions{Depth: 5}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	// A limit far past the depth, so nothing but the graft can stop the walk.
	snap, err := r.Snapshot(500)
	if err != nil {
		t.Fatalf("Snapshot: %v — the walk ran past the shallow boundary", err)
	}
	if len(snap.Commits) == 0 || len(snap.Commits) > 6 {
		t.Errorf("got %d commits, want about the 5 that were fetched", len(snap.Commits))
	}
	if !snap.Truncated {
		t.Error("a depth-limited window does not hold the whole history, and " +
			"must say so — an aggregate built from it is otherwise read as complete")
	}
}

// TestSnapshot_LimitTruncatesBeforeTheGraft covers the other reason a window
// ends: the caller asked for fewer commits than were fetched.
func TestSnapshot_LimitTruncatesBeforeTheGraft(t *testing.T) {
	remote := seed(t, 20)
	r := open(t, remote)
	if err := r.Sync(context.Background(), gitsource.SyncOptions{Depth: 15}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	snap, err := r.Snapshot(4)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snap.Commits) != 4 {
		t.Errorf("got %d commits, want 4", len(snap.Commits))
	}
	if !snap.Truncated {
		t.Error("the limit cut the history; Truncated should say so")
	}
}

// TestConfig_ReadsFromTheTreeNotAWorkingCopy verifies .ezcd.json is read as a
// blob. A client that could not reach it would lose declared flows and the
// lead time mode.
func TestConfig_ReadsFromTheTreeNotAWorkingCopy(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile(".ezcd.json", `{"branch":"main","clarity":{"leadTime":"pipeline"}}`)
	clone.CommitAll("chore: configure clarity")
	clone.Push("main")

	r := open(t, remote)
	if err := r.Sync(context.Background(), gitsource.SyncOptions{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	cfg, err := r.Config()
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if got := string(cfg.LeadTimeMode()); got != "pipeline" {
		t.Errorf("lead time mode = %q, want pipeline — the config did not reach the client", got)
	}
}

// TestConfig_AbsentIsTheDefaults covers most repositories, which have no
// .ezcd.json at all.
func TestConfig_AbsentIsTheDefaults(t *testing.T) {
	r := open(t, seed(t, 2))
	if err := r.Sync(context.Background(), gitsource.SyncOptions{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	cfg, err := r.Config()
	if err != nil {
		t.Fatalf("a repo with no config file is not an error: %v", err)
	}
	if cfg.Branch != "main" {
		t.Errorf("branch = %q, want the default", cfg.Branch)
	}
}

// TestSync_RepoWithNoEventsRefIsNotAnError covers a repository that has never
// reported — the state of every repo before its first pipeline run.
func TestSync_RepoWithNoEventsRefIsNotAnError(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("f.txt", "x")
	clone.CommitAll("feat: first")
	clone.Push("main")

	r := open(t, remote)
	if err := r.Sync(context.Background(), gitsource.SyncOptions{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	snap, err := r.Snapshot(50)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snap.Commits) == 0 {
		t.Error("the commits should still be read without an events ref")
	}
}

// TestSync_IsRepeatable verifies a second sync over the same store succeeds —
// the app syncs on every open, not only on first add.
func TestSync_IsRepeatable(t *testing.T) {
	r := open(t, seed(t, 3))
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := r.Sync(ctx, gitsource.SyncOptions{}); err != nil {
			t.Fatalf("sync %d: %v", i+1, err)
		}
	}
	if _, err := r.Snapshot(50); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
}
