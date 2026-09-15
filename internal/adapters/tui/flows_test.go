package tui_test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/adapters/tui"
	"github.com/ezcdlabs/clarity/internal/core"
)

// ansiPattern matches the escapes lipgloss emits. Stripping them is not
// optional when asserting on styled text: a bold+underlined name is rendered
// one SGR pair *per character*, so the name never appears as a contiguous
// substring and a bare strings.Contains silently never matches.
var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

func plainText(s string) string { return ansiPattern.ReplaceAllString(s, "") }

func headerOf(m tui.Model) string {
	return plainText(strings.SplitN(m.View().Content, "\n", 2)[0])
}

func depEv(target string, ts int64) clarityrefs.Event {
	return clarityrefs.Event{Stage: "deploy", Status: "passed", Time: time.Unix(ts, 0), Target: target}
}

// twoFlowView is the shape the strip exists for: the newest commit has shipped
// via the untargeted deploy but not to ios, so which section it renders under
// depends entirely on which flow is selected.
func twoFlowView() core.View {
	snap := core.Snapshot{
		RepoName: "clarity",
		Commits: []core.CommitView{
			{SHA: "a", Author: "alice", Subject: "checkout fix", Time: time.Unix(1000, 0),
				Events: []clarityrefs.Event{ev("ci", "passed", 1100), depEv("", 1200)}},
			{SHA: "b", Author: "bob", Subject: "target sdk", Time: time.Unix(800, 0),
				Events: []clarityrefs.Event{ev("ci", "passed", 850), depEv("ios", 900)}},
		},
	}
	return core.DeriveView(snap, core.DefaultLeadTimeMode, nil)
}

func press(m tui.Model, key string) tui.Model {
	next, _ := m.Update(tui.KeyMsg(key))
	return next.(tui.Model)
}

func TestModel_StripNamesEveryFlow(t *testing.T) {
	m, _ := newModel("clarity").Update(tui.ViewMsg(twoFlowView()))
	header := headerOf(m.(tui.Model))

	for _, name := range []string{"deploy", "ios"} {
		if !strings.Contains(header, name) {
			t.Errorf("strip does not name flow %q:\n%s", name, header)
		}
	}
}

// Each cell carries its flow's own deploy status. Without it a tab is just a
// name, and the whole repo's health stops being visible from the strip.
func TestModel_StripShowsPerFlowStatus(t *testing.T) {
	snap := core.Snapshot{
		RepoName: "clarity",
		Commits: []core.CommitView{
			{SHA: "a", Author: "alice", Subject: "x", Time: time.Unix(1000, 0),
				Events: []clarityrefs.Event{
					{Stage: "deploy", Status: "passed", Time: time.Unix(1100, 0)},
					{Stage: "deploy", Status: "failed", Time: time.Unix(1200, 0), Target: "ios"},
				}},
		},
	}
	m, _ := newModel("clarity").Update(tui.ViewMsg(core.DeriveView(snap, core.DefaultLeadTimeMode, nil)))
	header := headerOf(m.(tui.Model))

	if !strings.Contains(header, "✓") {
		t.Errorf("strip lost the passing flow's status:\n%s", header)
	}
	if !strings.Contains(header, "✗") {
		t.Errorf("strip lost the failing flow's status — one flow's red is the\n"+
			"whole point of seeing every tab at once:\n%s", header)
	}
}

// A repo with one flow must render exactly what it always did — no strip, no
// extra chrome. This is the promise that keeps targets free for everyone who
// doesn't use them.
func TestModel_SingleFlowRendersNoStrip(t *testing.T) {
	snap := core.Snapshot{
		RepoName: "clarity",
		Commits: []core.CommitView{
			{SHA: "a", Author: "alice", Subject: "x", Events: []clarityrefs.Event{ev("deploy", "passed", 100)}},
		},
	}
	m, _ := newModel("clarity").Update(tui.ViewMsg(core.DeriveView(snap, core.DefaultLeadTimeMode, nil)))
	header := headerOf(m.(tui.Model))

	// One flow means the pre-targets header: a single `deploy:` badge and no
	// strip. Two mentions would mean the group label plus a cell, i.e. a strip
	// rendered for a repo with nothing to switch between.
	if strings.Count(header, "deploy") != 1 {
		t.Errorf("single-flow header should mention deploy exactly once, got:\n%q", header)
	}
}

