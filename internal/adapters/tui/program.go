package tui

import (
	"context"
	"image/color"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ezcdlabs/clarity/internal/core"
)

// ViewMsg is sent to the Bubble Tea program for each View the renderer
// pulls off its Views channel. Carries the derived Groups / Weekly /
// Header (currently re-derived inside RenderSnapshot — folding them in
// is a future cleanup) plus the Stale flag that gates the "refreshing…"
// header indicator.
type ViewMsg core.View

// tickMsg fires often enough to keep the spinner animating and the lead-time
// timers updating.
type tickMsg time.Time

// headerHeight is the number of rows the fixed header occupies above the
// scrollable body: one line of badges plus one blank separator line.
const headerHeight = 2

// chromeElevation is how far the header bar is lifted off the terminal
// background. Below about 8% it stops reading as a distinct surface; much
// above it and the bar starts looking painted on rather than made of the
// background. Light terminals need less, so they darken by a fraction of it.
const chromeElevation = 0.10

// lightElevationRatio is how much of that elevation a light terminal uses.
// Darkening a pale background reads as a much stronger step than lightening a
// dark one does, so the same figure would look heavy-handed.
const lightElevationRatio = 0.6

// cellFixed is what one strip cell costs before its name: a leading space, the
// space before its status icon, the icon, and a trailing space.
const cellFixed = 4

// Model is the Bubble Tea state — the latest View (carrying the joined
// snapshot, its derived shapes, and the Stale flag for the SWR
// indicator), terminal dimensions, a flag for whether we've received
// any view yet (so the first paint can show a spinner-and-"Loading"
// state instead of the genuine "no commits" state), a clock function
// for timer updates, and a scrollable viewport holding the body.
type Model struct {
	view       core.View
	width      int
	height     int
	received   bool
	nowFn      func() time.Time
	spinnerIdx int
	viewport   viewport.Model
	// selectedFlow is the name of the deploy flow on screen, not its index.
	// A newly discovered target or a config change reorders the strip, and a
	// user who chose "ios" must still be looking at ios afterwards. Empty
	// means "the first flow", which is what a fresh Model shows.
	selectedFlow string
	// background is the terminal's own background colour, queried once over
	// OSC 11. The deploy strip derives its chrome by lightening it, so the bar
	// picks up whatever tint the user's theme has instead of pasting a grey
	// over it. nil when the terminal didn't answer — piped output, no TTY, a
	// terminal that ignores the query — and the strip falls back accordingly.
	background    color.Color
	backgroundSet bool
}

// WithBackground returns a copy of m with the terminal background fixed
// explicitly, bypassing the OSC 11 query. Used by tests to exercise both the
// chrome and the fallback rendering without a TTY.
func (m Model) WithBackground(bg color.Color) Model {
	m.background, m.backgroundSet = bg, true
	m.viewport.SetContent(m.renderBody())
	return m
}

// chrome returns the elevated surface the header bar is painted in, or nil
// when there is no background to derive one from.
func (m Model) chrome() color.Color {
	bg := m.terminalBackground()
	if bg == nil {
		return nil
	}
	// Derive the direction from the background in hand rather than the global
	// verdict: they agree in production, but only this one honours an
	// explicitly supplied background. Lightening a pale terminal produces a
	// bar invisible against it.
	if isDarkColor(bg) {
		return lipgloss.Lighten(bg, chromeElevation)
	}
	return lipgloss.Darken(bg, chromeElevation*lightElevationRatio)
}

func (m Model) terminalBackground() color.Color {
	if m.backgroundSet {
		return m.background
	}
	return detectBackgroundColor()
}

// KeyMsg builds the key press message the Model handles, so tests can drive
// flow selection without a TTY.
func KeyMsg(key string) tea.Msg {
	return tea.KeyPressMsg(tea.Key{Code: keyCode(key), Mod: keyMod(key), Text: keyText(key)})
}

// KeyMsg must produce what a real terminal produces, or a test can pass while
// the binding does nothing. shift+tab in particular arrives as CSI Z, decoded
// as KeyTab carrying ModShift — without the modifier it is indistinguishable
// from a plain tab and the backward branch is never exercised.
func keyCode(key string) rune {
	switch key {
	case "tab", "shift+tab":
		return tea.KeyTab
	}
	runes := []rune(key)
	if len(runes) == 0 {
		return 0
	}
	return runes[0]
}

