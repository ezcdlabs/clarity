package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/ezcdlabs/clarity/internal/core"
)

// The weekly view answers a different question from the commit list. That one
// asks "is main green right now?"; this asks "are we getting better?" — which
// is invisible in a list of commits, because trend needs rows to compare.
//
// One row per ISO week: a box plot of that week's lead times on the left, a
// bar of its deploy count on the right. The two halves are one signal. Bars
// share a RIGHT edge and grow leftward, so a bad week pushes the box right
// AND pulls the bar back — degradation reads as ink migrating one direction
// across the row. Bars anchored left would move the two halves apart instead,
// leaving the reader tracking two opposing signals.

const (
	// weekLabelWidth fits "W2026-39" plus a space.
	weekLabelWidth = 9
	// countWidth fits a four-figure deploy count, right-aligned.
	countWidth = 4
	// rowChrome is everything in a row that is not the plot or the bar: the
	// indent, the week label, the gap between the two columns, and the count.
	// Kept in one place because the header has to line up with it — computing
	// the two separately is what left "deploys" hanging a column past the
	// counts it names.
	rowChrome = 2 + weekLabelWidth + 2 + countWidth
	// minPlotCols is where a box plot stops carrying meaning. Below this the
	// row still renders — a squashed plot still ranks weeks against each
	// other, which is what the view is for.
	minPlotCols = 6
)

// RenderWeekly renders the weekly view for the selected flow, with the deploy
// strip above it when there is more than one flow to choose between.
func RenderWeekly(flows []core.FlowView, selected string, width int) string {
	if len(flows) == 0 {
		return "no deploy flows\n"
	}
	flow := flows[0]
	for _, f := range flows {
		if f.Name == selected {
			flow = f
		}
	}

	var b strings.Builder
	if len(flows) > 1 {
		// chrome nil: no elevated background bar. This view is a static page
		// rather than a live header, so the strip sits on the terminal's own
		// background.
		b.WriteString(renderStrip(flows, flow.Name, nil, nil, width))
		b.WriteString("\n\n")
	}

	plotCols, barCols := weeklyLayout(width)
	axis := flow.LeadAxis
	if axis.Max <= 0 {
		// Unreachable from DeriveView, which never picks a non-positive axis,
		// but a hand-built FlowView would pile every glyph into column zero
		// and label every tick there. Fall back rather than draw nonsense.
		axis.Max = core.ChooseAxisMax(nil)
	}

	b.WriteString(weeklyHeader(plotCols, barCols, width))
	if len(flow.Weekly) == 0 {
		b.WriteString(dimStyle().Render(ClipRight("  no deploys recorded yet", width)))
		b.WriteString("\n")
		return b.String()
	}

	maxDeploys := 0
	for _, w := range flow.Weekly {
		if w.Deploys > maxDeploys {
			maxDeploys = w.Deploys
		}
	}

	for _, w := range flow.Weekly {
		b.WriteString(weeklyRow(w, axis, maxDeploys, plotCols, barCols, width))
		b.WriteString("\n")
	}
	b.WriteString(weeklyAxis(axis, plotCols, width))
	b.WriteString("\n")
	return b.String()
}

// weeklyLayout splits the available width between the plot and the bars. The
// plot gets the larger share: the lead time distribution is the headline, and
// a bar only has to be comparable to the bars above and below it.
func weeklyLayout(width int) (plotCols, barCols int) {
	available := width - rowChrome
	if available < minPlotCols+1 {
		available = minPlotCols + 1
	}
	plotCols = available * 2 / 3
	barCols = available - plotCols
	if plotCols < minPlotCols {
		plotCols = minPlotCols
	}
	if barCols < 1 {
		barCols = 1
	}
	return plotCols, barCols
}

