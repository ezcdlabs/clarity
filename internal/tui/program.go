package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

// SnapshotMsg is sent to the Bubble Tea program when the watcher emits a new
// snapshot.
type SnapshotMsg watcher.Snapshot

// tickMsg fires once per second to drive the live half of the lead-time timer.
type tickMsg time.Time

// Model is the Bubble Tea state — the latest snapshot, terminal dimensions,
// the branch name (for the header), a flag for whether we've received any
// snapshot yet (so the first paint can show "Loading…" instead of the genuine
// "no commits" state), and a clock function used to drive timer updates.
type Model struct {
	snap     watcher.Snapshot
	width    int
	height   int
	branch   string
	received bool
	nowFn    func() time.Time
}

// New constructs a Model for the given branch using the real clock.
func New(branch string) Model {
	return Model{branch: branch, nowFn: time.Now}
}

// WithClock returns a copy of m with the clock function replaced. Used by
// tests to feed deterministic time into the renderer.
func (m Model) WithClock(nowFn func() time.Time) Model {
	m.nowFn = nowFn
	return m
}

func (m Model) Init() tea.Cmd { return tickEvery() }

func tickEvery() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case SnapshotMsg:
		m.snap = watcher.Snapshot(msg)
		m.received = true
	case tickMsg:
		// re-render is automatic on the returned Cmd's next firing.
		return m, tickEvery()
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(renderHeader(m.branch))
	b.WriteString("\n\n")

	if !m.received {
		b.WriteString("  Loading…\n")
	} else {
		now := time.Now()
		if m.nowFn != nil {
			now = m.nowFn()
		}
		b.WriteString(RenderSnapshot(m.snap, m.width, now))
	}

	b.WriteString("\n")
	b.WriteString(renderFooter())
	return b.String()
}

func renderHeader(branch string) string {
	title := lipgloss.NewStyle().Bold(true).Render("clarity")
	br := lipgloss.NewStyle().Foreground(colorGray).Render("· " + branch)
	return title + " " + br
}

func renderFooter() string {
	return lipgloss.NewStyle().Foreground(colorGray).Render("  press q to quit")
}

// NewProgram constructs a Bubble Tea program in alt-screen mode and starts a
// goroutine that forwards every snapshot from the channel into it. The
// returned *tea.Program is ready for callers to invoke .Run() on. Exposed
// (rather than hidden inside Run) so the demo binary can also Send synthetic
// messages — e.g. a scripted quit at the end of a recorded scenario.
func NewProgram(branch string, snapshots <-chan watcher.Snapshot) *tea.Program {
	return newProgram(branch, snapshots, nil)
}

// NewProgramWithClock is like NewProgram but lets the caller drive the
// timer's notion of "now". Used by the demo binary so lead-time timers tick
// relative to a scenario's reference time rather than wall time.
func NewProgramWithClock(branch string, snapshots <-chan watcher.Snapshot, nowFn func() time.Time) *tea.Program {
	return newProgram(branch, snapshots, nowFn)
}

func newProgram(branch string, snapshots <-chan watcher.Snapshot, nowFn func() time.Time) *tea.Program {
	m := New(branch)
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
func Run(branch string, snapshots <-chan watcher.Snapshot) error {
	_, err := NewProgram(branch, snapshots).Run()
	return err
}