// Selecting a flow changes what the body groups by. This is the whole feature:
// the same commit is deployed in one flow and still pending in another.
func TestModel_BodyFollowsTheSelectedFlow(t *testing.T) {
	m, _ := newModel("clarity").Update(tui.ViewMsg(twoFlowView()))
	model := m.(tui.Model)

	sectionOf := func(out, subject string) string {
		section := ""
		for _, line := range strings.Split(out, "\n") {
			for _, s := range []string{"HEAD", "CI Passed", "Deployed"} {
				if strings.Contains(line, s) {
					section = s
				}
			}
			if strings.Contains(line, subject) {
				return section
			}
		}
		return ""
	}

	first := model.View().Content
	if got := sectionOf(first, "checkout fix"); got != "Deployed" {
		t.Errorf("default flow: newest commit in %q, want Deployed\n%s", got, first)
	}

	second := press(model, "2").View().Content
	if got := sectionOf(second, "checkout fix"); got == "Deployed" {
		t.Errorf("ios flow: newest commit shown as Deployed, but no ios deploy shipped it\n%s", second)
	}
}

// Selection is tracked by flow name, not index. A config change or a newly
// discovered target reorders the strip, and the user must not find themselves
// looking at a different flow than the one they chose.
func TestModel_SelectionSurvivesReordering(t *testing.T) {
	m, _ := newModel("clarity").Update(tui.ViewMsg(twoFlowView()))
	model := press(m.(tui.Model), "2")

	if got := model.SelectedFlow(); got != "ios" {
		t.Fatalf("selected %q after pressing 2, want ios", got)
	}

	// A third flow appears and sorts ahead of ios, shifting its index.
	snap := core.Snapshot{
		RepoName: "clarity",
		Commits: []core.CommitView{
			{SHA: "a", Author: "alice", Subject: "checkout fix", Events: []clarityrefs.Event{depEv("", 1200)}},
			{SHA: "b", Author: "bob", Subject: "target sdk", Events: []clarityrefs.Event{depEv("ios", 900)}},
			{SHA: "c", Author: "cara", Subject: "droid", Events: []clarityrefs.Event{depEv("android", 800)}},
		},
	}
	next, _ := model.Update(tui.ViewMsg(core.DeriveView(snap, core.DefaultLeadTimeMode, nil)))

	if got := next.(tui.Model).SelectedFlow(); got != "ios" {
		t.Errorf("selection moved to %q after the strip reordered, want ios", got)
	}
}

// tab cycles forward and wraps, so the strip is navigable without knowing how
// many flows there are.
func TestModel_TabCyclesFlows(t *testing.T) {
	m, _ := newModel("clarity").Update(tui.ViewMsg(twoFlowView()))
	model := m.(tui.Model)

	if got := press(model, "tab").SelectedFlow(); got != "ios" {
		t.Errorf("tab selected %q, want ios", got)
	}
	if got := press(press(model, "tab"), "tab").SelectedFlow(); got != "deploy" {
		t.Errorf("tab did not wrap back to the first flow, got %q", got)
	}
}

// An index key beyond the last flow must do nothing rather than blank the body.
func TestModel_OutOfRangeSelectionIsIgnored(t *testing.T) {
	m, _ := newModel("clarity").Update(tui.ViewMsg(twoFlowView()))
	model := m.(tui.Model)

	if got := press(model, "9").SelectedFlow(); got != "deploy" {
		t.Errorf("pressing 9 with two flows changed selection to %q", got)
	}
}

// --- chrome ------------------------------------------------------------------

// The strip is an elevated chrome bar with the selected flow cut out of it, so
// the active tab is the only thing on the row sharing the body's background.
// That is what makes it read as a tab connected to the content below rather
// than a highlight floating in the header.
func TestModel_ChromeBarCutsOutTheSelectedFlow(t *testing.T) {
	bg := lipgloss.Color("#0d1117")
	m := tui.New().WithSize(100, 40).WithBackground(bg)
	next, _ := m.Update(tui.ViewMsg(twoFlowView()))
	header := strings.SplitN(next.(tui.Model).View().Content, "\n", 2)[0]

	chrome := lipgloss.Lighten(bg, 0.10)
	chromeSeq := lipgloss.NewStyle().Background(chrome).Render(" ")
	baseSeq := lipgloss.NewStyle().Background(bg).Render(" ")

	if !strings.Contains(header, ansiOf(baseSeq)) {
		t.Errorf("selected flow is not cut out to the body background:\n%q", header)
	}

	// The bar must run the full width, not just behind the cells — otherwise
	// the strip is a highlight floating in an unpainted header rather than a
	// tab cut out of chrome. The quit hint sits past the last cell, so chrome
	// reaching it is the evidence the row itself is painted.
	// A long run of chrome-painted spaces only occurs in the padding between
	// the strip and the quit hint — cells pad by one column at a time. That
	// run is what distinguishes a painted row from cells floating on a bare
	// one, and the bare version is a highlight, not a tab cut out of anything.
	run := ansiOf(chromeSeq) + strings.Repeat(" ", 10)
	if !strings.Contains(header, run) {
		t.Errorf("chrome does not extend past the strip across the row:\n%q", header)
	}
}

