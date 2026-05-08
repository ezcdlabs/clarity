package tui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/tui"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

func ev(stage, status string, ts int64) clarityrefs.Event {
	return clarityrefs.Event{Stage: stage, Status: status, Time: time.Unix(ts, 0)}
}

// --- OverallStatus -----------------------------------------------------------

func TestOverallStatus_TableDriven(t *testing.T) {
	cases := []struct {
		name   string
		events []clarityrefs.Event
		want   string
	}{
		{
			name:   "no events",
			events: nil,
			want:   "none",
		},
		{
			name:   "single passed",
			events: []clarityrefs.Event{ev("build", "passed", 100)},
			want:   "passed",
		},
		{
			name:   "single failed",
			events: []clarityrefs.Event{ev("build", "failed", 100)},
			want:   "failed",
		},
		{
			name:   "single started — running",
			events: []clarityrefs.Event{ev("build", "started", 100)},
			want:   "running",
		},
		{
			name: "all passed",
			events: []clarityrefs.Event{
				ev("build", "passed", 100), ev("deploy", "passed", 200),
			},
			want: "passed",
		},
		{
			name: "any failed beats running",
			events: []clarityrefs.Event{
				ev("build", "failed", 100), ev("deploy", "started", 200),
			},
			want: "failed",
		},
		{
			name: "running beats not-yet-completed others",
			events: []clarityrefs.Event{
				ev("build", "passed", 100), ev("deploy", "started", 200),
			},
			want: "running",
		},
		{
			name: "skipped counts as terminal-ok",
			events: []clarityrefs.Event{
				ev("build", "passed", 100), ev("deploy", "skipped", 200),
			},
			want: "passed",
		},
		{
			name: "later passed overrides earlier failed (collapse)",
			events: []clarityrefs.Event{
				ev("build", "failed", 100), ev("build", "passed", 200),
			},
			want: "passed",
		},
		{
			name: "later started overrides earlier passed (retry)",
			events: []clarityrefs.Event{
				ev("build", "passed", 100), ev("build", "started", 200),
			},
			want: "running",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := tui.OverallStatus(c.events)
			if got != c.want {
				t.Errorf("OverallStatus(%v) = %q, want %q", c.events, got, c.want)
			}
		})
	}
}

// --- CollapseStages ----------------------------------------------------------

func TestCollapseStages_LatestEventPerStage(t *testing.T) {
	events := []clarityrefs.Event{
		ev("build", "started", 100),
		ev("build", "passed", 200),
		ev("deploy", "started", 300),
	}
	got := tui.CollapseStages(events)
	if len(got) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(got))
	}

	byStage := map[string]string{}
	for _, s := range got {
		byStage[s.Stage] = s.Status
	}
	if byStage["build"] != "passed" {
		t.Errorf("build: expected passed (latest), got %q", byStage["build"])
	}
	if byStage["deploy"] != "started" {
		t.Errorf("deploy: expected started, got %q", byStage["deploy"])
	}
}

func TestCollapseStages_PreservesEventOrder(t *testing.T) {
	// build seen first, deploy seen later → returned in chronological order.
	events := []clarityrefs.Event{
		ev("build", "passed", 100),
		ev("deploy", "passed", 200),
	}
	got := tui.CollapseStages(events)
	if got[0].Stage != "build" || got[1].Stage != "deploy" {
		t.Errorf("expected [build, deploy] order, got %v", got)
	}
}

// --- RenderRow ---------------------------------------------------------------

func TestRenderRow_ShowsAuthorAndSubject(t *testing.T) {
	view := watcher.CommitView{
		SHA: "abc", Author: "alice", Subject: "refactor user model",
	}
	out := tui.RenderRow(view, 80)
	if !strings.Contains(out, "alice") {
		t.Errorf("expected author in row: %q", out)
	}
	if !strings.Contains(out, "refactor user model") {
		t.Errorf("expected subject in row: %q", out)
	}
}

func TestRenderRow_NoEvents_ShowsNoEventsMarker(t *testing.T) {
	view := watcher.CommitView{
		SHA: "abc", Author: "grace", Subject: "wip",
	}
	out := tui.RenderRow(view, 80)
	if !strings.Contains(out, "no events") {
		t.Errorf("expected 'no events' marker, got: %q", out)
	}
}

