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