// Without a known terminal background there is nothing to derive an elevation
// from, so the strip must still be usable rather than guessing a colour that
// fights the user's theme.
func TestModel_WithoutABackgroundTheStripStillMarksSelection(t *testing.T) {
	m := tui.New().WithSize(100, 40).WithBackground(nil)
	next, _ := m.Update(tui.ViewMsg(twoFlowView()))
	model := next.(tui.Model)

	header := headerOf(model)
	if !strings.Contains(header, "deploy") || !strings.Contains(header, "ios") {
		t.Errorf("fallback strip lost a flow name:\n%q", header)
	}

	// Selection must still be visible and still move. Compared with the
	// escapes intact, because in this path selection is carried *by* the
	// styling — stripping it would compare two identical strings.
	styled := strings.SplitN(model.View().Content, "\n", 2)[0]
	after := strings.SplitN(press(model, "2").View().Content, "\n", 2)[0]
	if styled == after {
		t.Error("fallback strip renders identically whichever flow is selected")
	}
}

// A single-flow repo gets no chrome at all: a bar behind a header with nothing
// to cut out of it is weight for no information.
func TestModel_SingleFlowGetsNoChromeBar(t *testing.T) {
	bg := lipgloss.Color("#0d1117")
	snap := core.Snapshot{
		RepoName: "clarity",
		Commits: []core.CommitView{
			{SHA: "a", Author: "alice", Subject: "x", Events: []clarityrefs.Event{ev("deploy", "passed", 100)}},
		},
	}
	m := tui.New().WithSize(100, 40).WithBackground(bg)
	next, _ := m.Update(tui.ViewMsg(core.DeriveView(snap, core.DefaultLeadTimeMode, nil)))
	styled := strings.SplitN(next.(tui.Model).View().Content, "\n", 2)[0]

	chromeSeq := ansiOf(lipgloss.NewStyle().Background(lipgloss.Lighten(bg, 0.10)).Render(" "))
	if strings.Contains(styled, chromeSeq) {
		t.Errorf("single-flow header painted a chrome bar:\n%q", styled)
	}
	if strings.Count(plainText(styled), "deploy") != 1 {
		t.Errorf("single-flow header rendered a strip:\n%q", plainText(styled))
	}
}

// ansiOf returns just the escape prefix of a rendered cell, so a test can look
// for "was this background applied" without depending on the cell contents.
func ansiOf(rendered string) string {
	if i := strings.Index(rendered, "m"); i > 0 {
		return rendered[:i+1]
	}
	return rendered
}

func manyFlowView(n int) core.View {
	names := []string{"", "ios", "android", "desktop", "edge-workers", "documentation-site"}
	commits := make([]core.CommitView, 0, n)
	for i := 0; i < n && i < len(names); i++ {
		commits = append(commits, core.CommitView{
			SHA: string(rune('a' + i)), Author: "a", Subject: "s", Time: time.Unix(int64(100+i), 0),
			Events: []clarityrefs.Event{depEv(names[i], int64(200+i))},
		})
	}
	return core.DeriveView(core.Snapshot{RepoName: "clarity", Commits: commits}, core.DefaultLeadTimeMode, nil)
}

// The header must fit the terminal. It is a fixed-height region — headerHeight
// sizes the viewport to match — so a header that wraps pushes the whole frame
// past the bottom of the screen.
func TestModel_HeaderFitsTheTerminalWidth(t *testing.T) {
	for _, flows := range []int{2, 3, 4, 6} {
		for _, width := range []int{40, 60, 80, 120} {
			m := tui.New().WithSize(width, 20).WithBackground(lipgloss.Color("#0d1117"))
			next, _ := m.Update(tui.ViewMsg(manyFlowView(flows)))
			header := headerOf(next.(tui.Model))

			if got := lipgloss.Width(header); got > width {
				t.Errorf("%d flows at width %d: header is %d columns and wraps out of the frame:\n%q",
					flows, width, got, header)
			}
		}
	}
}

// Narrowing truncates names but must never drop a flow: the stuck deploy is
// the one most worth seeing and would be the one to vanish.
func TestModel_NarrowStripKeepsEveryFlow(t *testing.T) {
	m := tui.New().WithSize(44, 20).WithBackground(lipgloss.Color("#0d1117"))
	next, _ := m.Update(tui.ViewMsg(manyFlowView(4)))
	header := headerOf(next.(tui.Model))

	// Every flow still contributes a status glyph, even where its name had to
	// be cut down to a stub.
	if got := strings.Count(header, "✓"); got < 4 {
		t.Errorf("only %d of 4 flows survived the narrow strip:\n%q", got, header)
	}
}