func keyMod(key string) tea.KeyMod {
	if key == "shift+tab" {
		return tea.ModShift
	}
	return 0
}

func keyText(key string) string {
	if runes := []rune(key); len(runes) == 1 {
		return key
	}
	return ""
}

// SelectedFlow returns the name of the flow currently on screen.
func (m Model) SelectedFlow() string {
	return m.currentFlow().Name
}

// currentFlow resolves the selection to a FlowView. Selection is by name, so
// a flow that disappears (a target that aged out of the window, or a config
// edit) falls back to the first rather than leaving the body blank.
func (m Model) currentFlow() core.FlowView {
	for _, f := range m.view.Flows {
		if f.Name == m.selectedFlow {
			return f
		}
	}
	if len(m.view.Flows) > 0 {
		return m.view.Flows[0]
	}
	return core.FlowView{}
}

func (m Model) flowIndex() int {
	for i, f := range m.view.Flows {
		if f.Name == m.selectedFlow {
			return i
		}
	}
	return 0
}

// selectFlow moves the selection to the given index, ignoring anything out of
// range — a stray key press must never blank the body.
func (m Model) selectFlow(i int) Model {
	if i < 0 || i >= len(m.view.Flows) {
		return m
	}
	m.selectedFlow = m.view.Flows[i].Name
	m.viewport.SetContent(m.renderBody())
	return m
}

// New constructs a Model with the real clock. RepoName is no longer a
// constructor argument; it travels on each Snapshot the Source emits.
func New() Model {
	return Model{
		nowFn:    time.Now,
		viewport: viewport.New(),
	}
}

// WithClock returns a copy of m with the clock function replaced. Used by
// tests and the demo binary to feed deterministic time into the renderer.
func (m Model) WithClock(nowFn func() time.Time) Model {
	m.nowFn = nowFn
	return m
}

// WithSize returns a copy of m with the terminal dimensions set explicitly
// (and the viewport sized accordingly). Used by tests to render at a known
// width without relying on a tea.WindowSizeMsg from a real terminal.
func (m Model) WithSize(width, height int) Model {
	m.width = width
	m.height = height
	m.viewport.SetWidth(width)
	m.viewport.SetHeight(max(0, height-headerHeight))
	m.viewport.SetContent(m.renderBody())
	return m
}

func (m Model) Init() tea.Cmd { return tickEvery() }

// tickEvery fires fast enough for smooth spinner animation. Lead-time
// timers display whole seconds so they only visually update once per
// second regardless.
func tickEvery() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		key := msg.String()
		switch key {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			if n := len(m.view.Flows); n > 1 {
				return m.selectFlow((m.flowIndex() + 1) % n), nil
			}
			return m, nil
		case "shift+tab":
			if n := len(m.view.Flows); n > 1 {
				return m.selectFlow((m.flowIndex() - 1 + n) % n), nil
			}
			return m, nil
		}
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			return m.selectFlow(int(key[0] - '1')), nil
		}
		// fall through to viewport for scroll keys (up/down/pgup/pgdn/k/j/g/G)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(max(0, msg.Height-headerHeight))
		m.viewport.SetContent(m.renderBody())
		return m, nil
	case ViewMsg:
		m.view = core.View(msg)
		m.received = true
		m.viewport.SetContent(m.renderBody())
		return m, nil
	case tickMsg:
		m.spinnerIdx++
		m.viewport.SetContent(m.renderBody())
		return m, tickEvery()
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

// renderBody returns the body content the viewport scrolls over. The header
// stays fixed above and is rendered separately by View().
func (m Model) renderBody() string {
	if !m.received {
		spin := lipgloss.NewStyle().Foreground(colorBlue).Render(spinnerFrame(m.spinnerIdx))
		return "  " + spin + " Loading\n"
	}
	now := time.Now()
	if m.nowFn != nil {
		now = m.nowFn()
	}
	return RenderSnapshot(m.view, m.currentFlow(), m.width, now, m.spinnerIdx)
}

func (m Model) View() tea.View {
	var b strings.Builder
	b.WriteString(renderHeader(m.view, m.selectedFlow, m.width, m.chrome(), m.terminalBackground()))
	b.WriteString("\n\n")
	b.WriteString(m.viewport.View())
	v := tea.NewView(b.String())
	// AltScreen replaces the v1 `tea.WithAltScreen` program option — v2 made
	// terminal features declarative (set on the View each frame) rather than
	// imperative (set once at NewProgram).
	v.AltScreen = true
	return v
}

