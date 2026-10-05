package present_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
	"github.com/ezcdlabs/clarity/mobile/internal/present"
	v1 "github.com/ezcdlabs/clarity/proto/gen/go/clarityv1"
)

// metricsOf derives a view and maps it, which is the whole path the FFI runs.
func metricsOf(t *testing.T, snap core.Snapshot) *v1.Metrics {
	t.Helper()
	view := core.DeriveView(snap, core.DefaultLeadTimeMode, nil)
	return present.Metrics(view, midWeek())
}

// shipping builds n deploys in one ISO week, each carrying one commit with the
// given lead time, so a week's distribution is whatever the test says it is.
//
// Deploy times are spread across the week rather than stacked on one instant:
// weeks are bucketed by deploy date, and a batch is distinct per deploy event.
func shipping(weekOffset int, leads ...time.Duration) []core.CommitView {
	base := midWeek().AddDate(0, 0, -7*weekOffset)
	var out []core.CommitView
	for i, lead := range leads {
		deployedAt := base.Add(-time.Duration(i) * time.Hour)
		sha := string(rune('a'+weekOffset)) + string(rune('a'+i))
		out = append(out, core.CommitView{
			SHA:     sha + "cccccccccc",
			Subject: "feat: work",
			Time:    deployedAt.Add(-lead),
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: deployedAt.Add(-time.Minute)},
				{Stage: "deploy", Status: "passed", Time: deployedAt},
			},
		})
	}
	return out
}

// TestMetrics_AWeekCarriesItsFiveNumberSummary is the headline of the view:
// each week's distribution, in seconds, with the median as the middle number.
func TestMetrics_AWeekCarriesItsFiveNumberSummary(t *testing.T) {
	// Six, so quartiles describe something and the box plot is chosen.
	leads := []time.Duration{
		time.Hour, 2 * time.Hour, 3 * time.Hour,
		4 * time.Hour, 5 * time.Hour, 6 * time.Hour,
	}
	out := metricsOf(t, core.Snapshot{RepoName: "api", Commits: shipping(0, leads...)})

	if len(out.Flows) != 1 {
		t.Fatalf("expected one flow, got %d", len(out.Flows))
	}
	weeks := out.Flows[0].Weeks
	if len(weeks) != 1 {
		t.Fatalf("expected one week, got %d: %+v", len(weeks), weeks)
	}
	w := weeks[0]

	if w.Plot != v1.Plot_PLOT_BOX {
		t.Errorf("plot = %v, want a box: six deploys is enough for quartiles", w.Plot)
	}
	if w.N != 6 {
		t.Errorf("n = %d, want 6", w.N)
	}
	if w.MinSeconds != 3600 || w.MaxSeconds != 6*3600 {
		t.Errorf("extremes = %ds..%ds, want 3600..21600", w.MinSeconds, w.MaxSeconds)
	}
	// The quartiles interpolate, so these are the positions rather than
	// samples: p50 of six values is between the third and fourth.
	if w.P50Seconds <= w.P25Seconds || w.P75Seconds <= w.P50Seconds {
		t.Errorf("quartiles out of order: %d, %d, %d", w.P25Seconds, w.P50Seconds, w.P75Seconds)
	}
	if w.Deploys != 6 {
		t.Errorf("deploys = %d, want 6", w.Deploys)
	}
	if w.Label == "" {
		t.Error("the week has no label, so no row can be named")
	}
}

// TestMetrics_TooFewDeploysSendsTheSamplesInstead pins the honest rendering of
// a quiet week. Below the floor the quantiles are interpolations between two or
// three real values, and drawing them would put a smear on screen where there
// were only two deploys.
func TestMetrics_TooFewDeploysSendsTheSamplesInstead(t *testing.T) {
	out := metricsOf(t, core.Snapshot{
		RepoName: "api",
		Commits:  shipping(0, time.Hour, 5*time.Hour),
	})
	w := out.Flows[0].Weeks[0]

	if w.Plot != v1.Plot_PLOT_POINTS {
		t.Fatalf("plot = %v, want points for two deploys", w.Plot)
	}
	if len(w.SampleSeconds) != 2 {
		t.Fatalf("samples = %v, want the two real lead times", w.SampleSeconds)
	}
	if w.SampleSeconds[0] != 3600 || w.SampleSeconds[1] != 5*3600 {
		t.Errorf("samples = %v, want [3600 18000] sorted", w.SampleSeconds)
	}
}

