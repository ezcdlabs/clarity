package core_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
)

// batchPush builds the scenario the lead-time modes exist to handle: five
// commits authored on Friday afternoon and pushed together on Monday morning.
//
// CI runs on the pushed head, not on every commit in the push, so only the
// newest commit carries events. The four behind it have none — but they still
// inherit a deploy time by fix-forward, because the deploy that shipped the
// head shipped them too.
func batchPush() core.Snapshot {
	friday := utc(2026, 1, 2, 17) // authored Friday 17:00
	monday := utc(2026, 1, 5, 9)  // pushed Monday 09:00, CI starts
	deploy := utc(2026, 1, 5, 10) // deployed Monday 10:00

	head := core.CommitView{
		SHA:  "head",
		Time: friday,
		Events: []clarityrefs.Event{
			{Stage: "ci", Status: "started", Time: monday},
			{Stage: "ci", Status: "passed", Time: monday.Add(30 * time.Minute)},
			{Stage: "deploy", Status: "passed", Time: deploy},
		},
	}
	commits := []core.CommitView{head}
	for i := range 4 {
		commits = append(commits, core.CommitView{
			SHA:  "behind",
			Time: friday.Add(-time.Duration(i+1) * time.Minute),
		})
	}
	return core.Snapshot{Commits: commits}
}

// TestLeadTimeModes_BatchedPush is the acceptance test for all three modes,
// run against the same batched-push data so the difference between them is the
// only thing on show.
//
// It is also the evidence for why "exclude commits without events" alone
// doesn't do what it looks like it does: it drops four of the five samples,
// but the survivor is still the head commit authored on Friday, so the
// weekend is still in the average. Only moving the start point removes it.
func TestLeadTimeModes_BatchedPush(t *testing.T) {
	snap := batchPush()
	now := utc(2026, 1, 5, 12)

	cases := []struct {
		mode core.LeadTimeMode
		// how many commits show a lead time at all
		wantRows int
		// the head commit's lead time
		wantHead time.Duration
		// the week's average across every contributing commit
		wantAvg time.Duration
	}{
		{
			// Every commit contributes, each from its own authoring time, so
			// the weekend is counted five times over.
			mode:     core.LeadAll,
			wantRows: 5,
			wantHead: 65 * time.Hour, // Fri 17:00 → Mon 10:00
			// 65h, 65h1m, 65h2m, 65h3m, 65h4m → mean 65h2m.
			wantAvg: 65*time.Hour + 2*time.Minute,
		},
		{
			// Only the head contributes — but still from Friday, so the
			// average barely moves.
			mode:     core.LeadReported,
			wantRows: 1,
			wantHead: 65 * time.Hour,
			wantAvg:  65 * time.Hour,
		},
		{
			// Only the head, timed from the moment the pipeline picked it
			// up. The weekend is gone.
			mode:     core.LeadPipeline,
			wantRows: 1,
			wantHead: time.Hour, // Mon 09:00 → Mon 10:00
			wantAvg:  time.Hour,
		},
	}

	for _, c := range cases {
		t.Run(string(c.mode), func(t *testing.T) {
			view := core.DeriveView(snap, c.mode)

			rows := 0
			for i := range snap.Commits {
				if _, _, ok := view.Groups.LeadTime(i, now); ok {
					rows++
				}
			}
			if rows != c.wantRows {
				t.Errorf("commits showing a lead time = %d, want %d", rows, c.wantRows)
			}

			got, frozen, ok := view.Groups.LeadTime(0, now)
			if !ok {
				t.Fatal("the head commit must always have a lead time — it has events and a deploy")
			}
			if !frozen {
				t.Error("expected the head's lead time to be frozen by its deploy")
			}
			if got != c.wantHead {
				t.Errorf("head lead time = %v, want %v", got, c.wantHead)
			}

			if len(view.Weekly) != 1 {
				t.Fatalf("expected 1 week bucket, got %d", len(view.Weekly))
			}
			if view.Weekly[0].AvgLead != c.wantAvg {
				t.Errorf("weekly average = %v, want %v", view.Weekly[0].AvgLead, c.wantAvg)
			}
		})
	}
}

// TestLeadTimeModes_UnreportedCommitsStillRender checks the commits dropped
// from the metric are dropped from the metric only. They stay in the log —
// the point is to stop them skewing the average, not to hide that they
// shipped.
func TestLeadTimeModes_UnreportedCommitsStillRender(t *testing.T) {
	snap := batchPush()

	for _, mode := range []core.LeadTimeMode{core.LeadAll, core.LeadReported, core.LeadPipeline} {
		view := core.DeriveView(snap, mode)
		total := 0
		for _, b := range view.Groups.Deployed {
			total += len(b.Commits)
		}
		if total != 5 {
			t.Errorf("%s: expected all 5 commits still grouped as deployed, got %d", mode, total)
		}
	}
}

// TestLeadTime_PipelineMode_InFlightTicksFromFirstEvent checks the live timer
// starts at the first event too, not the commit. A commit whose CI has started
// but which hasn't deployed shows how long the *pipeline* has had it.
func TestLeadTime_PipelineMode_InFlightTicksFromFirstEvent(t *testing.T) {
	commitAt := utc(2026, 1, 2, 17)
	ciStart := utc(2026, 1, 5, 9)
	now := utc(2026, 1, 5, 10)

	snap := core.Snapshot{Commits: []core.CommitView{{
		SHA:    "abc",
		Time:   commitAt,
		Events: []clarityrefs.Event{{Stage: "ci", Status: "started", Time: ciStart}},
	}}}

	view := core.DeriveView(snap, core.LeadPipeline)
	got, frozen, ok := view.Groups.LeadTime(0, now)
	if !ok {
		t.Fatal("expected a live lead time")
	}
	if frozen {
		t.Error("expected the timer to still be ticking — nothing has deployed")
	}
	if got != time.Hour {
		t.Errorf("lead time = %v, want 1h (ticking from ci started, not the commit)", got)
	}
}

