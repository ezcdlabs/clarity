package core_test

import (
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/internal/core"
)

// TestTrimToWholeWeeks covers the weekly window.
//
// The window is weeks rather than commits so that how far back you can see
// does not depend on how busy the repository was — a commit limit gives a
// quiet repo a year and a busy one four days, which is useless for comparing
// trend.
func TestTrimToWholeWeeks(t *testing.T) {
	weeks := func(n int) []core.WeekStat {
		out := make([]core.WeekStat, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, core.WeekStat{Year: 2026, Week: 40 - i})
		}
		return out
	}
	view := core.View{Flows: []core.FlowView{
		{Flow: core.Flow{Name: "web"}, Weekly: weeks(20)},
		{Flow: core.Flow{Name: "ios"}, Weekly: weeks(3)},
	}}

	got := core.TrimToWholeWeeks(view, 12)
	if n := len(got.Flows[0].Weekly); n != 12 {
		t.Errorf("web kept %d weeks, want 12", n)
	}
	// A flow with less history than the window keeps what it has rather than
	// being padded with weeks it never had.
	if n := len(got.Flows[1].Weekly); n != 3 {
		t.Errorf("ios kept %d weeks, want its own 3", n)
	}
	// Newest first, so trimming takes the oldest off the end.
	if got.Flows[0].Weekly[0].Week != 40 {
		t.Errorf("trimmed from the wrong end: first week is %d", got.Flows[0].Weekly[0].Week)
	}
}

// TestTrimToWholeWeeks_DoesNotMutateTheCallersView pins that the function
// reads as it behaves. It returns a View by value, which looks non-mutating,
// but Flows is a slice and writing through it reaches the caller.
func TestTrimToWholeWeeks_DoesNotMutateTheCallersView(t *testing.T) {
	weeks := make([]core.WeekStat, 10)
	for i := range weeks {
		weeks[i] = core.WeekStat{Year: 2026, Week: 40 - i}
	}
	original := core.View{Flows: []core.FlowView{
		{Flow: core.Flow{Name: "web"}, Weekly: weeks},
	}}

	_ = core.TrimToWholeWeeks(original, 3)
	if n := len(original.Flows[0].Weekly); n != 10 {
		t.Errorf("the caller's view was trimmed to %d weeks behind its back", n)
	}
}

// TestTrimToWholeWeeks_RecomputesTheAxis verifies the scale matches what is
// on screen.
//
// The axis was derived from the whole read window and then the window was
// trimmed, so a flow could advertise "+" on its final tick — meaning "some
// lead time is beyond this scale" — while every remaining row fitted. The
// scale described weeks nobody could see.
func TestTrimToWholeWeeks_RecomputesTheAxis(t *testing.T) {
	fast := core.Quantiles([]time.Duration{time.Hour, 2 * time.Hour, 3 * time.Hour})
	slow := core.Quantiles([]time.Duration{300 * time.Hour, 320 * time.Hour, 340 * time.Hour})

	view := core.View{Flows: []core.FlowView{{
		Flow: core.Flow{Name: "web"},
		Weekly: []core.WeekStat{
			{Year: 2026, Week: 40, Deploys: 3, Leads: fast},
			{Year: 2026, Week: 39, Deploys: 3, Leads: fast},
			{Year: 2026, Week: 38, Deploys: 3, Leads: slow}, // dropped below
		},
		LeadAxis: core.LeadAxis{Max: 8 * time.Hour, Clamped: true},
	}}}

	got := core.TrimToWholeWeeks(view, 2)
	axis := got.Flows[0].LeadAxis
	if axis.Clamped {
		t.Errorf("axis still claims data is hidden, but the week it referred to "+
			"was trimmed away; no visible row is beyond %v", axis.Max)
	}
	for _, w := range got.Flows[0].Weekly {
		if w.Leads.Max > axis.Max {
			t.Errorf("W%d-%02d has a lead of %v beyond the axis %v",
				w.Year, w.Week, w.Leads.Max, axis.Max)
		}
	}
}
