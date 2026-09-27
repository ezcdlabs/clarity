package core

import (
	"math"
	"sort"
	"time"
)

// LeadQuantiles summarises one week's lead time distribution.
//
// Quantiles rather than raw samples, because a renderer consumes the View it
// is handed and never re-derives from the Snapshot — the rule the leadTime
// bug established. A box plot needs exactly these five numbers.
//
// The median is the headline, not the mean: lead times are strongly
// right-skewed, so one commit that sat over a weekend drags an average badly.
// The quartiles carry the spread, which matters as much as the middle — a
// two-hour median with a three-day p75 is a problem the median alone hides.
type LeadQuantiles struct {
	N                       int // how many commits contributed a lead time
	Min, P25, P50, P75, Max time.Duration
	// Samples carries the individual lead times, but only when N is too
	// small for quartiles to describe anything — see SampleFloor.
	//
	// Below that floor the quantiles are interpolations between two or three
	// real values, so drawing them plots a smear where there were only two
	// deploys. The samples are the honest rendering, and they are bounded by
	// the floor, so this cannot grow with a busy week.
	Samples []time.Duration
}

// SampleFloor is the sample size below which quantiles stop meaning anything
// and LeadQuantiles carries the individual values instead.
const SampleFloor = 5

// Quantiles summarises leads. Input need not be sorted; it is not mutated.
func Quantiles(leads []time.Duration) LeadQuantiles {
	if len(leads) == 0 {
		return LeadQuantiles{}
	}
	s := append([]time.Duration(nil), leads...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	q := LeadQuantiles{
		N:   len(s),
		Min: s[0],
		P25: quantileAt(s, 0.25),
		P50: quantileAt(s, 0.50),
		P75: quantileAt(s, 0.75),
		Max: s[len(s)-1],
	}
	if len(s) < SampleFloor {
		q.Samples = s
	}
	return q
}

// quantileAt interpolates between neighbours, so a quantile that falls
// between two samples is between their values rather than snapped to one.
func quantileAt(sorted []time.Duration, q float64) time.Duration {
	if len(sorted) == 1 {
		return sorted[0]
	}
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	frac := pos - float64(lo)
	return sorted[lo] + time.Duration(float64(sorted[hi]-sorted[lo])*frac)
}

// defaultAxisMax is the axis for a window with no lead times at all, so an
// empty view still draws a scale instead of collapsing.
const defaultAxisMax = 8 * time.Hour

// axisSteps are the boundaries an axis is allowed to end on. Rounding to one
// of these does two jobs: the ticks read as durations rather than arbitrary
// fractions of the data, and the axis stops moving every time a deploy lands,
// so weeks stay comparable between runs and not only within one render.
var axisSteps = []time.Duration{
	30 * time.Minute, time.Hour, 2 * time.Hour, 4 * time.Hour, 6 * time.Hour,
	8 * time.Hour, 12 * time.Hour, 24 * time.Hour, 48 * time.Hour,
	72 * time.Hour, 7 * 24 * time.Hour,
}

// ChooseAxisMax picks one x-axis for every week in a window, from the pooled
// lead times of all of them.
//
// One axis, because per-row scaling would make the rows incomparable and
// comparing them is the entire point. Derived from the data, because a fixed
// ceiling wastes the width for a fast team and squashes a slow one.
//
// The hard part is what to do about outliers. Scaling to the largest value
// lets a single commit that sat over a bank holiday set an 80-hour axis and
// compress every other week into two columns — the worst week silently
// dictating the resolution of all the others. A percentile is the wrong
// instrument for the same reason it looks right: it is a quantile, so there
// is always data above it, and it clips whether or not anything deserves
// clipping.
//
// Tukey's fence — Q3 + 1.5×IQR — is the box plot's own definition of an
// outlier, so it answers exactly the question being asked. It is computed on
// LOG durations rather than raw ones: lead times are strongly right-skewed,
// a long tail is ordinary rather than exceptional, and a raw fence reads that
// ordinary skew as a crowd of outliers and clips healthy data. In log space
// the fence is multiplicative, which is how lead times actually vary — "about
// five times the typical commit" is an outlier, "two hours longer than the
// typical commit" is not.
//
// Well-behaved data has nothing beyond the fence and nothing is clipped.
func ChooseAxisMax(leads []time.Duration) time.Duration {
	positive := make([]float64, 0, len(leads))
	largest := time.Duration(0)
	for _, d := range leads {
		if d <= 0 {
			continue // a non-positive lead time is excluded upstream too
		}
		positive = append(positive, math.Log(float64(d)))
		if d > largest {
			largest = d
		}
	}
	if len(positive) == 0 {
		return defaultAxisMax
	}
	sort.Float64s(positive)

	q1 := quantileFloat(positive, 0.25)
	q3 := quantileFloat(positive, 0.75)
	fence := time.Duration(math.Exp(q3 + 1.5*(q3-q1)))

	limit := largest // everything fits unless the fence says otherwise
	if fence < limit {
		limit = fence
	}
	return roundUpToAxisStep(limit)
}

func quantileFloat(sorted []float64, q float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	return sorted[lo] + (sorted[hi]-sorted[lo])*(pos-float64(lo))
}

func roundUpToAxisStep(d time.Duration) time.Duration {
	for _, s := range axisSteps {
		if d <= s {
			return s
		}
	}
	return d
}

// LeadAxis is the shared x-axis a set of weeks is drawn against.
type LeadAxis struct {
	Max time.Duration // every week is scaled to this
	// Clamped reports whether any lead time sits beyond Max, so the view can
	// mark the scale as incomplete rather than clipping in silence.
	Clamped bool
}

// Report is a window of whole weeks drawn against one shared axis.
//
// The axis lives on the report rather than on each week because it is chosen
// from all of them at once: a per-week axis would make the rows incomparable,
// and comparing them is the only reason the view exists.
type Report struct {
	Weeks []WeekStat
	LeadAxis
}

// WeeklyReport builds a Report from every commit in snap.
func WeeklyReport(snap Snapshot, mode LeadTimeMode) Report {
	stats, pooled := weeklyStats(snap, GroupCommitsMode(snap.Commits, mode))
	return newReport(stats, pooled)
}

// WeeklyReportForFlow builds a Report for one deploy flow, with candidacy
// applied — so a flow's throughput is built only from commits it ships. A
// repo that ships web and ios from one trunk has genuinely different lead
// times for each, and averaging them describes neither.
func WeeklyReportForFlow(snap Snapshot, mode LeadTimeMode, f Flow) Report {
	stats, pooled := weeklyStats(snap, GroupCommitsForFlow(snap.Commits, mode, f))
	return newReport(stats, pooled)
}

// newReport picks the shared axis from the pooled raw lead times, then
// records whether anything was left beyond it.
func newReport(weeks []WeekStat, pooled []time.Duration) Report {
	axis := ChooseAxisMax(pooled)
	clamped := false
	for _, w := range weeks {
		if w.Leads.N > 0 && w.Leads.Max > axis {
			clamped = true
			break
		}
	}
	return Report{Weeks: weeks, LeadAxis: LeadAxis{Max: axis, Clamped: clamped}}
}