func weeklyHeader(plotCols, barCols, width int) string {
	legend := "lead time  ·  median, p25–p75"
	if lipgloss.Width(legend) > plotCols {
		legend = "lead time"
	}
	if lipgloss.Width(legend) > plotCols {
		legend = ""
	}
	left := "  " + strings.Repeat(" ", weekLabelWidth) + legend

	right := "deploys"
	total := rowChrome + plotCols + barCols
	if total > width {
		total = width
	}
	gap := total - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		// No room for both; the legend is the one that can go, since the
		// deploy column has no other label.
		left = "  " + strings.Repeat(" ", weekLabelWidth)
		gap = total - lipgloss.Width(left) - lipgloss.Width(right)
	}
	if gap < 1 {
		return dimStyle().Render(ClipRight(left, width)) + "\n"
	}
	return dimStyle().Render(left+strings.Repeat(" ", gap)+right) + "\n"
}

func weeklyRow(w core.WeekStat, axis core.LeadAxis, maxDeploys, plotCols, barCols, width int) string {
	label := fmt.Sprintf("W%d-%02d", w.Year, w.Week)

	var plot string
	switch {
	case w.Leads.N == 0:
		note := "· no deploys"
		if plotCols < lipgloss.Width(note) {
			note = "·"
		}
		plot = dimStyle().Render(note)
	case w.Leads.N < core.SampleFloor:
		plot = renderLeadPoints(w.Leads, axis.Max, plotCols)
	default:
		plot = renderBoxPlot(w.Leads, axis.Max, plotCols)
	}
	pad := plotCols - lipgloss.Width(plot)
	if pad < 0 {
		plot, pad = ClipRight(plot, plotCols), 0
	}

	filled := barLength(w.Deploys, maxDeploys, barCols)
	count := fmt.Sprintf("%*d", countWidth, w.Deploys)
	if w.Deploys == 0 {
		count = dimStyle().Render(count)
	}

	row := fmt.Sprintf("  %-*s%s%s  %s%s%s",
		weekLabelWidth, label,
		plot, strings.Repeat(" ", pad),
		strings.Repeat(" ", barCols-filled),
		dimStyle().Render(strings.Repeat("█", filled)),
		count)
	// The layout floors the plot at a minimum, so a very narrow terminal
	// cannot be satisfied by shrinking columns alone. Clip rather than
	// overflow: a wrapped row ranks nothing, which is the one job the view
	// has at that size.
	return ClipRight(row, width)
}

// renderBoxPlot draws whiskers to the extremes, a block across the
// interquartile range, and a bright cap at the median.
//
// The whiskers stay because spread matters as much as the middle: a two-hour
// median with a three-day p75 is a problem the median alone hides.
func renderBoxPlot(q core.LeadQuantiles, axisMax time.Duration, w int) string {
	cells := blankCells(w)
	// When something is beyond the axis the arrow takes the final column and
	// the plot scales into what is left. Drawing it over the plot instead
	// erased whatever landed there — including the median, and including
	// values sitting exactly ON the axis, which are shown rather than
	// excluded. A lone arrow carries less than a squashed box.
	w = plotSpan(q, axisMax, w)
	if w < 1 {
		return strings.Join(cells, "")
	}

	lo := scaleTo(q.Min, axisMax, w)
	q1 := scaleTo(q.P25, axisMax, w)
	med := scaleTo(q.P50, axisMax, w)
	q3 := scaleTo(q.P75, axisMax, w)
	hi := scaleTo(q.Max, axisMax, w)

	for i := lo; i <= hi && i < w; i++ {
		cells[i] = dimStyle().Render("─")
	}
	cells[lo] = dimStyle().Render("├")
	cells[hi] = dimStyle().Render("┤")
	for i := q1; i <= q3 && i < w; i++ {
		cells[i] = boxStyle().Render("▓")
	}
	cells[med] = medianStyle().Render("█")

	// Strictly greater, so a value landing exactly on the axis is shown
	// rather than marked as excluded — and agrees with the "+" on the final
	// tick, which is set the same way.
	if q.Max > axisMax {
		cells[len(cells)-1] = clampStyle().Render("→")
	}
	return strings.Join(cells, "")
}

// plotSpan is how many columns the distribution itself may use: all of them,
// or one fewer when the clamp arrow needs the last.
func plotSpan(q core.LeadQuantiles, axisMax time.Duration, w int) int {
	if q.Max > axisMax {
		return w - 1
	}
	return w
}