// TestMetrics_AWeekWithNoDeploysIsStillARow — a gap in the trend is a finding,
// and a view that simply omitted the week would read as continuous.
func TestMetrics_AWeekWithNoDeploysIsStillARow(t *testing.T) {
	// A commit that passed CI but never shipped, in a week of its own.
	never := midWeek()
	out := metricsOf(t, core.Snapshot{RepoName: "api", Commits: append(
		shipping(0, time.Hour, 2*time.Hour),
		core.CommitView{SHA: "zzzzzzzzzzzz", Subject: "feat: unshipped", Time: never,
			Events: []clarityrefs.Event{{Stage: "ci", Status: "passed", Time: never}}},
	)})

	for _, w := range out.Flows[0].Weeks {
		if w.Plot == v1.Plot_PLOT_UNSPECIFIED {
			t.Errorf("week %s has no plot decision", w.Label)
		}
		if w.Plot == v1.Plot_PLOT_NONE && w.N != 0 {
			t.Errorf("week %s says nothing shipped but measured %d lead times", w.Label, w.N)
		}
	}
}

// TestMetrics_WeeksAreNewestFirst keeps the phone's order the terminal's, so a
// row means the same thing in both.
func TestMetrics_WeeksAreNewestFirst(t *testing.T) {
	commits := append(shipping(0, time.Hour, 2*time.Hour), shipping(2, 3*time.Hour)...)
	out := metricsOf(t, core.Snapshot{RepoName: "api", Commits: commits})

	weeks := out.Flows[0].Weeks
	if len(weeks) < 2 {
		t.Fatalf("expected at least two weeks, got %+v", weeks)
	}
	if weeks[0].Label <= weeks[len(weeks)-1].Label {
		t.Errorf("weeks run %s first and %s last; want newest first",
			weeks[0].Label, weeks[len(weeks)-1].Label)
	}
}

// TestMetrics_TheAxisIsSharedAndLabelled covers the one scale every row is
// drawn against, and the labels a client puts on it.
func TestMetrics_TheAxisIsSharedAndLabelled(t *testing.T) {
	out := metricsOf(t, core.Snapshot{
		RepoName: "api",
		Commits:  shipping(0, time.Hour, 2*time.Hour, 3*time.Hour),
	})
	axis := out.Flows[0].Axis

	if axis == nil {
		t.Fatal("no axis, so nothing on this flow has a scale")
	}
	if axis.MaxSeconds <= 0 {
		t.Fatalf("axis max = %ds; every value would pile into one place", axis.MaxSeconds)
	}
	// Five ticks: the ends and the quarters. Which of them a client draws is
	// its own business — how many fit is geometry — but it cannot invent the
	// labels, so all five cross.
	if len(axis.Ticks) != 5 {
		t.Fatalf("ticks = %+v, want five", axis.Ticks)
	}
	if axis.Ticks[0].Fraction != 0 || axis.Ticks[0].Label != "0" {
		t.Errorf("first tick = %+v, want the origin labelled \"0\"", axis.Ticks[0])
	}
	if axis.Ticks[4].Fraction != 1 {
		t.Errorf("last tick is at %v, want the end of the axis", axis.Ticks[4].Fraction)
	}
	seen := map[string]bool{}
	for _, tick := range axis.Ticks {
		if seen[tick.Label] {
			t.Errorf("two ticks are both labelled %q, so they read as one place", tick.Label)
		}
		seen[tick.Label] = true
		// Nothing here is beyond the scale, so nothing may claim to be. A "+"
		// on a complete axis tells the reader data is hidden from them.
		if strings.HasSuffix(tick.Label, "+") {
			t.Errorf("tick %q claims the scale is incomplete, but this axis holds everything",
				tick.Label)
		}
	}
	if axis.Clamped {
		t.Error("axis is marked clamped with every lead time comfortably on it")
	}
}