// Light terminals need the chrome darkened, not lightened — lightening a pale
// background produces a bar that is invisible against it.
func TestModel_LightTerminalDarkensTheChrome(t *testing.T) {
	light := lipgloss.Color("#fdf6e3")
	m := tui.New().WithSize(100, 40).WithBackground(light)
	next, _ := m.Update(tui.ViewMsg(twoFlowView()))
	styled := strings.SplitN(next.(tui.Model).View().Content, "\n", 2)[0]

	lightened := ansiOf(lipgloss.NewStyle().Background(lipgloss.Lighten(light, 0.10)).Render(" "))
	darkened := ansiOf(lipgloss.NewStyle().Background(lipgloss.Darken(light, 0.10*0.6)).Render(" "))

	if strings.Contains(styled, lightened) {
		t.Errorf("chrome was lightened on a light terminal:\n%q", styled)
	}
	if !strings.Contains(styled, darkened) {
		t.Errorf("chrome was not darkened on a light terminal:\n%q", styled)
	}
}

// shift+tab has to cycle backwards. The helper must also produce what a real
// terminal produces — CSI Z decodes to KeyTab carrying ModShift — or this
// passes while the binding does nothing.
func TestModel_ShiftTabCyclesBackwards(t *testing.T) {
	m, _ := newModel("clarity").Update(tui.ViewMsg(manyFlowView(3)))
	model := m.(tui.Model)

	if got := press(model, "shift+tab").SelectedFlow(); got != "ios" {
		t.Errorf("shift+tab from the first flow selected %q, want the last (ios)", got)
	}
	if got := press(press(model, "2"), "shift+tab").SelectedFlow(); got != "deploy" {
		t.Errorf("shift+tab from the second flow selected %q, want deploy", got)
	}
}

// --deploy opens the TUI on a chosen flow, which is how a monorepo gets the
// polyrepo feel: one terminal pane per subsystem, each pinned to its own flow.
func TestModel_DeployFlagPreselectsAFlow(t *testing.T) {
	m := tui.New().WithSize(100, 40).WithFlow("ios")
	next, _ := m.Update(tui.ViewMsg(twoFlowView()))

	if got := next.(tui.Model).SelectedFlow(); got != "ios" {
		t.Errorf("--deploy=ios opened on %q", got)
	}
}

// Once the user moves, later Views must not yank them back to where --deploy
// started them — the flag chooses the opening flow, not a permanent pin.
func TestModel_DeployFlagDoesNotOverrideLaterSelection(t *testing.T) {
	m := tui.New().WithSize(100, 40).WithFlow("ios")
	first, _ := m.Update(tui.ViewMsg(twoFlowView()))
	moved := press(first.(tui.Model), "1")

	if got := moved.SelectedFlow(); got != "deploy" {
		t.Fatalf("pressing 1 selected %q", got)
	}

	next, _ := moved.Update(tui.ViewMsg(twoFlowView()))
	if got := next.(tui.Model).SelectedFlow(); got != "deploy" {
		t.Errorf("a later view reset the selection to %q — --deploy chose the opening flow, not a pin", got)
	}
}

// An unmatched --deploy is an error, not a fallback. Opening on some other
// flow looks exactly like a deliberate selection, so a user who asked for ios
// and got web has no way to tell.
func TestModel_UnknownDeployFlagIsAnError(t *testing.T) {
	m := tui.New().WithSize(100, 40).WithFlow("nope")
	next, cmd := m.Update(tui.ViewMsg(twoFlowView()))
	model := next.(tui.Model)

	if model.FlowErr() == nil {
		t.Fatal("an unmatched --deploy did not produce an error")
	}
	for _, want := range []string{"nope", "deploy", "ios"} {
		if !strings.Contains(model.FlowErr().Error(), want) {
			t.Errorf("error does not mention %q: %v", want, model.FlowErr())
		}
	}
	if cmd == nil {
		t.Error("the program was not asked to quit")
	}
}

// A stale view cannot answer --deploy. The cached lens paints one first, from
// a snapshot that may predate the flow being asked for, so rejecting the flag
// there would fail a command the fresh view is about to satisfy.
func TestModel_StaleViewDoesNotConsumeTheDeployFlag(t *testing.T) {
	stale := twoFlowView()
	stale.Flows = stale.Flows[:1] // cache predates the ios flow
	stale.Stale = true

	m := tui.New().WithSize(100, 40).WithFlow("ios")
	afterStale, _ := m.Update(tui.ViewMsg(stale))
	if err := afterStale.(tui.Model).FlowErr(); err != nil {
		t.Fatalf("a stale view rejected the flag: %v", err)
	}

	fresh, _ := afterStale.(tui.Model).Update(tui.ViewMsg(twoFlowView()))
	model := fresh.(tui.Model)
	if err := model.FlowErr(); err != nil {
		t.Fatalf("the fresh view rejected the flag: %v", err)
	}
	if got := model.SelectedFlow(); got != "ios" {
		t.Errorf("--deploy=ios was consumed by the stale view; opened on %q", got)
	}
}
