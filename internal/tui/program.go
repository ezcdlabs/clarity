package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

// SnapshotMsg is sent to the Bubble Tea program when the watcher emits a new
// snapshot.
type SnapshotMsg watcher.Snapshot

// tickMsg fires often enough to keep the spinner animating and the lead-time
// timers updating.
type tickMsg time.Time

// headerHeight is the number of rows the fixed header occupies above the
// scrollable body: one line of badges plus one blank separator line.
const headerHeight = 2

// Model is the Bubble Tea state — the latest snapshot, terminal dimensions,
// the repository name (for the header), a flag for whether we've received
// any snapshot yet (so the first paint can show a spinner-and-"Loading"
// state instead of the genuine "no commits" state), a clock function for
// timer updates, and a scrollable viewport holding the body.
type Model struct {
	snap       watcher.Snapshot
	width      int
	height     int
	repoName   string
	received   bool
	nowFn      func() time.Time
	spinnerIdx int
	viewport   viewport.Model
}

// New constructs a Model for the given repository name using the real clock.
func New(repoName string) Model {
	return Model{
		repoName: repoName,
		nowFn:    time.Now,
		viewport: viewport.New(0, 0),
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
	m.viewport.Width = width
	m.viewport.Height = max(0, height-headerHeight)
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
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		// fall through to viewport for scroll keys (up/down/pgup/pgdn/k/j/g/G)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = max(0, msg.Height-headerHeight)
		m.viewport.SetContent(m.renderBody())
		return m, nil
	case SnapshotMsg:
		m.snap = watcher.Snapshot(msg)
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

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(renderHeader(m.repoName, m.snap, m.width))
	b.WriteString("\n\n")
	b.WriteString(m.viewport.View())
	return b.String()
}

// renderHeader builds the top line: repo name (bold) + build/deploy status
// badges on the left, "press q to quit" right-aligned to width. When any
// badge has resolved to failed, the repo name flips bold red — a focused
// alarm in the top-left where the eye naturally lands first, without
// recolouring the rest of the header.
func renderHeader(repoName string, snap watcher.Snapshot, width int) string {
	ciStatus := currentStageStatus(snap.Commits, "ci")
	deployStatus := currentStageStatus(snap.Commits, "deploy")

	titleStyle := lipgloss.NewStyle().Bold(true)
	if ciStatus == "failed" || deployStatus == "failed" {
		titleStyle = titleStyle.Foreground(colorRed)
	}
	title := titleStyle.Render(repoName)

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

// currentStageStatus returns the latest *resolved* (passed or failed) event
// status for the given stage anywhere in the snapshot, or "" if no resolved
// events exist. "started" and "skipped" events are intentionally ignored so
// the header badge holds its colour through transient retries instead of
// flickering to neutral every time a build kicks off.
func currentStageStatus(commits []watcher.CommitView, stage string) string {
	var latest clarityrefs.Event
	found := false
	for _, c := range commits {
		for _, e := range c.Events {
			if e.Stage != stage {
				continue
			}
			if e.Status != "passed" && e.Status != "failed" {
				continue
			}
			if !found || e.Time.After(latest.Time) {
				latest = e
				found = true
			}
		}
	}
	if !found {
		return ""
	}
	return latest.Status
}

// NewProgram constructs a Bubble Tea program in alt-screen mode and starts a
// goroutine that forwards every snapshot from the channel into it. The
// returned *tea.Program is ready for callers to invoke .Run() on. Exposed
// (rather than hidden inside Run) so the demo binary can also Send synthetic
// messages — e.g. a scripted quit at the end of a recorded scenario.
func NewProgram(repoName string, snapshots <-chan watcher.Snapshot) *tea.Program {
	return newProgram(repoName, snapshots, nil)
}

// NewProgramWithClock is like NewProgram but lets the caller drive the
// timer's notion of "now". Used by the demo binary so lead-time timers tick
// relative to a scenario's reference time rather than wall time.
func NewProgramWithClock(repoName string, snapshots <-chan watcher.Snapshot, nowFn func() time.Time) *tea.Program {
	return newProgram(repoName, snapshots, nowFn)
}

func newProgram(repoName string, snapshots <-chan watcher.Snapshot, nowFn func() time.Time) *tea.Program {
	m := New(repoName)
	if nowFn != nil {
		m = m.WithClock(nowFn)
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	go func() {
		for snap := range snapshots {
			p.Send(SnapshotMsg(snap))
		}
	}()
	return p
}

// Run starts the Bubble Tea program and blocks until the user quits or the
// program exits.
func Run(repoName string, snapshots <-chan watcher.Snapshot) error {
	_, err := NewProgram(repoName, snapshots).Run()
	return err
}
