package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	out := tui.RenderWeekly(core.View{Flows: []core.FlowView{f}}, "deploy", 100)

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
	// Down to zero: the header is clipped to the requested width, so the rows
	// have to be too, or the two disagree and every row wraps.
	for _, width := range []int{120, 100, 80, 60, 48, 40, 30, 26, 20, 10, 1, 0} {
		out := tui.RenderWeekly(core.View{Flows: []core.FlowView{f}}, "deploy", width)
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
	out := stripANSI(tui.RenderWeekly(core.View{Flows: []core.FlowView{f}}, "deploy", 100))

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
	out := stripANSI(tui.RenderWeekly(core.View{Flows: []core.FlowView{f}}, "deploy", 100))

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

	out := stripANSI(tui.RenderWeekly(core.View{Flows: []core.FlowView{web, ios}}, "ios", 100))
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

// TestTruncateName_WideRunes covers a flow name whose runes are wider than
// one column.
//
// The budget is a display width and the name was sliced by rune index, which
// for wide runes differ by a factor of two. Slicing within capacity injected
// NUL runes into the output; slicing past it panicked, taking down both this
// view and the live TUI. Any repo with a CJK deploy target reached it.
func TestTruncateName_WideRunes(t *testing.T) {
	names := []string{
		"デプロイ",
		"配置到生产环境服务器集群",
		"ios",
		"a-very-long-ascii-flow-name-indeed",
		"мобильное-приложение",
		"",
	}
	for _, name := range names {
		for max := 0; max <= 30; max++ {
			got := tui.TruncateNameForTest(name, max)
			if strings.ContainsRune(got, 0) {
				t.Fatalf("truncateName(%q, %d) injected NUL runes: %q", name, max, got)
			}
			if w := lipgloss.Width(got); w > max {
				t.Fatalf("truncateName(%q, %d) returned %q, %d columns wide", name, max, got, w)
			}
		}
	}
}

// TestRenderWeekly_WideRuneFlowName drives the same case through the view, so
// the strip's own budget arithmetic is exercised rather than the helper alone.
func TestRenderWeekly_WideRuneFlowName(t *testing.T) {
	wide := flowWith("配置到生产环境服务器集群", weekOf(2026, 39, 5, 1, 2, 3))
	other := flowWith("ios", weekOf(2026, 39, 2, 40, 50))
	for _, width := range []int{120, 100, 80, 60, 40} {
		out := tui.RenderWeekly(core.View{Flows: []core.FlowView{wide, other}}, "ios", width)
		if strings.ContainsRune(out, 0) {
			t.Errorf("width %d: NUL runes in output", width)
		}
	}
}

// TestRenderWeekly_ClampArrowKeepsTheMedian covers a week much slower than
// the shared axis.
//
// The arrow was written to the last column unconditionally, after the box and
// the median. When the median also scaled to that column it was destroyed and
// the row became a lone arrow — strictly less information than a squashed
// box, and it erased values that sit exactly ON the axis, which the code
// deliberately shows rather than marks as excluded.
func TestRenderWeekly_ClampArrowKeepsTheMedian(t *testing.T) {
	slow := core.FlowView{
		Flow:     core.Flow{Name: "deploy"},
		LeadAxis: core.LeadAxis{Max: dur(10), Clamped: true},
		Weekly: []core.WeekStat{{
			Year: 2026, Week: 39, Deploys: 6,
			// Spread, so the box has extent and the upper whisker reaches
			// the axis — the cell the arrow used to overwrite. All-identical
			// values would collapse the plot to a single cell and prove
			// nothing about the arrow.
			Leads: core.Quantiles([]time.Duration{
				dur(2), dur(4), dur(6), dur(8), dur(10), dur(100),
			}),
		}},
	}
	row := ""
	for _, line := range plainLines(tui.RenderWeekly(core.View{Flows: []core.FlowView{slow}}, "deploy", 100)) {
		if strings.Contains(line, "W2026-39") {
			row = line
		}
	}
	if row == "" {
		t.Fatal("no week row rendered")
	}
	if !strings.Contains(row, "→") {
		t.Errorf("a lead beyond the axis should be marked: %q", row)
	}
	// On ▓ specifically, not on "█▓": the deploy bar on the same row is drawn
	// with █, so the looser assertion was satisfied by the bar no matter what
	// happened to the plot — a test that could not fail.
	if !strings.Contains(row, "▓") {
		t.Errorf("the arrow erased the distribution: %q", row)
	}
	// The upper whisker is the cell the arrow used to take. Its survival is
	// the precise property: the plot scales into one fewer column so the
	// arrow has one of its own.
	if !strings.Contains(row, "┤") {
		t.Errorf("the arrow overwrote the end of the whisker: %q", row)
	}

	// The scale has to admit it is incomplete too. The row arrow says this
	// week ran off the end; the "+" says the axis does not cover everything.
	// Without it a reader takes the final tick as the true maximum.
	var axisLine string
	for _, line := range plainLines(tui.RenderWeekly(core.View{Flows: []core.FlowView{slow}}, "deploy", 100)) {
		if strings.Contains(line, "0─") {
			axisLine = line
		}
	}
	if !strings.Contains(axisLine, "+") {
		t.Errorf("the axis excludes data but its final tick does not say so: %q", axisLine)
	}
}

// TestRenderWeekly_HeaderAlignsWithTheRows pins that "deploys" sits over the
// column it names. They were computed from two different totals, so the label
// hung one column past the counts.
func TestRenderWeekly_HeaderAlignsWithTheRows(t *testing.T) {
	f := flowWith("deploy",
		weekOf(2026, 39, 18, 1, 2, 3, 4, 5, 6),
		weekOf(2026, 38, 7, 1, 2, 3, 4, 5, 6),
	)
	for _, width := range []int{100, 80, 60} {
		lines := plainLines(tui.RenderWeekly(core.View{Flows: []core.FlowView{f}}, "deploy", width))
		var header, row string
		for _, l := range lines {
			if strings.Contains(l, "deploys") {
				header = l
			}
			if strings.Contains(l, "W2026-39") {
				row = l
			}
		}
		if header == "" || row == "" {
			t.Fatalf("width %d: missing header or row", width)
		}
		if len([]rune(strings.TrimRight(header, " "))) > len([]rune(strings.TrimRight(row, " "))) {
			t.Errorf("width %d: header (%d) extends past the row it labels (%d)",
				width, len([]rune(strings.TrimRight(header, " "))),
				len([]rune(strings.TrimRight(row, " "))))
		}
	}
}

// TestWeeklyModel_ScrollingStopsAtTheOldestWeek covers running off the end.
//
// The offset was unclamped, so holding a scroll key walked past the history
// and left the view rendering "no deploys recorded yet" — telling the reader
// a repo full of deploys has none — and needed as many presses to get back as
// it took to leave.
func TestWeeklyModel_ScrollingStopsAtTheOldestWeek(t *testing.T) {
	// More weeks than fit, so there is genuinely something to scroll.
	var weeks []core.WeekStat
	for i := 0; i < 20; i++ {
		weeks = append(weeks, weekOf(2026, 40-i, 5, 1, 2, 3, 4, 5, 6))
	}
	view := core.View{Flows: []core.FlowView{flowWith("deploy", weeks...)}}
	m := tui.NewWeeklyModel(view, "", 100, 12)

	for i := 0; i < 50; i++ {
		next, _ := m.Update(keyPress("j"))
		m = next.(tui.WeeklyModel)
	}
	body := stripANSI(m.View().Content)
	if strings.Contains(body, "no deploys recorded yet") {
		t.Fatalf("scrolled past the end into an empty view:\n%s", body)
	}
	if !strings.Contains(body, "W2026-21") {
		t.Errorf("the oldest week should be on screen at the end of the scroll:\n%s", body)
	}

	// And one press back up must move, rather than spending the overshoot.
	next, _ := m.Update(keyPress("k"))
	m = next.(tui.WeeklyModel)
	if got := stripANSI(m.View().Content); got == body {
		t.Error("scrolling up after hitting the end did nothing; the offset " +
			"kept growing past the history")
	}
}

// TestWeeklyModel_FooterMentionsHowToGetBack verifies the reset key is
// advertised wherever scrolling is.
func TestWeeklyModel_FooterMentionsHowToGetBack(t *testing.T) {
	var weeks []core.WeekStat
	for i := 0; i < 30; i++ {
		weeks = append(weeks, weekOf(2026, 39-i, 5, 1, 2, 3, 4, 5, 6))
	}
	view := core.View{Flows: []core.FlowView{flowWith("deploy", weeks...)}}
	m := tui.NewWeeklyModel(view, "", 100, 12)

	footer := stripANSI(m.View().Content)
	if !strings.Contains(footer, "scroll") {
		t.Fatalf("a history longer than the screen should advertise scrolling:\n%s", footer)
	}
	if !strings.Contains(footer, "g") {
		t.Errorf("scrolling is advertised but the way back to the top is not:\n%s", footer)
	}
}

// TestRenderWeekly_GoldenLayout pins the exact columns at a fixed width.
//
// Every other renderer test asserts that a glyph is *present*. None of them
// asserts where it sits, which left every scaling and layout decision
// unverified — the median could be drawn at p25, the bars could grow from the
// left instead of sharing a right edge, the clamp arrow could vanish, and the
// suite stayed green.
//
// A golden comparison is brittle by design: it fails whenever the layout
// changes, which is the point. Regenerate it deliberately, and read the diff
// as a description of what moved.
func TestRenderWeekly_GoldenLayout(t *testing.T) {
	want := []string{
		"app  ·  ci: ✓  ·  deploy: ✓                                                                         ",
		"",
		"           lead time  ·  median, p25–p75                                                     deploys",
		"  W2026-40     ├────▓▓▓▓▓▓█▓▓▓▓▓▓▓▓▓▓▓▓▓▓────────────────────────┤         █████████████████████  18",
		"  W2026-39     ├─▓█▓▓─┤                                             ████████████████████████████  24",
		"  W2026-38                   ▫        ▫        ▫                                            ████   3",
		"  W2026-37 · no deploys                                                                            0",
		"  W2026-36          ├────▓▓▓▓▓▓▓▓█▓▓▓▓▓▓▓▓▓▓▓───────────────┤                        ███████████   9",
		"           0────────────3h────────────6h───────────9h──────────12h",
		"",
	}

	got := plainLines(tui.RenderWeekly(goldenView(), "deploy", 100))
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d:\n got %q\nwant %q", i, got[i], want[i])
		}
	}
}

