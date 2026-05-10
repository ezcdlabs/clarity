package tui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/tui"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

func newModel(repo string) tui.Model {
	return tui.New(repo).WithSize(120, 40)
}

func TestModel_BeforeFirstSnapshot_ShowsLoading(t *testing.T) {
	m := newModel("clarity")
	out := m.View()
	if !strings.Contains(strings.ToLower(out), "loading") {
		t.Errorf("expected a loading indicator before any snapshot, got:\n%s", out)
	}
	if strings.Contains(out, "no commits yet") {
		t.Errorf("should not show 'no commits yet' before first snapshot, got:\n%s", out)
	}
}

func TestModel_AfterEmptySnapshot_ShowsNoCommitsState(t *testing.T) {
	m, _ := newModel("clarity").Update(tui.SnapshotMsg(watcher.Snapshot{}))
	out := m.View()
	if !strings.Contains(out, "no commits yet") {
		t.Errorf("expected 'no commits yet' after an empty snapshot, got:\n%s", out)
	}
	if strings.Contains(strings.ToLower(out), "loading") {
		t.Errorf("should no longer show loading once a snapshot has arrived, got:\n%s", out)
	}
}

func TestModel_AfterPopulatedSnapshot_RendersCommits(t *testing.T) {
	snap := watcher.Snapshot{
		Commits: []watcher.CommitView{
			{SHA: "1", Author: "alice", Subject: "first commit",
				Events: []clarityrefs.Event{{Stage: "ci", Status: "passed", Time: time.Unix(100, 0)}}},
		},
	}
	m, _ := newModel("clarity").Update(tui.SnapshotMsg(snap))
	out := m.View()
	if !strings.Contains(out, "alice") {
		t.Errorf("expected commit author rendered, got:\n%s", out)
	}
	if !strings.Contains(out, "first commit") {
		t.Errorf("expected commit subject rendered, got:\n%s", out)
	}
}

// The header shows the repository name (not the branch).
func TestModel_HeaderShowsRepoName(t *testing.T) {
	m := newModel("my-cool-repo")
	out := m.View()
	if !strings.Contains(out, "my-cool-repo") {
		t.Errorf("expected repo name in header, got:\n%s", out)
	}
}

// The header shows pipeline status badges for CI and deploy.
func TestModel_HeaderShowsCIAndDeployBadges(t *testing.T) {
	snap := watcher.Snapshot{
		Commits: []watcher.CommitView{
			{SHA: "1", Author: "alice", Subject: "x", Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: time.Unix(100, 0)},
				{Stage: "deploy", Status: "passed", Time: time.Unix(150, 0)},
			}},
		},
	}
	m, _ := newModel("clarity").Update(tui.SnapshotMsg(snap))
	out := m.View()
	if !strings.Contains(strings.ToLower(out), "ci") {
		t.Errorf("expected 'ci' badge in header, got:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "deploy") {
		t.Errorf("expected 'deploy' badge in header, got:\n%s", out)
	}
}

// The "q to quit" hint sits in the top-right of the header line.
func TestModel_QuitHint_OnHeaderLine_RightAligned(t *testing.T) {
	m := newModel("clarity")
	out := m.View()
	if !strings.Contains(strings.ToLower(out), "q") {
		t.Errorf("expected quit hint, got:\n%s", out)
	}
	// Quit hint must appear on the first line (the header), not below.
	firstLine := strings.SplitN(out, "\n", 2)[0]
	if !strings.Contains(strings.ToLower(firstLine), "q") {
		t.Errorf("expected quit hint on the header line, got first line:\n%q", firstLine)
	}
}