// TestLeadTime_PipelineMode_NoEventsYet checks a freshly pushed commit shows
// no timer until the pipeline reports something. This is the known cost of
// pipeline mode: there is nothing to measure from until CI says it started.
func TestLeadTime_PipelineMode_NoEventsYet(t *testing.T) {
	snap := core.Snapshot{Commits: []core.CommitView{{
		SHA: "abc", Time: utc(2026, 1, 5, 9),
	}}}

	view := core.DeriveView(snap, core.LeadPipeline)
	if _, _, ok := view.Groups.LeadTime(0, utc(2026, 1, 5, 10)); ok {
		t.Error("expected no lead time for a commit the pipeline hasn't touched")
	}
}

// TestLeadTime_RejectsNonPositiveInterval guards the average against samples
// that aren't fast deliveries but data artefacts.
//
// The rebase case covers a commit whose author date is later than the deploy
// that shipped it — a negative interval. The terminal-events-only case covers
// LeadPipeline when a team reports just `deploy passed`: the earliest event
// *is* the deploy, so the start lands on the finish line and the interval is
// exactly zero. That mode has nothing to measure, which is not the same as
// measuring nothing, and a run of zeroes would quietly report a perfect
// pipeline.
func TestLeadTime_RejectsNonPositiveInterval(t *testing.T) {
	cases := []struct {
		name  string
		modes []core.LeadTimeMode
		snap  core.Snapshot
	}{
		{
			name:  "commit authored after the deploy that shipped it",
			modes: []core.LeadTimeMode{core.LeadAll, core.LeadReported, core.LeadPipeline},
			snap: core.Snapshot{Commits: []core.CommitView{{
				SHA:  "rebased",
				Time: utc(2026, 1, 5, 11),
				Events: []clarityrefs.Event{
					{Stage: "deploy", Status: "passed", Time: utc(2026, 1, 5, 9)},
				},
			}}},
		},
		{
			name:  "only terminal events reported",
			modes: []core.LeadTimeMode{core.LeadPipeline},
			snap: core.Snapshot{Commits: []core.CommitView{{
				SHA:  "terminal-only",
				Time: utc(2026, 1, 5, 8),
				Events: []clarityrefs.Event{
					{Stage: "ci", Status: "passed", Time: utc(2026, 1, 5, 9)},
					{Stage: "deploy", Status: "passed", Time: utc(2026, 1, 5, 9)},
				},
			}}},
		},
	}

	for _, c := range cases {
		for _, mode := range c.modes {
			view := core.DeriveView(c.snap, mode)
			if _, _, ok := view.Groups.LeadTime(0, utc(2026, 1, 5, 12)); ok {
				t.Errorf("%s/%s: expected no lead time for a non-positive interval", c.name, mode)
			}
			for _, w := range view.Weekly {
				if w.AvgLead != 0 {
					t.Errorf("%s/%s: expected the sample excluded, got average %v",
						c.name, mode, w.AvgLead)
				}
			}
		}
	}
}

// TestLeadTime_PipelineMode_UsesEarliestEvent checks the start is the earliest
// event, whatever order they arrive in — event slices come from adapters that
// don't all guarantee ordering.
func TestLeadTime_PipelineMode_UsesEarliestEvent(t *testing.T) {
	snap := core.Snapshot{Commits: []core.CommitView{{
		SHA:  "abc",
		Time: utc(2026, 1, 2, 17),
		Events: []clarityrefs.Event{
			{Stage: "deploy", Status: "passed", Time: utc(2026, 1, 5, 10)},
			{Stage: "ci", Status: "started", Time: utc(2026, 1, 5, 9)},
		},
	}}}

	view := core.DeriveView(snap, core.LeadPipeline)
	got, _, ok := view.Groups.LeadTime(0, utc(2026, 1, 5, 12))
	if !ok {
		t.Fatal("expected a lead time")
	}
	if got != time.Hour {
		t.Errorf("lead time = %v, want 1h measured from the earliest event", got)
	}
}

// TestParseLeadTimeMode pins the config surface, including that an unset
// value keeps the historical behaviour rather than silently changing every
// existing user's numbers.
func TestParseLeadTimeMode(t *testing.T) {
	cases := []struct {
		in      string
		want    core.LeadTimeMode
		wantErr bool
	}{
		{in: "", want: core.LeadAll},
		{in: "all", want: core.LeadAll},
		{in: "reported", want: core.LeadReported},
		{in: "pipeline", want: core.LeadPipeline},
		{in: "ALL", want: core.LeadAll},
		{in: "Pipeline", want: core.LeadPipeline},
		{in: "everything", wantErr: true},
		{in: "commit", wantErr: true},
	}

	for _, c := range cases {
		got, err := core.ParseLeadTimeMode(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseLeadTimeMode(%q) should have failed", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseLeadTimeMode(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseLeadTimeMode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestParseLeadTimeMode_ErrorNamesTheOptions — an invalid enum in a config
// file is only useful if the message says what the valid values are.
func TestParseLeadTimeMode_ErrorNamesTheOptions(t *testing.T) {
	_, err := core.ParseLeadTimeMode("nonsense")
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"all", "reported", "pipeline", "nonsense"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}