// TestRenderWeekly_GoldenClampedRow pins the exact columns of a week that
// runs off the axis — the case the main golden fixture does not reach, and
// the one where the arrow and the plot compete for the last column.
func TestRenderWeekly_GoldenClampedRow(t *testing.T) {
	f := core.FlowView{
		Flow:     core.Flow{Name: "deploy"},
		LeadAxis: core.LeadAxis{Max: dur(8), Clamped: true},
		Weekly: []core.WeekStat{{
			Year: 2026, Week: 39, Deploys: 6,
			Leads: core.Quantiles([]time.Duration{
				dur(2), dur(3), dur(4), dur(6), dur(8), dur(40),
			}),
		}},
	}
	want := []string{
		"app  ·  ci: ✓  ·  deploy: ✓                                                                         ",
		"",
		"           lead time  ·  median, p25–p75                                                     deploys",
		"  W2026-39              ├───────▓▓▓▓▓▓▓▓▓▓▓▓█▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓───┤→  ████████████████████████████   6",
		"           0────────────2h────────────4h───────────6h──────────8h+",
		"",
	}
	view := goldenView()
	view.Flows = []core.FlowView{f}
	got := plainLines(tui.RenderWeekly(view, "deploy", 100))
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d:\n got %q\nwant %q", i, got[i], want[i])
		}
	}
}

