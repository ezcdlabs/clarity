package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

// SnapshotMsg is sent to the Bubble Tea program when the watcher emits a new
// snapshot.
type SnapshotMsg watcher.Snapshot

// Model is the Bubble Tea state — the latest snapshot, terminal dimensions,
// the branch name (for the header), and a flag for whether we've received
// any snapshot yet (so the first paint can show "Loading…" instead of the
// genuine "no commits" state).
type Model struct {
	snap     watcher.Snapshot
	width    int
	height   int
	branch   string
	received bool
}

// New constructs a Model for the given branch.
func New(branch string) Model {
	return Model{branch: branch}
}

func (m Model) Init() tea.Cmd { return nil }

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
		b.WriteString(RenderSnapshot(m.snap, m.width))
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
	p := tea.NewProgram(New(branch), tea.WithAltScreen())
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
