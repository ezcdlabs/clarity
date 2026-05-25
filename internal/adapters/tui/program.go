package tui

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ezcdlabs/clarity/internal/core"
)

// SnapshotMsg is sent to the Bubble Tea program for each Snapshot the
// renderer pulls off its Views channel. The Model only needs the Snapshot
// part of the View (RepoName, Commits) — DeriveView's other outputs are
// recomputed inside RenderSnapshot for now, since the existing pure
// renderer takes a Snapshot, not a View. A later cleanup can fold the View
// in directly so derived Groups / Weekly aren't recomputed.
type SnapshotMsg core.Snapshot

// tickMsg fires often enough to keep the spinner animating and the lead-time
// timers updating.
type tickMsg time.Time

// headerHeight is the number of rows the fixed header occupies above the
// scrollable body: one line of badges plus one blank separator line.
const headerHeight = 2

// Model is the Bubble Tea state — the latest snapshot (carrying RepoName),
// terminal dimensions, a flag for whether we've received any snapshot yet
// (so the first paint can show a spinner-and-"Loading" state instead of
// the genuine "no commits" state), a clock function for timer updates,
// and a scrollable viewport holding the body.
type Model struct {
	snap       core.Snapshot
	width      int
	height     int
	received   bool
	nowFn      func() time.Time
	spinnerIdx int
	viewport   viewport.Model
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
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		// fall through to viewport for scroll keys (up/down/pgup/pgdn/k/j/g/G)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(max(0, msg.Height-headerHeight))
		m.viewport.SetContent(m.renderBody())
		return m, nil
	case SnapshotMsg:
		m.snap = core.Snapshot(msg)
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
	return RenderSnapshot(m.snap, m.width, now, m.spinnerIdx)
}

func (m Model) View() tea.View {
	var b strings.Builder
	b.WriteString(renderHeader(m.snap, m.width))
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
// badges on the left, "press q to quit" right-aligned to width. When any
// badge has resolved to failed, the repo name flips bold red — a focused
// alarm in the top-left where the eye naturally lands first, without
// recolouring the rest of the header.
func renderHeader(snap core.Snapshot, width int) string {
	ciStatus := core.CurrentStageStatus(snap.Commits, "ci")
	deployStatus := core.CurrentStageStatus(snap.Commits, "deploy")

	titleStyle := lipgloss.NewStyle().Bold(true)
	if ciStatus == "failed" || deployStatus == "failed" {
		titleStyle = titleStyle.Foreground(colorRed)
	}
	title := titleStyle.Render(snap.RepoName)

	ci := badge("ci", ciStatus)
	deploy := badge("deploy", deployStatus)
	dot := lipgloss.NewStyle().Foreground(colorGray).Render("·")

	left := strings.Join([]string{title, dot, ci, dot, deploy}, "  ")
	right := lipgloss.NewStyle().Foreground(colorGray).Render("press q to quit")

	if width <= 0 {
		return left + "  " + right
	}
	pad := width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 2 {
		pad = 2
	}
	return left + strings.Repeat(" ", pad) + right
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

// Render runs the bubble-tea program, sending each incoming View's
// Snapshot into the Model as a SnapshotMsg. Blocks until the user quits
// (q / Ctrl+C). When ctx is cancelled the program is asked to quit so
// callers can interrupt cleanly on signal.
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
// goroutine that forwards each View's Snapshot into it. The returned
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
	detectDarkBackground()
	// AltScreen is now declared on the View itself (see Model.View), so we
	// don't pass it as a program option anymore.
	p := tea.NewProgram(m)
	go func() {
		for v := range views {
			p.Send(SnapshotMsg(v.Snapshot))
		}
	}()
	return p
}