// TestMetrics_ALeadTimeExactlyOnTheAxisIsDrawnRatherThanExcluded pins the
// boundary. A value landing precisely on the scale is shown — the alternative
// marks a row as having hidden data and then hides nothing, and it would
// disagree with the "+" on the final tick, which is set the same way.
//
// Built by hand rather than derived: the axis chooser rounds to named steps, so
// arranging a lead time that lands exactly on one through a snapshot would be
// luck rather than a test.
func TestMetrics_ALeadTimeExactlyOnTheAxisIsDrawnRatherThanExcluded(t *testing.T) {
	const max = 8 * time.Hour
	view := core.View{Flows: []core.FlowView{{
		Flow:     core.Flow{Name: "web"},
		LeadAxis: core.LeadAxis{Max: max},
		Weekly: []core.WeekStat{{
			Year: 2026, Week: 40, Deploys: 5,
			Leads: core.Quantiles([]time.Duration{
				time.Hour, 2 * time.Hour, 3 * time.Hour, 4 * time.Hour, max,
			}),
		}},
	}}}

	w := present.Metrics(view, midWeek()).Flows[0].Weeks[0]
	if w.MaxSeconds != int64(max/time.Second) {
		t.Fatalf("longest lead = %ds, want %ds", w.MaxSeconds, int64(max/time.Second))
	}
	if w.BeyondAxis {
		t.Error("a lead time sitting exactly on the axis is marked as beyond it")
	}
}

// TestMetrics_AClampedAxisSaysSoOnBothTheTickAndTheRow verifies the two places
// an excluded value is disclosed: the scale says it is incomplete, and the week
// responsible says it is the one running past the end.
func TestMetrics_AClampedAxisSaysSoOnBothTheTickAndTheRow(t *testing.T) {
	// One commit that sat for a month, among a week of fast ones — which is
	// exactly the outlier Tukey's fence excludes from the axis.
	fast := []time.Duration{time.Hour, 2 * time.Hour, time.Hour, 2 * time.Hour, time.Hour}
	out := metricsOf(t, core.Snapshot{RepoName: "api", Commits: append(
		shipping(0, fast...),
		shipping(2, 30*24*time.Hour, time.Hour, 2*time.Hour, time.Hour, 2*time.Hour)...,
	)})

	flow := out.Flows[0]
	if !flow.Axis.Clamped {
		t.Fatalf("axis max %ds is not marked clamped, but a month-long lead time "+
			"cannot be on it", flow.Axis.MaxSeconds)
	}
	last := flow.Axis.Ticks[4]
	if last.Label == "" || last.Label[len(last.Label)-1] != '+' {
		t.Errorf("final tick = %q, want a %q to say the scale is incomplete", last.Label, "+")
	}

	var marked int
	for _, w := range flow.Weeks {
		if w.BeyondAxis {
			marked++
			if w.MaxSeconds <= flow.Axis.MaxSeconds {
				t.Errorf("week %s is marked beyond the axis but its longest lead is %ds of %ds",
					w.Label, w.MaxSeconds, flow.Axis.MaxSeconds)
			}
		}
	}
	if marked == 0 {
		t.Error("the axis is clamped but no row admits to being the reason")
	}
}

// TestMetrics_BarsAreRelativeToTheBusiestWeekOnShow pins the denominator the
// deploy bars are drawn against. "The busiest week you can see" is what makes
// the rows on screen comparable to each other.
func TestMetrics_BarsAreRelativeToTheBusiestWeekOnShow(t *testing.T) {
	commits := append(
		shipping(0, time.Hour, 2*time.Hour),
		shipping(1, time.Hour, 2*time.Hour, 3*time.Hour, 4*time.Hour)...,
	)
	out := metricsOf(t, core.Snapshot{RepoName: "api", Commits: commits})

	flow := out.Flows[0]
	busiest := int32(0)
	for _, w := range flow.Weeks {
		if w.Deploys > busiest {
			busiest = w.Deploys
		}
	}
	if flow.MaxDeploys != busiest {
		t.Errorf("max deploys = %d, but the busiest week on show had %d",
			flow.MaxDeploys, busiest)
	}
}

