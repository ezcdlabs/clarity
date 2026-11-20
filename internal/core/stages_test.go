package core_test

import (
	"testing"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
)

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
			events: []clarityrefs.Event{ev("ci", "passed", 100)},
			want:   "passed",
		},
		{
			name:   "single failed",
			events: []clarityrefs.Event{ev("ci", "failed", 100)},
			want:   "failed",
		},
		{
			name:   "single started — running",
			events: []clarityrefs.Event{ev("ci", "started", 100)},
			want:   "running",
		},
		{
			name: "all passed",
			events: []clarityrefs.Event{
				ev("ci", "passed", 100), ev("deploy", "passed", 200),
			},
			want: "passed",
		},
		{
			name: "any failed beats running",
			events: []clarityrefs.Event{
				ev("ci", "failed", 100), ev("deploy", "started", 200),
			},
			want: "failed",
		},
		{
			name: "running beats not-yet-completed others",
			events: []clarityrefs.Event{
				ev("ci", "passed", 100), ev("deploy", "started", 200),
			},
			want: "running",
		},
		{
			name: "skipped counts as terminal-ok",
			events: []clarityrefs.Event{
				ev("ci", "passed", 100), ev("deploy", "skipped", 200),
			},
			want: "passed",
		},
		{
			name: "noop counts as terminal-ok — delivery completed, nothing shipped",
			events: []clarityrefs.Event{
				ev("ci", "passed", 100), ev("deploy", "noop", 200),
			},
			want: "passed",
		},
		{
			name: "later passed overrides earlier failed (collapse)",
			events: []clarityrefs.Event{
				ev("ci", "failed", 100), ev("ci", "passed", 200),
			},
			want: "passed",
		},
		{
			name: "later started overrides earlier passed (retry)",
			events: []clarityrefs.Event{
				ev("ci", "passed", 100), ev("ci", "started", 200),
			},
			want: "running",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := core.OverallStatus(c.events)
			if got != c.want {
				t.Errorf("OverallStatus(%v) = %q, want %q", c.events, got, c.want)
			}
		})
	}
}

// --- CollapseStages ----------------------------------------------------------

func TestCollapseStages_LatestEventPerStage(t *testing.T) {
	events := []clarityrefs.Event{
		ev("ci", "started", 100),
		ev("ci", "passed", 200),
		ev("deploy", "started", 300),
	}
	got := core.CollapseStages(events)
	if len(got) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(got))
	}

	byStage := map[string]string{}
	for _, s := range got {
		byStage[s.Stage] = s.Status
	}
	if byStage["ci"] != "passed" {
		t.Errorf("ci: expected passed (latest), got %q", byStage["ci"])
	}
	if byStage["deploy"] != "started" {
		t.Errorf("deploy: expected started, got %q", byStage["deploy"])
	}
}

func TestCollapseStages_PreservesEventOrder(t *testing.T) {
	// ci seen first, deploy seen later → returned in chronological order.
	events := []clarityrefs.Event{
		ev("ci", "passed", 100),
		ev("deploy", "passed", 200),
	}
	got := core.CollapseStages(events)
	if got[0].Stage != "ci" || got[1].Stage != "deploy" {
		t.Errorf("expected [ci, deploy] order, got %v", got)
	}
}

// --- CurrentStageStatus ------------------------------------------------------

// Commits are passed newest-first, matching Snapshot's documented ordering.
// cv (shared with groupings_test.go) builds a CommitView from a SHA + events.

