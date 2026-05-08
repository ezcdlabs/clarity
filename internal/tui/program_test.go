package tui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/tui"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

func TestModel_BeforeFirstSnapshot_ShowsLoading(t *testing.T) {
	m := tui.New("main")
	out := m.View()
	if !strings.Contains(strings.ToLower(out), "loading") {
		t.Errorf("expected a loading indicator before any snapshot, got:\n%s", out)
	}
	if strings.Contains(out, "no commits yet") {
		t.Errorf("should not show 'no commits yet' before first snapshot, got:\n%s", out)
	}
}

func TestModel_AfterEmptySnapshot_ShowsNoCommitsState(t *testing.T) {
	m, _ := tui.New("main").Update(tui.SnapshotMsg(watcher.Snapshot{}))
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
				Events: []clarityrefs.Event{{Stage: "build", Status: "passed", Time: time.Unix(100, 0)}}},
		},
	}
	m, _ := tui.New("main").Update(tui.SnapshotMsg(snap))
	out := m.View()
	if !strings.Contains(out, "alice") {
		t.Errorf("expected commit author rendered, got:\n%s", out)
	}
	if !strings.Contains(out, "first commit") {
		t.Errorf("expected commit subject rendered, got:\n%s", out)
	}
}

func TestModel_HeaderShowsBranch(t *testing.T) {
	m := tui.New("trunk")
	out := m.View()
	if !strings.Contains(out, "trunk") {
		t.Errorf("expected branch name in header, got:\n%s", out)
	}
}

func TestModel_FooterShowsQuitHint(t *testing.T) {
	m := tui.New("main")
	out := m.View()
	if !strings.Contains(strings.ToLower(out), "q") {
		t.Errorf("expected quit hint mentioning 'q' somewhere, got:\n%s", out)
	}
}
