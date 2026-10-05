package core_test

import (
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/internal/core"
)

// TestWeekLabel pins the one spelling of a week. It is written into a deploy
// divider, a metrics row and a mobile payload, and three spellings of the same
// week would read as three different weeks.
func TestWeekLabel(t *testing.T) {
	cases := []struct {
		year, week int
		want       string
	}{
		{2026, 39, "W2026-39"},
		// Zero-padded, so labels are the same width and a column of them
		// lines up — and so W2026-9 cannot be misread as week 90-something.
		{2026, 9, "W2026-09"},
		{2026, 1, "W2026-01"},
		// ISO year, which at a year boundary is not the calendar year. Week 53
		// exists and has to survive the formatting.
		{2025, 53, "W2025-53"},
	}
	for _, c := range cases {
		got := core.WeekLabel(core.WeekStat{Year: c.year, Week: c.week})
		if got != c.want {
			t.Errorf("WeekLabel(%d, %d) = %q, want %q", c.year, c.week, got, c.want)
		}
	}
}

// TestWeekDividerLabel_StartsWithTheWeekLabel keeps the divider and the bare
// label from drifting apart: the divider is the label plus facts, not its own
// spelling of the week.
func TestWeekDividerLabel_StartsWithTheWeekLabel(t *testing.T) {
	s := core.WeekStat{Year: 2026, Week: 7, Deploys: 3, AvgLead: 2 * time.Hour}
	divider := core.WeekDividerLabel(s)
	if label := core.WeekLabel(s); len(divider) < len(label) || divider[:len(label)] != label {
		t.Errorf("WeekDividerLabel(%+v) = %q, which does not start with %q", s, divider, label)
	}
}

// TestFormatAxisTick covers the labels on a lead time axis.
//
// A separate format from FormatElapsed, which is right for a timer and wrong
// here: it renders two hours as "2h 00m 00s" and has no day unit at all, so a
// three-day axis would be labelled "72h 00m 00s" four times over.
//
// The rule being pinned is precision at the quarter marks. An axis is labelled
// at 0, ¼, ½, ¾ and the end, so whatever unit a tick uses has to keep those
// five distinct — which is why days carry a decimal and hours do not.
func TestFormatAxisTick(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		// Zero is "0" with no unit: it is the origin of the scale rather than
		// a duration of zero seconds.
		{0, "0"},
		{30 * time.Minute, "30m"},
		{45 * time.Minute, "45m"},
		{time.Hour, "1h"},
		{2 * time.Hour, "2h"},
		{8 * time.Hour, "8h"},
		// A half hour keeps its decimal, so the ¾ tick of a 2h axis is
		// distinct from the end of it.
		{90 * time.Minute, "1.5h"},
		{4*time.Hour + 30*time.Minute, "4.5h"},
		{48 * time.Hour, "48h"},
		// Days only past three of them, where "72h" has stopped being
		// readable. A whole number of days loses the decimal.
		{72 * time.Hour, "3d"},
		{7 * 24 * time.Hour, "7d"},
		// And keeps it where it is the only thing separating two ticks: the
		// quarters of a 2d axis are 12h, 1d and 1.5d.
		{36 * time.Hour, "36h"},
		{84 * time.Hour, "3.5d"},
	}
	for _, c := range cases {
		if got := core.FormatAxisTick(c.d); got != c.want {
			t.Errorf("FormatAxisTick(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

// TestFormatAxisTick_QuartersOfAnAxisAreDistinct is the property the format
// exists for, asserted against every axis the chooser can pick rather than
// against a handful of durations.
func TestFormatAxisTick_QuartersOfAnAxisAreDistinct(t *testing.T) {
	axes := []time.Duration{
		30 * time.Minute, time.Hour, 2 * time.Hour, 4 * time.Hour, 6 * time.Hour,
		8 * time.Hour, 12 * time.Hour, 24 * time.Hour, 48 * time.Hour,
		72 * time.Hour, 7 * 24 * time.Hour,
	}
	for _, max := range axes {
		seen := map[string]time.Duration{}
		for _, f := range []float64{0, 0.25, 0.5, 0.75, 1} {
			d := time.Duration(float64(max) * f)
			label := core.FormatAxisTick(d)
			if prev, dup := seen[label]; dup {
				t.Errorf("axis %v labels both %v and %v as %q, so two ticks read as one place",
					max, prev, d, label)
			}
			seen[label] = d
		}
	}
}