// renderHeader builds the top line: repo name (bold) + build/deploy status
// badges on the left, an optional "refreshing…" SWR hint, "press q to
// quit" right-aligned to width. When any badge has resolved to failed,
// the repo name flips bold red — a focused alarm in the top-left where
// the eye naturally lands first, without recolouring the rest of the
// header.
func renderHeader(view core.View, selected string, width int, chrome, base color.Color) string {
	snap := view.Snapshot
	ciStatus := view.Header.CI
	deployStatus := view.Header.Deploy

	// With a strip on it the header row becomes chrome, painted the full width
	// so it reads as one surface with the selected flow cut out of it. The
	// background has to be set on every constituent style rather than wrapped
	// around the joined string: lipgloss does not re-apply an outer style after
	// an inner SGR reset, so wrapping paints only as far as the first styled
	// run and leaves a hole through the middle of the bar.
	barred := chrome != nil && len(view.Flows) > 1
	bg := color.Color(nil)
	if barred {
		bg = chrome
	}
	on := func(st lipgloss.Style) lipgloss.Style {
		if barred {
			return st.Background(chrome)
		}
		return st
	}

	titleStyle := on(lipgloss.NewStyle().Bold(true))
	if ciStatus == "failed" || deployStatus == "failed" {
		titleStyle = titleStyle.Foreground(colorRed)
	}
	title := titleStyle.Render(snap.RepoName)

	dot := on(lipgloss.NewStyle().Foreground(colorGray)).Render("·")
	ci := on(lipgloss.NewStyle().Foreground(colorGray)).Render("ci:") + " " + statusIcon(ciStatus, bg)

	parts := []string{title, dot, ci, dot}
	if view.Stale {
		parts = append(parts,
			on(lipgloss.NewStyle().Foreground(colorGray).Italic(true)).Render("refreshing…"), dot)
	}
	left := strings.Join(parts, "  ")
	right := on(lipgloss.NewStyle().Foreground(colorGray)).Render("press q to quit")

	deploy := on(lipgloss.NewStyle().Foreground(colorGray)).Render("deploy:") + " " + statusIcon(deployStatus, bg)
	if len(view.Flows) > 1 {
		// The strip takes whatever the fixed parts leave. If that isn't enough
		// the quit hint goes first — it is the only thing on the row carrying
		// no information — and only then do names start truncating. A flow is
		// never dropped: the stuck deploy is the one most worth seeing and
		// would be the one to vanish.
		// What the strip may occupy: the row minus the fixed left parts, the
		// two-column gap after them, and the quit hint with its own gap. When
		// that isn't enough the hint goes first — it is the only thing on the
		// row carrying no information.
		budget := width - lipgloss.Width(left) - 2 - lipgloss.Width(right) - 2
		if width <= 0 {
			budget = stripWidth(view.Flows)
		} else if stripWidth(view.Flows) > budget {
			right = ""
			budget = width - lipgloss.Width(left) - 2
		}
		deploy = renderStrip(view.Flows, selected, chrome, base, budget)
	}

	line := left + "  " + deploy
	if width <= 0 {
		if right == "" {
			return line
		}
		return line + "  " + right
	}

	pad := width - lipgloss.Width(line) - lipgloss.Width(right)
	if pad < 0 {
		pad = 0
	}
	fill := lipgloss.NewStyle()
	if barred {
		fill = fill.Background(chrome)
	}
	return line + fill.Render(strings.Repeat(" ", pad)) + right
}

// stripWidth is how many columns the strip wants before any truncation: the
// group label, each cell's name plus its three columns of padding and status
// icon, and one separator between neighbours.
func stripWidth(flows []core.FlowView) int {
	w := lipgloss.Width("deploy:") + cellFixed*len(flows) + len(flows) - 1
	for _, f := range flows {
		w += lipgloss.Width(f.Name)
	}
	return w
}

