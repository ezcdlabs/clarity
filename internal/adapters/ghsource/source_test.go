package ghsource_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/internal/adapters/ghsource"
	"github.com/ezcdlabs/clarity/internal/cache"
	"github.com/ezcdlabs/clarity/internal/clock"
	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/core"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// fakeGHClient is a programmable GHClient: each ListRuns call pulls the
// next scripted response off the queue. Lets tests script a sequence
// of poll outcomes (initial fetch, then a follow-up fetch with new
// runs) without spinning up gh on PATH.
type fakeGHClient struct {
	mu          sync.Mutex
	scripts     map[string][][]ghsource.Run // workflow name → per-call returns
	calls       map[string]int
	workflows   []ghsource.WorkflowSummary
	workflowErr error
	runsErr     error // when set, ListRuns returns it
}

func newFakeGHClient() *fakeGHClient {
	return &fakeGHClient{
		scripts: map[string][][]ghsource.Run{},
		calls:   map[string]int{},
		// Tests that don't customise this still get a sensible default
		// so Source.Validate doesn't fail on a missing workflow.
		workflows: []ghsource.WorkflowSummary{
			{ID: 1, Name: "CI", Path: ".github/workflows/ci.yml"},
		},
	}
}

func (f *fakeGHClient) script(workflow string, returns ...[]ghsource.Run) {
	f.scripts[workflow] = append(f.scripts[workflow], returns...)
}

func (f *fakeGHClient) ListWorkflows() ([]ghsource.WorkflowSummary, error) {
	return f.workflows, f.workflowErr
}

func (f *fakeGHClient) ListRuns(workflowName, branch string, since time.Time) ([]ghsource.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.runsErr != nil {
		return nil, f.runsErr
	}
	idx := f.calls[workflowName]
	queue := f.scripts[workflowName]
	if idx >= len(queue) {
		return nil, nil
	}
	out := queue[idx]
	f.calls[workflowName]++
	return out, nil
}

// Compile-time check: the adapter satisfies the core.Source port.
var _ core.Source = (*ghsource.Source)(nil)

// TestSource_Watch_EmitsInitialSnapshot_WithDerivedEvents is the
// load-bearing end-to-end: a configured GH mapping + a scripted client
// returning one CI-passed run produces a Snapshot whose joined commit
// view carries the derived ci:passed event. Verifies the wiring:
// gitlog walk + DeriveEvents + core.BuildSnapshot + RepoName stamp.
func TestSource_Watch_EmitsInitialSnapshot_WithDerivedEvents(t *testing.T) {
	// A repo + commit on main, then fetch the remote-tracking ref so
	// gitlog.Walk resolves origin/main.
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "a")
	clone.CommitAll("a commit")
	clone.Push("main")
	headSHA := headSHA(t, clone.Path)
	fetchAll(t, clone.Path)

	fake := newFakeGHClient()
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute)
	fake.script("CI", []ghsource.Run{{
		ID: 1, HeadSHA: headSHA, Workflow: "CI",
		UpdatedAt: end,
		Jobs: []ghsource.Job{
			{Name: "Test", Status: "completed", Conclusion: "success", StartedAt: start, CompletedAt: end},
		},
	}})

	cfg := &config.GitHubConfig{
		CI: stageMapping("CI", "Test"),
	}
	cf := cache.New(filepath.Join(t.TempDir(), "github-runs.json.gz"))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	src, err := ghsource.New(ghsource.Options{
		RepoPath: clone.Path,
		RepoName: "myrepo",
		Branch:   "main",
		Limit:    50,
		Mapping:  cfg,
		Cache:    cf,
		Client:   fake,
		Interval: testInterval,
		Clock:    clock.NewFake(),
	})
	if err != nil {
		t.Fatalf("ghsource.New: %v", err)
	}

	snap := waitForSnapshot(t, src.Watch(ctx))
	if snap.RepoName != "myrepo" {
		t.Errorf("RepoName: expected myrepo, got %q", snap.RepoName)
	}
	var saw bool
	for _, c := range snap.Commits {
		if c.SHA == headSHA {
			for _, e := range c.Events {
				if e.Stage == "ci" && e.Status == "passed" {
					saw = true
				}
			}
		}
	}
	if !saw {
		t.Errorf("expected ci:passed event on HEAD SHA in the snapshot, got %+v", snap)
	}
}

// TestSource_Watch_ContextCancel_ClosesChannel: every Source must close
// its Snapshots channel on ctx cancellation so the Lens upstream can
// shut down cleanly.
func TestSource_Watch_ContextCancel_ClosesChannel(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	fetchAll(t, clone.Path)

	src, err := ghsource.New(ghsource.Options{
		RepoPath: clone.Path,
		Branch:   "main",
		Limit:    50,
		Mapping:  &config.GitHubConfig{CI: stageMapping("CI", "Test")},
		Cache:    cache.New(filepath.Join(t.TempDir(), "github-runs.json.gz")),
		Client:   newFakeGHClient(),
		Interval: testInterval,
		Clock:    clock.NewFake(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch := src.Watch(ctx)
	waitForSnapshot(t, ch) // drain the initial emit
	cancel()

	deadline := time.After(3 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("channel did not close after context cancel")
		}
	}
}

// --- helpers ---------------------------------------------------------

const testInterval = 30 * time.Second

func stageMapping(workflow string, jobNames ...string) *config.StageMapping {
	var js config.JobSet
	data, _ := json.Marshal(jobNames)
	if err := js.UnmarshalJSON(data); err != nil {
		panic(err)
	}
	return &config.StageMapping{Workflow: workflow, Jobs: js}
}

func waitForSnapshot(t *testing.T, ch <-chan core.Snapshot) core.Snapshot {
	t.Helper()
	select {
	case snap := <-ch:
		return snap
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for snapshot")
		return core.Snapshot{}
	}
}

func headSHA(t *testing.T, repoPath string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func fetchAll(t *testing.T, repoPath string) {
	t.Helper()
	cmd := exec.Command("git", "fetch", "origin",
		"+refs/heads/main:refs/remotes/origin/main")
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil &&
		!strings.Contains(string(out), "couldn't find remote ref") {
		t.Fatalf("fetch: %v\n%s", err, out)
	}
}
