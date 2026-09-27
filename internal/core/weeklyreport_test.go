package core_test

import (
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
)

func hrs(v float64) time.Duration { return time.Duration(v * float64(time.Hour)) }

// TestLeadQuantiles covers the distribution summary a box plot is drawn from.
// A mean cannot describe a right-skewed distribution, and the spread matters
// as much as the middle: a two-hour median with a three-day p75 is a problem
// the median alone hides.
func TestLeadQuantiles(t *testing.T) {
	tests := []struct {
		name                    string
		in                      []time.Duration
		wantN                   int
		min, p25, p50, p75, max time.Duration
	}{
		{
			name:  "five evenly spaced values land on exact indices",
			in:    []time.Duration{hrs(1), hrs(2), hrs(3), hrs(4), hrs(5)},
			wantN: 5,
			min:   hrs(1), p25: hrs(2), p50: hrs(3), p75: hrs(4), max: hrs(5),
		},
		{
			name:  "unsorted input is sorted first",
			in:    []time.Duration{hrs(5), hrs(1), hrs(4), hrs(2), hrs(3)},
			wantN: 5,
			min:   hrs(1), p25: hrs(2), p50: hrs(3), p75: hrs(4), max: hrs(5),
		},
		{
			name:  "quantiles interpolate between neighbours",
			in:    []time.Duration{hrs(0), hrs(4)},
			wantN: 2,
			min:   hrs(0), p25: hrs(1), p50: hrs(2), p75: hrs(3), max: hrs(4),
		},
		{
			name:  "a single value is every quantile",
			in:    []time.Duration{hrs(3)},
			wantN: 1,
			min:   hrs(3), p25: hrs(3), p50: hrs(3), p75: hrs(3), max: hrs(3),
		},
		{
			name:  "no values is the zero summary",
			in:    nil,
			wantN: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := core.Quantiles(tc.in)
			if got.N != tc.wantN {
				t.Fatalf("N = %d, want %d", got.N, tc.wantN)
			}
			for _, c := range []struct {
				label string
				got   time.Duration
				want  time.Duration
			}{
				{"Min", got.Min, tc.min}, {"P25", got.P25, tc.p25},
				{"P50", got.P50, tc.p50}, {"P75", got.P75, tc.p75},
				{"Max", got.Max, tc.max},
			} {
				if c.got != c.want {
					t.Errorf("%s = %v, want %v", c.label, c.got, c.want)
				}
			}
		})
	}
}