// TestRenderWeekly_PlotsEverySample verifies a sparse week draws one mark per
// deploy — not five marks interpolated from quantiles, which is the smear the
// sample floor exists to avoid.
func TestRenderWeekly_PlotsEverySample(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4} {
		leads := make([]float64, n)
		for i := range leads {
			leads[i] = float64(2 + i*3) // well separated, so marks cannot overlap
		}
		f := flowWith("deploy", weekOf(2026, 39, n, leads...))
		var row string
		for _, line := range plainLines(tui.RenderWeekly(core.View{Flows: []core.FlowView{f}}, "deploy", 100)) {
			if strings.Contains(line, "W2026-39") {
				row = line
			}
		}
		if got := strings.Count(row, "▫"); got != n {
			t.Errorf("%d deploys drew %d marks: %q", n, got, row)
		}
	}
}

// TestRenderWeekly_AlwaysHasATopBar covers a repo with no deploy targets.
//
// The header used to be the deploy strip and nothing else, so a repo with one
// flow — which is most repos — got no top bar at all and the view opened on a
// bare column heading. The strip is what varies with targets; the bar is not.
func TestRenderWeekly_AlwaysHasATopBar(t *testing.T) {
	one := core.View{
		Snapshot: core.Snapshot{RepoName: "api"},
		Header:   core.HeaderStatus{CI: "passed", Deploy: "passed"},
		Flows:    []core.FlowView{flowWith("deploy", weekOf(2026, 39, 5, 1, 2, 3, 4, 5, 6))},
	}
	first := plainLines(tui.RenderWeekly(one, "deploy", 100))[0]
	if !strings.Contains(first, "api") {
		t.Errorf("a single-flow repo should still be named in the top bar: %q", first)
	}

	many := one
	many.Flows = []core.FlowView{
		flowWith("web", weekOf(2026, 39, 5, 1, 2, 3, 4, 5, 6)),
		flowWith("ios", weekOf(2026, 39, 2, 40, 50)),
	}
	firstMany := plainLines(tui.RenderWeekly(many, "ios", 100))[0]
	for _, want := range []string{"api", "web", "ios"} {
		if !strings.Contains(firstMany, want) {
			t.Errorf("the top bar should carry %q with targets present: %q", want, firstMany)
		}
	}
}

// TestWeeklyModel_FooterOmitsTheQuitKey verifies the footer lists only keys
// that change what is on screen. Every terminal program quits; saying so
// spends a line on the one hint carrying no information.
func TestWeeklyModel_FooterOmitsTheQuitKey(t *testing.T) {
	view := core.View{
		Snapshot: core.Snapshot{RepoName: "api"},
		Flows:    []core.FlowView{flowWith("deploy", weekOf(2026, 39, 5, 1, 2, 3, 4, 5, 6))},
	}
	body := stripANSI(tui.NewWeeklyModel(view, "", 100, 30).View().Content)
	if strings.Contains(body, "quit") {
		t.Errorf("the footer still advertises quitting:\n%s", body)
	}
}