// renderStrip renders the deploy flows as selectable cells after a "deploy:"
// group label. The selected cell is the only one rendered in full weight; the
// rest stay dim, so the strip reads as one control with one active member.
func renderStrip(flows []core.FlowView, selected string, chrome, base color.Color, budget int) string {
	active := 0
	for i, f := range flows {
		if f.Name == selected {
			active = i
		}
	}

	labelStyle := lipgloss.NewStyle().Foreground(colorGray)
	if chrome != nil {
		labelStyle = labelStyle.Background(chrome)
	}

	perName, withLabel, withSep := stripLayout(flows, budget)

	label := ""
	if withLabel {
		label = labelStyle.Render("deploy:")
	}

	cells := make([]string, 0, len(flows))
	for i, f := range flows {
		cells = append(cells, renderFlowCell(f, i == active, chrome, base, perName))
	}

	sep := ""
	if withSep {
		sepStyle := lipgloss.NewStyle().Foreground(colorGray)
		if chrome != nil {
			sepStyle = sepStyle.Background(chrome)
		}
		sep = sepStyle.Render(" ")
	}
	return label + strings.Join(cells, sep)
}

// stripLayout picks the widest arrangement that fits the budget, degrading in
// the order things stop earning their columns: names shorten, then names go
// entirely, then the group label, and finally the separators. A flow is never
// dropped — the stuck deploy is the one most worth seeing and would be exactly
// the one to vanish — so at the extreme the strip becomes a row of bare status
// glyphs, which is still the higher-priority half of what a tab carries.
func stripLayout(flows []core.FlowView, budget int) (perName int, withLabel, withSep bool) {
	n := len(flows)
	longest := 0
	for _, f := range flows {
		if w := lipgloss.Width(f.Name); w > longest {
			longest = w
		}
	}

	width := func(name int, label, sep bool) int {
		w := n * (cellFixed - 1) // pads and status icon
		if name > 0 {
			w += n * (name + 1) // the name and the space before the icon
		}
		if label {
			w += lipgloss.Width("deploy:")
		}
		if sep {
			w += n - 1
		}
		return w
	}

	for _, opt := range []struct{ label, sep bool }{{true, true}, {false, true}, {false, false}} {
		for name := longest; name >= 0; name-- {
			if width(name, opt.label, opt.sep) <= budget {
				return name, opt.label, opt.sep
			}
		}
	}
	// Nothing fits; render the most compact form and let it overflow rather
	// than hide a flow.
	return 0, false, false
}

// truncateName shortens a flow name to fit, keeping it identifiable. Never
// shorter than a single character, because a nameless tab is no more useful
// than a hidden one.
func truncateName(name string, max int) string {
	if lipgloss.Width(name) <= max {
		return name
	}
	runes := []rune(name)
	switch {
	case max <= 0:
		// Nothing left for a name. The status glyph still renders, which is
		// the higher-priority half of what a tab carries.
		return ""
	case max == 1:
		return string(runes[:1])
	default:
		return string(runes[:max-1]) + "…"
	}
}

// renderFlowCell renders one flow. With chrome available the selected cell is
// painted in the *body* background, so it is the only thing on the row sharing
// a colour with the content below — a tab connected to what it controls.
// Without it, weight and an underline carry the selection instead; a colour
// picked from the text palette would only ever look pasted on.
func renderFlowCell(f core.FlowView, selected bool, chrome, base color.Color, nameWidth int) string {
	nameStyle := lipgloss.NewStyle().Foreground(colorGray)
	cellBg := chrome

	switch {
	case selected && chrome != nil:
		cellBg = base
		nameStyle = lipgloss.NewStyle().Bold(true)
	case selected:
		nameStyle = lipgloss.NewStyle().Bold(true).Underline(true)
	}
	if cellBg != nil {
		nameStyle = nameStyle.Background(cellBg)
	}

	pad := lipgloss.NewStyle()
	if cellBg != nil {
		pad = pad.Background(cellBg)
	}
	name := truncateName(f.Name, nameWidth)
	if name == "" {
		// cellFixed still holds: one pad short of a named cell, because the
		// space that separated name from icon goes with the name.
		return pad.Render(" ") + statusIcon(f.Deploy, cellBg) + pad.Render(" ")
	}
	return pad.Render(" ") + nameStyle.Render(name) + pad.Render(" ") +
		statusIcon(f.Deploy, cellBg) + pad.Render(" ")
}

// statusIcon renders a flow's deploy badge on the given background.
func statusIcon(status string, bg color.Color) string {
	st := lipgloss.NewStyle()
	if bg != nil {
		st = st.Background(bg)
	}
	switch status {
	case "passed":
		return st.Foreground(colorGreen).Render("✓")
	case "failed":
		return st.Foreground(colorRed).Render("✗")
	}
	return st.Foreground(colorGray).Render("·")
}