// TestChooseAxisMax covers the shared x-axis every week is drawn against.
//
// Three failure modes it has to avoid at once, each of which is what an
// obvious choice does:
//
//   - a fixed ceiling wastes the width for a fast team and squashes a slow one
//   - scaling to the largest value lets one commit that sat over a weekend
//     compress every other week into a couple of columns
//   - a percentile always clips, because a quantile by construction has data
//     above it
//
// Tukey's fence on LOG durations gets all three. Lead times are right-skewed,
// so a long tail is ordinary; in log space the fence is multiplicative, which
// is how lead times actually vary.
func TestChooseAxisMax(t *testing.T) {
	tidy := []time.Duration{
		hrs(1), hrs(1.2), hrs(1.5), hrs(1.8), hrs(2), hrs(2.4), hrs(2.8),
		hrs(3.2), hrs(3.8), hrs(4.4), hrs(5), hrs(6),
	}

	t.Run("tidy data is never clipped", func(t *testing.T) {
		got := core.ChooseAxisMax(tidy)
		if got < hrs(6) {
			t.Errorf("axis %v is below the largest value %v, so tidy data would be clipped", got, hrs(6))
		}
	})

	t.Run("a lone outlier is excluded rather than setting the scale", func(t *testing.T) {
		withOutlier := append(append([]time.Duration(nil), tidy...), hrs(74))
		got := core.ChooseAxisMax(withOutlier)
		if got >= hrs(74) {
			t.Errorf("axis %v was dragged out to the outlier; every other week would be squashed", got)
		}
		if got < hrs(6) {
			t.Errorf("axis %v excludes ordinary data as well as the outlier", got)
		}
	})

	t.Run("a uniformly slow team widens the axis instead of clipping", func(t *testing.T) {
		slow := make([]time.Duration, len(tidy))
		for i, d := range tidy {
			slow[i] = d * 6
		}
		got := core.ChooseAxisMax(slow)
		if got < hrs(36) {
			t.Errorf("axis %v clips a team that is simply slow, not erratic", got)
		}
	})

	t.Run("identical values do not collapse the axis", func(t *testing.T) {
		same := []time.Duration{hrs(3), hrs(3), hrs(3), hrs(3)}
		got := core.ChooseAxisMax(same)
		if got < hrs(3) {
			t.Errorf("axis %v is below the only value present, %v", got, hrs(3))
		}
	})

	t.Run("no data still yields a usable axis", func(t *testing.T) {
		if got := core.ChooseAxisMax(nil); got <= 0 {
			t.Errorf("axis = %v, want a positive default", got)
		}
	})

	t.Run("the axis lands on a readable boundary", func(t *testing.T) {
		// Otherwise the ticks read as arbitrary fractions of the data, and the
		// axis shifts every time a deploy lands, so weeks stop being
		// comparable between runs.
		got := core.ChooseAxisMax(tidy)
		nice := map[time.Duration]bool{
			30 * time.Minute: true, time.Hour: true, 2 * time.Hour: true,
			4 * time.Hour: true, 6 * time.Hour: true, 8 * time.Hour: true,
			12 * time.Hour: true, 24 * time.Hour: true, 48 * time.Hour: true,
			72 * time.Hour: true, 7 * 24 * time.Hour: true,
		}
		if !nice[got] {
			t.Errorf("axis = %v, which is not one of the readable boundaries", got)
		}
	})
}

// TestWeeklyStats_CarriesTheDistribution verifies a week's stats carry the
// spread, not only the mean. The weekly view draws a box plot from these, and
// nothing downstream may go back to the Snapshot to recompute them.
func TestWeeklyStats_CarriesTheDistribution(t *testing.T) {
	// One deploy on the Thursday, carrying three commits authored 1h, 3h and
	// 5h before it — so the week's lead times are exactly 1h, 3h, 5h.
	deployAt := utc(2026, 1, 8, 12)
	snap := core.Snapshot{Commits: []core.CommitView{
		commit("a", deployAt.Add(-1*time.Hour), deployAt),
		commit("b", deployAt.Add(-3*time.Hour), deployAt),
		commit("c", deployAt.Add(-5*time.Hour), deployAt),
	}}

	got := core.WeeklyStats(snap)
	if len(got) != 1 {
		t.Fatalf("expected 1 week, got %d", len(got))
	}
	q := got[0].Leads
	if q.N != 3 {
		t.Fatalf("N = %d, want 3", q.N)
	}
	if q.Min != hrs(1) || q.P50 != hrs(3) || q.Max != hrs(5) {
		t.Errorf("min/median/max = %v/%v/%v, want 1h/3h/5h", q.Min, q.P50, q.Max)
	}
	// The mean is unchanged, so the deploy strip keeps reporting what it did.
	if got[0].AvgLead != hrs(3) {
		t.Errorf("AvgLead = %v, want 3h", got[0].AvgLead)
	}
}