// renderLeadPoints plots each deploy individually, for weeks with too few of
// them for quartiles to describe anything.
//
// The real samples, not the quantiles: below the floor those are
// interpolations between two or three values, and drawing them would put a
// smear on screen where there were only two deploys.
func renderLeadPoints(q core.LeadQuantiles, axisMax time.Duration, w int) string {
	cells := blankCells(w)
	span := plotSpan(q, axisMax, w)
	if span < 1 {
		return strings.Join(cells, "")
	}
	for _, d := range q.Samples {
		cells[scaleTo(d, axisMax, span)] = boxStyle().Render("▫")
	}
	if q.Max > axisMax {
		cells[len(cells)-1] = clampStyle().Render("→")
	}
	return strings.Join(cells, "")
}

func blankCells(w int) []string {
	cells := make([]string, w)
	for i := range cells {
		cells[i] = " "
	}
	return cells
}

// scaleTo maps a duration onto a column, clamping past the axis to the last
// one — which is where the arrow is drawn.
func scaleTo(d, axisMax time.Duration, w int) int {
	if axisMax <= 0 || w <= 1 {
		return 0
	}
	c := int(float64(d) / float64(axisMax) * float64(w-1))
	if c < 0 {
		return 0
	}
	if c > w-1 {
		return w - 1
	}
	return c
}

func barLength(n, max, w int) int {
	if max <= 0 || n <= 0 {
		return 0
	}
	filled := int(math.Round(float64(n) / float64(max) * float64(w)))
	if filled < 1 {
		filled = 1
	}
	if filled > w {
		filled = w
	}
	return filled
}

// weeklyAxis labels the shared scale at quarters, shedding intermediate ticks
// before they collide — the same degradation order the week stats use.
func weeklyAxis(axis core.LeadAxis, w, width int) string {
	type tick struct {
		at    int
		label string
	}
	fractions := []float64{0, 0.25, 0.5, 0.75, 1}
	if w < 26 {
		fractions = []float64{0, 0.5, 1}
	}
	if w < 16 {
		fractions = []float64{0, 1}
	}

	cells := make([]string, w)
	for i := range cells {
		cells[i] = "─"
	}
	for _, f := range fractions {
		d := time.Duration(float64(axis.Max) * f)
		label := formatAxisTick(d)
		if f == 1 && axis.Clamped {
			label += "+"
		}
		at := scaleTo(d, axis.Max, w)
		if at+len(label) > w {
			at = w - len(label) // right-align the last tick rather than clip it
		}
		for j, r := range label {
			if at+j >= 0 && at+j < w {
				cells[at+j] = string(r)
			}
		}
	}
	return ClipRight("  "+strings.Repeat(" ", weekLabelWidth)+
		dimStyle().Render(strings.Join(cells, "")), width)
}

// formatAxisTick keeps the quarter marks distinct. Days lose too much
// precision at the quarters — a 2d axis would label 1d twice.
func formatAxisTick(d time.Duration) string {
	switch {
	case d == 0:
		return "0"
	case d >= 72*time.Hour:
		days := d.Hours() / 24
		if days == math.Trunc(days) {
			return fmt.Sprintf("%dd", int(days))
		}
		return fmt.Sprintf("%.1fd", days)
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(math.Round(d.Hours())))
	default:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
}

func dimStyle() lipgloss.Style    { return lipgloss.NewStyle().Foreground(colorGray) }
func boxStyle() lipgloss.Style    { return lipgloss.NewStyle().Foreground(colorBlue) }
func medianStyle() lipgloss.Style { return lipgloss.NewStyle().Foreground(colorBlue).Bold(true) }
func clampStyle() lipgloss.Style  { return lipgloss.NewStyle().Foreground(colorYellowLight) }

// DimNotice renders a footnote in the same dim weight as the view's chrome,
// clipped to the width it was given.
func DimNotice(text string, width int) string {
	return dimStyle().Render(ClipRight(text, width))
}
