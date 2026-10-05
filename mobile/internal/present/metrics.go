package present

import (
	"time"

	"github.com/ezcdlabs/clarity/internal/core"
	v1 "github.com/ezcdlabs/clarity/proto/gen/go/clarityv1"
)

// axisFractions are the places a lead time axis is labelled.
//
// All five always cross. Which of them a client actually draws is geometry —
// the terminal sheds the quarters as it narrows, and a phone will do something
// of its own — but what a tick is *called* is a decision, and a client that
// derived its own labels from the axis maximum would be formatting durations in
// a third language.
var axisFractions = []float64{0, 0.25, 0.5, 0.75, 1}

// Metrics maps a derived view to the weekly aggregates model.
//
// now is passed in rather than read, for the same reason View takes one.
func Metrics(view core.View, now time.Time) *v1.Metrics {
	out := &v1.Metrics{
		Truncated: view.Snapshot.Truncated,
		Limit:     int32(view.Snapshot.Limit),
		// Always set, for the reason View.GeneratedUnixSeconds explains: a
		// message with no fields set encodes to zero bytes, and gomobile turns
		// a zero-length slice into a null array.
		GeneratedUnixSeconds: now.Unix(),
	}
	for _, f := range view.Flows {
		out.Flows = append(out.Flows, metricsFlow(f))
	}
	return out
}

func metricsFlow(f core.FlowView) *v1.MetricsFlow {
	out := &v1.MetricsFlow{
		Name:       f.Name,
		Undeclared: f.Undeclared,
		Axis:       axis(f.LeadAxis),
	}
	for _, w := range f.Weekly {
		if int32(w.Deploys) > out.MaxDeploys {
			out.MaxDeploys = int32(w.Deploys)
		}
		out.Weeks = append(out.Weeks, week(w, f.LeadAxis))
	}
	return out
}

// axis carries the scale and its labels.
//
// A non-positive maximum is replaced rather than passed on. DeriveView never
// produces one, but a flow with no deploys at all arrives with a zero axis from
// a hand-built view, and every value would then pile into one place with every
// tick labelled "0".
func axis(a core.LeadAxis) *v1.LeadAxis {
	if a.Max <= 0 {
		a.Max = core.ChooseAxisMax(nil)
	}
	out := &v1.LeadAxis{
		MaxSeconds: int64(a.Max / time.Second),
		Clamped:    a.Clamped,
	}
	for _, fraction := range axisFractions {
		label := core.FormatAxisTick(time.Duration(float64(a.Max) * fraction))
		if fraction == 1 && a.Clamped {
			// The scale is incomplete, said on the tick that claims to be the
			// end of it — which is the only place a reader is looking when they
			// ask how far the axis goes.
			label += "+"
		}
		out.Ticks = append(out.Ticks, &v1.AxisTick{Fraction: fraction, Label: label})
	}
	return out
}

func week(w core.WeekStat, a core.LeadAxis) *v1.Week {
	out := &v1.Week{
		Label:   core.WeekLabel(w),
		Deploys: int32(w.Deploys),
		Plot:    plot(w.Leads),
		N:       int32(w.Leads.N),
		// Strictly beyond, so a lead time landing exactly on the axis is drawn
		// rather than marked as excluded — and agrees with the "+" on the final
		// tick, which is set the same way.
		BeyondAxis: w.Leads.N > 0 && w.Leads.Max > a.Max,
	}
	if w.Leads.N == 0 {
		// No distribution to describe. Left at zero rather than filled with
		// anything: a five-number summary of nothing would draw a box at the
		// origin, which reads as "everything shipped instantly".
		return out
	}
	out.MinSeconds = seconds(w.Leads.Min)
	out.P25Seconds = seconds(w.Leads.P25)
	out.P50Seconds = seconds(w.Leads.P50)
	out.P75Seconds = seconds(w.Leads.P75)
	out.MaxSeconds = seconds(w.Leads.Max)
	for _, d := range w.Leads.Samples {
		out.SampleSeconds = append(out.SampleSeconds, seconds(d))
	}
	return out
}

// plot is the core's decision about how a week can honestly be drawn, carried
// as an answer rather than as the threshold behind it.
func plot(q core.LeadQuantiles) v1.Plot {
	switch {
	case q.N == 0:
		return v1.Plot_PLOT_NONE
	case q.N < core.SampleFloor:
		return v1.Plot_PLOT_POINTS
	default:
		return v1.Plot_PLOT_BOX
	}
}

func seconds(d time.Duration) int64 { return int64(d / time.Second) }