// TestWeeklyReport_SharesOneAxisAcrossWeeks verifies the report pools every
// week's lead times to pick a single axis. A per-week axis would make the
// rows incomparable, which is the one thing the view exists to do.
func TestWeeklyReport_SharesOneAxisAcrossWeeks(t *testing.T) {
	fast := utc(2026, 1, 8, 12)  // ISO week 2
	slow := utc(2026, 1, 15, 12) // ISO week 3
	snap := core.Snapshot{Commits: []core.CommitView{
		commit("a", fast.Add(-1*time.Hour), fast),
		commit("b", fast.Add(-2*time.Hour), fast),
		commit("c", slow.Add(-9*time.Hour), slow),
		commit("d", slow.Add(-11*time.Hour), slow),
	}}

	rep := core.WeeklyReport(snap, core.DefaultLeadTimeMode)
	if len(rep.Weeks) != 2 {
		t.Fatalf("expected 2 weeks, got %d", len(rep.Weeks))
	}
	if rep.Max < hrs(11) {
		t.Errorf("AxisMax = %v, which clips the slower week's data", rep.Max)
	}
	if rep.Clamped {
		t.Error("nothing here is an outlier; Clamped should be false")
	}
}

// TestWeeklyReport_FlagsClampingHonestly verifies the report says when the
// axis excludes something. The "+" on the final tick and the arrow on a row
// both come from this, and a view that clips silently is worse than one that
// squashes.
func TestWeeklyReport_FlagsClampingHonestly(t *testing.T) {
	at := utc(2026, 1, 8, 12)
	commits := []core.CommitView{}
	for _, lead := range []float64{1, 1.2, 1.5, 1.8, 2, 2.4, 2.8, 3.2, 3.8, 4.4, 5, 6} {
		commits = append(commits, commit("c", at.Add(-hrs(lead)), at))
	}
	// One commit that sat over a long weekend.
	commits = append(commits, commit("outlier", at.Add(-hrs(74)), at))

	rep := core.WeeklyReport(core.Snapshot{Commits: commits}, core.DefaultLeadTimeMode)
	if !rep.Clamped {
		t.Error("a 74h lead time sits beyond the axis; Clamped should say so")
	}
	if rep.Max >= hrs(74) {
		t.Errorf("AxisMax = %v; the outlier set the scale instead of being excluded", rep.Max)
	}
}

// TestDeriveView_PutsTheAxisOnEveryFlow verifies the shared axis reaches the
// renderer on the View, for every flow.
//
// On the View rather than recomputed downstream, because a renderer consumes
// what it is handed — the rule the leadTime bug established, where two layers
// derived the same thing and silently disagreed.
//
// The axis is per flow rather than once for the repo, so that a store review
// and a web deploy can scale differently. Whether two flows actually differ
// depends on candidacy, which is covered by the flow tests; what matters here
// is that each flow carries its own, agreeing with its own weeks.
func TestDeriveView_PutsTheAxisOnEveryFlow(t *testing.T) {
	at := utc(2026, 1, 8, 12)
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: "w1", Time: at.Add(-1 * time.Hour), Events: []clarityrefs.Event{
			{Stage: "deploy", Status: "passed", Time: at, Target: "web"}}},
		{SHA: "i1", Time: at.Add(-40 * time.Hour), Events: []clarityrefs.Event{
			{Stage: "deploy", Status: "passed", Time: at, Target: "ios"}}},
	}}

	view := core.DeriveView(snap, core.DefaultLeadTimeMode, nil)
	if len(view.Flows) < 2 {
		t.Fatalf("expected a flow per target, got %d", len(view.Flows))
	}

	for _, f := range view.Flows {
		if f.LeadAxis.Max <= 0 {
			t.Errorf("flow %q has no axis", f.Name)
			continue
		}
		// Every week the flow carries must fit the axis it was given, unless
		// the flow itself reports that something was left outside it.
		for _, w := range f.Weekly {
			if w.Leads.N > 0 && w.Leads.Max > f.LeadAxis.Max && !f.LeadAxis.Clamped {
				t.Errorf("flow %q: W%d-%02d has a lead of %v beyond the axis %v, "+
					"but the flow does not report clamping",
					f.Name, w.Year, w.Week, w.Leads.Max, f.LeadAxis.Max)
			}
		}
	}
}
