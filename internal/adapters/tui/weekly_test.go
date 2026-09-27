package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/ezcdlabs/clarity/internal/adapters/tui"
	"github.com/ezcdlabs/clarity/internal/core"
)

func dur(h float64) time.Duration { return time.Duration(h * float64(time.Hour)) }

// weekOf builds a WeekStat with a distribution, so the box plot has something
// to draw.
func weekOf(year, week, deploys int, leads ...float64) core.WeekStat {
	ds := make([]time.Duration, 0, len(leads))
	for _, l := range leads {
		ds = append(ds, dur(l))
	}
	q := core.Quantiles(ds)
	return core.WeekStat{Year: year, Week: week, Deploys: deploys, Leads: q, AvgLead: q.P50}
}

func flowWith(name string, weeks ...core.WeekStat) core.FlowView {
	var pooled []time.Duration
	for _, w := range weeks {
		if w.Leads.N > 0 {
			pooled = append(pooled, w.Leads.Min, w.Leads.Max)
		}
	}
	return core.FlowView{
		Flow:     core.Flow{Name: name},
		Weekly:   weeks,
		Deploy:   "passed",
		LeadAxis: core.LeadAxis{Max: core.ChooseAxisMax(pooled)},
	}
}

// plainLines strips styling so assertions are about layout and content rather
// than escape sequences.
func plainLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		out = append(out, stripANSI(line))
	}
	return out
}

func stripANSI(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			in = true
		case in && r == 'm':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// TestRenderWeekly_ShowsAWeekPerRow is the shape of the thing: one row per
// ISO week, newest first, each carrying its deploy count.
func TestRenderWeekly_ShowsAWeekPerRow(t *testing.T) {
	f := flowWith("deploy",
		weekOf(2026, 39, 18, 1, 2, 3, 4, 5, 6),
		weekOf(2026, 38, 24, 1, 1.5, 2, 2.5, 3, 4),
	)
	out := tui.RenderWeekly([]core.FlowView{f}, "deploy", 100)

	for _, want := range []string{"W2026-39", "W2026-38", "18", "24"} {
		if !strings.Contains(stripANSI(out), want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	if i, j := strings.Index(out, "W2026-39"), strings.Index(out, "W2026-38"); i > j {
		t.Error("weeks should read newest first")
	}
}

// TestRenderWeekly_NeverExceedsTheWidth pins that every line fits. The view is
// a grid of aligned columns; one overflowing row wraps and destroys the
// alignment that makes the rows comparable.
func TestRenderWeekly_NeverExceedsTheWidth(t *testing.T) {
	f := flowWith("deploy",
		weekOf(2026, 39, 180, 1, 2, 3, 4, 5, 60),
		weekOf(2026, 38, 4, 0.5, 1, 1.5, 2),
		weekOf(2026, 37, 0),
	)
	for _, width := range []int{120, 100, 80, 60, 48, 40} {
		out := tui.RenderWeekly([]core.FlowView{f}, "deploy", width)
		for _, line := range plainLines(out) {
			if got := len([]rune(line)); got > width {
				t.Errorf("width %d: line of %d runes overflows:\n%q", width, got, line)
			}
		}
	}
}

// TestRenderWeekly_EmptyWeekIsNotAZeroLengthPlot verifies a week with no
// deploys says so, rather than drawing a box plot of nothing.
func TestRenderWeekly_EmptyWeekIsNotAZeroLengthPlot(t *testing.T) {
	f := flowWith("deploy",
		weekOf(2026, 39, 12, 1, 2, 3, 4, 5, 6),
		weekOf(2026, 38, 0),
	)
	out := stripANSI(tui.RenderWeekly([]core.FlowView{f}, "deploy", 100))

	var emptyRow string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "W2026-38") {
			emptyRow = line
		}
	}
	if emptyRow == "" {
		t.Fatalf("no row for the empty week:\n%s", out)
	}
	if strings.ContainsAny(emptyRow, "▓█├┤") {
		t.Errorf("the empty week drew plot glyphs: %q", emptyRow)
	}
}

// TestRenderWeekly_TooFewDeploysPlotsPointsNotQuartiles verifies the small-n
// fallback. Quartiles of two numbers are theatre; the points are honest.
func TestRenderWeekly_TooFewDeploysPlotsPointsNotQuartiles(t *testing.T) {
	f := flowWith("deploy",
		weekOf(2026, 39, 12, 1, 2, 3, 4, 5, 6),
		weekOf(2026, 38, 2, 3, 4),
	)
	out := stripANSI(tui.RenderWeekly([]core.FlowView{f}, "deploy", 100))

	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "W2026-38") {
			continue
		}
		if strings.ContainsAny(line, "├┤") {
			t.Errorf("two deploys drew a box plot: %q", line)
		}
		return
	}
	t.Fatalf("no row for the sparse week:\n%s", out)
}