func TestRenderRow_PassedShowsCheckmark(t *testing.T) {
	view := watcher.CommitView{
		SHA: "abc", Author: "alice", Subject: "x",
		Events: []clarityrefs.Event{ev("build", "passed", 100)},
	}
	out := tui.RenderRow(view, 80)
	if !strings.Contains(out, "✓") {
		t.Errorf("expected ✓ for passed: %q", out)
	}
}

func TestRenderRow_FailedShowsCross(t *testing.T) {
	view := watcher.CommitView{
		SHA: "abc", Author: "eve", Subject: "x",
		Events: []clarityrefs.Event{ev("build", "failed", 100)},
	}
	out := tui.RenderRow(view, 80)
	if !strings.Contains(out, "✗") {
		t.Errorf("expected ✗ for failed: %q", out)
	}
}

func TestRenderRow_RunningShowsHourglass(t *testing.T) {
	view := watcher.CommitView{
		SHA: "abc", Author: "dave", Subject: "x",
		Events: []clarityrefs.Event{ev("build", "started", 100)},
	}
	out := tui.RenderRow(view, 80)
	if !strings.Contains(out, "⧗") {
		t.Errorf("expected ⧗ for running: %q", out)
	}
}

func TestRenderRow_StagesListed(t *testing.T) {
	view := watcher.CommitView{
		SHA: "abc", Author: "alice", Subject: "x",
		Events: []clarityrefs.Event{
			ev("build", "passed", 100),
			ev("deploy", "passed", 200),
		},
	}
	out := tui.RenderRow(view, 80)
	if !strings.Contains(out, "build") {
		t.Errorf("expected stage 'build' in row: %q", out)
	}
	if !strings.Contains(out, "deploy") {
		t.Errorf("expected stage 'deploy' in row: %q", out)
	}
}

// --- RenderSnapshot ----------------------------------------------------------