func TestCurrentStageStatus_TableDriven(t *testing.T) {
	cases := []struct {
		name    string
		commits []core.CommitView
		stage   string
		want    string
	}{
		{
			name:    "no commits",
			commits: nil,
			stage:   "ci",
			want:    "",
		},
		{
			name:    "commits with no events",
			commits: []core.CommitView{cv("head"), cv("older")},
			stage:   "ci",
			want:    "",
		},
		{
			name:    "only unresolved events — no badge",
			commits: []core.CommitView{cv("head", ev("ci", "started", 100))},
			stage:   "ci",
			want:    "",
		},
		{
			name:    "skipped is not a resolution",
			commits: []core.CommitView{cv("head", ev("deploy", "skipped", 100))},
			stage:   "deploy",
			want:    "",
		},
		{
			name:    "noop is a resolution, and the badge reads it as green",
			commits: []core.CommitView{cv("head", ev("deploy", "noop", 100))},
			stage:   "deploy",
			want:    "passed",
		},
		{
			name: "noop supersedes an older failed deploy — production converged",
			commits: []core.CommitView{
				cv("head", ev("deploy", "noop", 300)),
				cv("older", ev("deploy", "failed", 100)),
			},
			stage: "deploy",
			want:  "passed",
		},
		{
			name:    "other stages ignored",
			commits: []core.CommitView{cv("head", ev("deploy", "failed", 100))},
			stage:   "ci",
			want:    "",
		},
		{
			name:    "retry on the same commit — latest resolution wins",
			commits: []core.CommitView{cv("head", ev("ci", "failed", 100), ev("ci", "passed", 200))},
			stage:   "ci",
			want:    "passed",
		},
		{
			name:    "restart on the same commit holds the last colour",
			commits: []core.CommitView{cv("head", ev("ci", "passed", 100), ev("ci", "started", 200))},
			stage:   "ci",
			want:    "passed",
		},
		{
			name: "HEAD silent on the stage — falls back to newest commit that isn't",
			commits: []core.CommitView{
				cv("head"),
				cv("older", ev("ci", "passed", 100)),
			},
			stage: "ci",
			want:  "passed",
		},
		{
			name: "HEAD still running — holds the older commit's colour",
			commits: []core.CommitView{
				cv("head", ev("ci", "started", 300)),
				cv("older", ev("ci", "passed", 100)),
			},
			stage: "ci",
			want:  "passed",
		},
		{
			name: "newest commit wins even when an older commit resolved later",
			// The overtaking-push race: a bad commit's CI is still in flight
			// when its revert is pushed, and the revert's CI reports green
			// three seconds before the bad commit reports red. The newest
			// event belongs to a commit that is no longer HEAD.
			commits: []core.CommitView{
				cv("c51e367", ev("ci", "passed", 1787654768), ev("deploy", "passed", 1787654800)),
				cv("37ea7bb", ev("ci", "failed", 1787654771)),
			},
			stage: "ci",
			want:  "passed",
		},
		{
			name: "the reverted commit's own failure is still the older commit's truth",
			commits: []core.CommitView{
				cv("37ea7bb", ev("ci", "failed", 1787654771)),
				cv("earlier", ev("ci", "passed", 1787654000)),
			},
			stage: "ci",
			want:  "failed",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := core.CurrentStageStatus(c.commits, c.stage)
			if got != c.want {
				t.Errorf("CurrentStageStatus(%q) = %q, want %q", c.stage, got, c.want)
			}
		})
	}
}

// CurrentStage keeps the honest status: a no-op resolves the deploy stage —
// the pipeline ran to a verdict — and the resolution says which verdict.
func TestCurrentStage_NoopResolvesAsItself(t *testing.T) {
	res, found := core.CurrentStage(
		[]core.CommitView{cv("head", ev("deploy", "noop", 100))}, "deploy")
	if !found {
		t.Fatal("a no-op did not resolve the deploy stage")
	}
	if res.Status != "noop" {
		t.Errorf("resolution status = %q, want noop", res.Status)
	}
}

// CurrentStageStatus is the badge projection, and the badge answers "is
// production current?" — after a no-op it is, so the badge reads green
// exactly as it does after a passed deploy. The per-batch subheader is where
// "nothing shipped" is said.
func TestCurrentStageStatus_NoopReadsAsPassed(t *testing.T) {
	got := core.CurrentStageStatus(
		[]core.CommitView{cv("head", ev("deploy", "noop", 100))}, "deploy")
	if got != "passed" {
		t.Errorf("badge for a no-op = %q, want passed", got)
	}
}