// TestMetrics_EveryFlowGetsItsOwnWeeks verifies a repo that ships two things
// from one trunk reports them separately. Averaging them describes neither.
func TestMetrics_EveryFlowGetsItsOwnWeeks(t *testing.T) {
	now := midWeek()
	snap := core.Snapshot{RepoName: "api", Commits: []core.CommitView{
		{SHA: "aaaaaaaaaaaa", Subject: "feat: both", Time: now.Add(-3 * time.Hour),
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: now.Add(-2 * time.Hour)},
				{Stage: "deploy", Target: "web", Status: "passed", Time: now.Add(-time.Hour)},
			}},
	}}
	flows := []core.Flow{
		{Name: "web", Targets: []string{"web"}},
		{Name: "ios", Targets: []string{"ios"}},
	}
	out := present.Metrics(core.DeriveView(snap, core.DefaultLeadTimeMode, flows), now)

	if len(out.Flows) != 2 {
		t.Fatalf("expected a flow each, got %d", len(out.Flows))
	}
	if out.Flows[0].Name != "web" || out.Flows[1].Name != "ios" {
		t.Errorf("flows = %q, %q; want display order preserved",
			out.Flows[0].Name, out.Flows[1].Name)
	}
	// Each flow needs its own axis, even an empty one: a client that found nil
	// would have nothing to draw a scale from.
	for _, f := range out.Flows {
		if f.Axis == nil || f.Axis.MaxSeconds <= 0 {
			t.Errorf("flow %q has no usable axis: %+v", f.Name, f.Axis)
		}
	}
}

// TestMetrics_AFlowWithNoScaleStillGetsOne covers the fallback.
//
// DeriveView never produces a non-positive axis — a flow with nothing deployed
// still gets the default — so this is about the boundary rather than about
// today's callers. Passed through, a zero axis puts every value in the same
// place and labels every tick "0", which is a chart that looks like data and is
// not one.
func TestMetrics_AFlowWithNoScaleStillGetsOne(t *testing.T) {
	view := core.View{Flows: []core.FlowView{{
		Flow:   core.Flow{Name: "web"},
		Weekly: []core.WeekStat{{Year: 2026, Week: 40}},
	}}}

	axis := present.Metrics(view, midWeek()).Flows[0].Axis
	if axis.MaxSeconds <= 0 {
		t.Fatalf("axis max = %ds, so every lead time would pile into one place", axis.MaxSeconds)
	}
	seen := map[string]bool{}
	for _, tick := range axis.Ticks {
		if seen[tick.Label] {
			t.Fatalf("every tick is labelled %q", tick.Label)
		}
		seen[tick.Label] = true
	}
}

// TestMetrics_TruncationCrosses — an aggregate short a few deploys is wrong in
// a way no reader can see, so the client has to be told the window ran out.
func TestMetrics_TruncationCrosses(t *testing.T) {
	snap := core.Snapshot{RepoName: "api", Truncated: true, Limit: 2000,
		Commits: shipping(0, time.Hour, 2*time.Hour)}
	out := metricsOf(t, snap)

	if !out.Truncated || out.Limit != 2000 {
		t.Errorf("truncated = %v, limit = %d; want true and 2000", out.Truncated, out.Limit)
	}
}

// TestMetrics_IsStampedWithWhenItWasBuilt is load-bearing twice over: a client
// holds this across a failed refresh and needs to know how old it is, and a
// message with no field set encodes to zero bytes, which gomobile cannot carry.
func TestMetrics_IsStampedWithWhenItWasBuilt(t *testing.T) {
	now := midWeek()
	out := present.Metrics(core.View{}, now)

	if out.GeneratedUnixSeconds != now.Unix() {
		t.Errorf("stamped %d, want %d", out.GeneratedUnixSeconds, now.Unix())
	}
}