// badge renders "<label>: <icon>" for the header status indicators.
func badge(label, status string) string {
	return lipgloss.NewStyle().Foreground(colorGray).Render(label+":") + " " + iconForStatus(status)
}

// iconForStatus colours the binary header badge: green ✓ when the pipeline
// is known-good, red ✗ when known-broken, gray · when there's no resolved
// data yet. The header is a summary — it gets the coloured tick that the
// per-row icons deliberately don't, so it can answer "is the pipeline
// green?" at a glance. Transient/started events are not reflected here;
// see currentStageStatus.
func iconForStatus(status string) string {
	switch status {
	case "passed":
		return lipgloss.NewStyle().Foreground(colorGreen).Render("✓")
	case "failed":
		return lipgloss.NewStyle().Foreground(colorRed).Render("✗")
	default:
		return lipgloss.NewStyle().Foreground(colorGray).Render("·")
	}
}

// Compile-time check: the Renderer adapter satisfies the core.Renderer
// port. Drift on the port signature surfaces here at build time.
var _ core.Renderer = (*Renderer)(nil)

// Renderer is the core.Renderer adapter for the bubble-tea TUI. Constructed
// without any arguments — the repository name travels on the View's
// Snapshot, and there are no other inputs the constructor needs to know
// about. Tests / the demo binary that want a custom clock use WithClock.
type Renderer struct {
	nowFn func() time.Time
}

// NewRenderer returns a TUI Renderer with the wall clock. Inject a
// deterministic clock via WithClock for the demo binary.
func NewRenderer() *Renderer { return &Renderer{} }

// WithClock returns a copy of r with the clock function set. Same idiom
// as Model.WithClock — used by the demo to pin lead-time timers to a
// scenario's reference time.
func (r *Renderer) WithClock(nowFn func() time.Time) *Renderer {
	cp := *r
	cp.nowFn = nowFn
	return &cp
}

// Render runs the bubble-tea program, sending each incoming View into
// the Model as a ViewMsg. Blocks until the user quits (q / Ctrl+C).
// When ctx is cancelled the program is asked to quit so callers can
// interrupt cleanly on signal.
func (r *Renderer) Render(ctx context.Context, views <-chan core.View) error {
	p := newProgram(views, r.nowFn)
	go func() {
		<-ctx.Done()
		p.Quit()
	}()
	_, err := p.Run()
	return err
}

// NewProgram constructs a Bubble Tea program in alt-screen mode and starts a
// goroutine that forwards each View into it as a ViewMsg. The returned
// *tea.Program is ready for callers to invoke .Run() on. Exposed (rather
// than hidden inside Renderer.Render) so the demo binary can also Send
// synthetic messages — e.g. a scripted quit at the end of a recorded
// scenario.
func NewProgram(views <-chan core.View) *tea.Program {
	return newProgram(views, nil)
}

// NewProgramWithClock is like NewProgram but lets the caller drive the
// timer's notion of "now". Used by the demo binary so lead-time timers tick
// relative to a scenario's reference time rather than wall time.
func NewProgramWithClock(views <-chan core.View, nowFn func() time.Time) *tea.Program {
	return newProgram(views, nowFn)
}

func newProgram(views <-chan core.View, nowFn func() time.Time) *tea.Program {
	m := New()
	if nowFn != nil {
		m = m.WithClock(nowFn)
	}
	// Force the lazy background-color detection to fire BEFORE bubbletea
	// claims stdin. Without this, the first divider render triggers the
	// query while bubbletea is also reading stdin, the response gets lost,
	// and lazyAdaptive falls back to its dark default — light-terminal
	// users get bright yellow instead of the dim yellow v1 picked for them.
	// Both the light/dark verdict and the background colour itself come from
	// this one query, and it must complete before bubbletea claims stdin —
	// otherwise the OSC round-trip happens inside the event loop, blocking the
	// first frame and racing bubbletea's input reader for the same fd.
	detectTerminal()
	// AltScreen is now declared on the View itself (see Model.View), so we
	// don't pass it as a program option anymore.
	p := tea.NewProgram(m)
	go func() {
		for v := range views {
			p.Send(ViewMsg(v))
		}
	}()
	return p
}