func TestRenderSnapshot_OneRowPerCommit(t *testing.T) {
	snap := watcher.Snapshot{
		Commits: []watcher.CommitView{
			{SHA: "1", Author: "alice", Subject: "first"},
			{SHA: "2", Author: "bob", Subject: "second"},
			{SHA: "3", Author: "carol", Subject: "third"},
		},
	}
	out := tui.RenderSnapshot(snap, 80, time.Time{})
	for _, want := range []string{"alice", "bob", "carol", "first", "second", "third"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
}

func TestRenderSnapshot_EmptyCommits_ProducesNonEmptyOutput(t *testing.T) {
	out := tui.RenderSnapshot(watcher.Snapshot{}, 80, time.Time{})
	if out == "" {
		t.Error("expected some placeholder output for empty snapshot")
	}
}

// --- section rendering -------------------------------------------------------

// All commits with no events sit in NeedsCI, so only that section header should appear.
func TestRenderSnapshot_RendersOnlyNonEmptySections(t *testing.T) {
	snap := watcher.Snapshot{
		Commits: []watcher.CommitView{
			{SHA: "1", Author: "alice", Subject: "wip"},
		},
	}
	out := tui.RenderSnapshot(snap, 80, time.Time{})
	if !strings.Contains(out, "On main") {
		t.Errorf("expected 'On main' header for a NeedsCI commit, got:\n%s", out)
	}
	if strings.Contains(out, "Next deploy") {
		t.Errorf("did not expect 'Next deploy' header for an empty group, got:\n%s", out)
	}
	if strings.Contains(out, "Deployed to production") {
		t.Errorf("did not expect 'Deployed to production' header for an empty group, got:\n%s", out)
	}
}

// A snapshot covering all three lifecycle stages should produce all three section headers.
func TestRenderSnapshot_AllThreeSections(t *testing.T) {
	snap := watcher.Snapshot{
		Commits: []watcher.CommitView{
			{SHA: "d", Author: "dave", Subject: "broken"},                                        // NeedsCI
			{SHA: "c", Author: "carol", Subject: "built", Events: []clarityrefs.Event{ev("build", "passed", 200)}}, // NextDeploy
			{SHA: "b", Author: "bob", Subject: "shipped", Events: []clarityrefs.Event{
				ev("build", "passed", 100), ev("deploy", "passed", 150),
			}},                                                                                   // Deployed
		},
	}
	out := tui.RenderSnapshot(snap, 80, time.Time{})
	for _, h := range []string{"On main", "Next deploy", "Deployed to production"} {
		if !strings.Contains(out, h) {
			t.Errorf("expected section header %q in output:\n%s", h, out)
		}
	}
	for _, name := range []string{"dave", "carol", "bob"} {
		if !strings.Contains(out, name) {
			t.Errorf("expected commit author %q in output:\n%s", name, out)
		}
	}
}

// While a deploy is in flight the Next deploy header is annotated.
func TestRenderSnapshot_DeployingAnnotatesHeader(t *testing.T) {
	snap := watcher.Snapshot{
		Commits: []watcher.CommitView{
			{SHA: "b", Author: "bob", Subject: "shipping", Events: []clarityrefs.Event{
				ev("build", "passed", 100), ev("deploy", "started", 200),
			}},
			{SHA: "a", Author: "alice", Subject: "live", Events: []clarityrefs.Event{
				ev("build", "passed", 50), ev("deploy", "passed", 75),
			}},
		},
	}
	out := tui.RenderSnapshot(snap, 80, time.Time{})
	if !strings.Contains(strings.ToLower(out), "deploying") {
		t.Errorf("expected 'deploying' indicator on Next deploy header, got:\n%s", out)
	}
}

// A live (not-yet-deployed) commit shows its lead-time timer ticking against now.
func TestRenderSnapshot_LiveCommit_RendersTickingLeadTime(t *testing.T) {
	commitTime := time.Unix(1000, 0)
	now := time.Unix(1090, 0) // 1m 30s later
	snap := watcher.Snapshot{
		Commits: []watcher.CommitView{
			{SHA: "a", Author: "alice", Subject: "wip", Time: commitTime,
				Events: []clarityrefs.Event{ev("build", "started", 50)}},
		},
	}
	out := tui.RenderSnapshot(snap, 80, now)
	if !strings.Contains(out, "1m 30s") {
		t.Errorf("expected '1m 30s' lead time in output, got:\n%s", out)
	}
}

// A deployed commit shows its lead time frozen at the deploy moment, not now.
func TestRenderSnapshot_DeployedCommit_FrozenLeadTime(t *testing.T) {
	commitTime := time.Unix(1000, 0)
	deployTime := time.Unix(1300, 0) // exactly 5m after commit
	now := time.Unix(9999, 0)        // long after — must NOT influence frozen value
	snap := watcher.Snapshot{
		Commits: []watcher.CommitView{
			{SHA: "a", Author: "alice", Subject: "shipped", Time: commitTime,
				Events: []clarityrefs.Event{
					ev("build", "passed", 100),
					{Stage: "deploy", Status: "passed", Time: deployTime},
				}},
		},
	}
	out := tui.RenderSnapshot(snap, 80, now)
	if !strings.Contains(out, "5m 00s") {
		t.Errorf("expected '5m 00s' frozen lead time, got:\n%s", out)
	}
}

// Sections appear top-to-bottom in lifecycle order: NeedsCI → NextDeploy → Deployed.
func TestRenderSnapshot_SectionsInLifecycleOrder(t *testing.T) {
	snap := watcher.Snapshot{
		Commits: []watcher.CommitView{
			{SHA: "d", Author: "dave", Subject: "broken"},
			{SHA: "c", Author: "carol", Subject: "built", Events: []clarityrefs.Event{ev("build", "passed", 200)}},
			{SHA: "b", Author: "bob", Subject: "shipped", Events: []clarityrefs.Event{
				ev("build", "passed", 100), ev("deploy", "passed", 150),
			}},
		},
	}
	out := tui.RenderSnapshot(snap, 80, time.Time{})
	pNeedsCI := strings.Index(out, "On main")
	pNextDeploy := strings.Index(out, "Next deploy")
	pDeployed := strings.Index(out, "Deployed to production")
	if !(pNeedsCI < pNextDeploy && pNextDeploy < pDeployed) {
		t.Errorf("expected sections in order NeedsCI → NextDeploy → Deployed, got positions %d, %d, %d in:\n%s",
			pNeedsCI, pNextDeploy, pDeployed, out)
	}
}