// TestRenderWeekly_ShowsTheDeployStripForSeveralFlows verifies the tab bar is
// rendered and marks which flow is on screen — the whole reason the view is
// interactive rather than a one-shot print.
func TestRenderWeekly_ShowsTheDeployStripForSeveralFlows(t *testing.T) {
	web := flowWith("web", weekOf(2026, 39, 10, 1, 2, 3, 4))
	ios := flowWith("ios", weekOf(2026, 39, 3, 40, 50, 60))

	out := stripANSI(tui.RenderWeekly([]core.FlowView{web, ios}, "ios", 100))
	if !strings.Contains(out, "web") || !strings.Contains(out, "ios") {
		t.Errorf("both flows should appear in the strip:\n%s", out)
	}
}

// TestWeeklyModel_TabSwitchesFlow is the reason this view is a Bubble Tea
// program rather than a one-shot print: the deploy strip is a control, and a
// tab bar that cannot be pressed is not one.
func TestWeeklyModel_TabSwitchesFlow(t *testing.T) {
	view := core.View{Flows: []core.FlowView{
		flowWith("web", weekOf(2026, 39, 5, 1, 2, 3)),
		flowWith("ios", weekOf(2026, 39, 2, 40, 50)),
		flowWith("android", weekOf(2026, 39, 3, 20, 30, 40)),
	}}
	m := tui.NewWeeklyModel(view, "", 100, 30)

	if got := m.SelectedFlow(); got != "web" {
		t.Fatalf("opens on %q, want the first flow", got)
	}
	for _, step := range []struct {
		key  string
		want string
	}{
		{"tab", "ios"},
		{"tab", "android"},
		{"tab", "web"}, // wraps
		{"shift+tab", "android"},
		{"3", "android"},
		{"1", "web"},
	} {
		next, _ := m.Update(keyPress(step.key))
		m = next.(tui.WeeklyModel)
		if got := m.SelectedFlow(); got != step.want {
			t.Fatalf("after %q: selected %q, want %q", step.key, got, step.want)
		}
	}
}

// TestWeeklyModel_OpensOnTheRequestedFlow covers --deploy.
func TestWeeklyModel_OpensOnTheRequestedFlow(t *testing.T) {
	view := core.View{Flows: []core.FlowView{
		flowWith("web", weekOf(2026, 39, 5, 1, 2, 3)),
		flowWith("ios", weekOf(2026, 39, 2, 40, 50)),
	}}
	m := tui.NewWeeklyModel(view, "ios", 100, 30)
	if got := m.SelectedFlow(); got != "ios" {
		t.Errorf("selected %q, want ios", got)
	}
}

// TestWeeklyModel_RendersTheSelectedFlowsNumbers guards the seam where the
// strip and the body can disagree: switching tabs has to change the rows, not
// only the highlight.
func TestWeeklyModel_RendersTheSelectedFlowsNumbers(t *testing.T) {
	view := core.View{Flows: []core.FlowView{
		flowWith("web", weekOf(2026, 39, 17, 1, 2, 3)),
		flowWith("ios", weekOf(2026, 39, 4, 40, 50, 60, 70)),
	}}
	m := tui.NewWeeklyModel(view, "", 100, 30)

	if body := stripANSI(m.View().Content); !strings.Contains(body, "17") {
		t.Errorf("web's deploy count is missing:\n%s", body)
	}
	next, _ := m.Update(keyPress("tab"))
	m = next.(tui.WeeklyModel)
	body := stripANSI(m.View().Content)
	if !strings.Contains(body, " 4") {
		t.Errorf("after switching, ios's deploy count is missing:\n%s", body)
	}
	if strings.Contains(body, "17") {
		t.Errorf("after switching, web's numbers are still on screen:\n%s", body)
	}
}

// keyPress builds the KeyPressMsg for a key name, mirroring how Bubble Tea
// reports it — tab carries a code, a digit carries its rune and text.
func keyPress(name string) tea.KeyPressMsg {
	switch name {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	}
	r := []rune(name)[0]
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}
