package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

// SnapshotMsg is sent to the Bubble Tea program when the watcher emits a new
// snapshot.
type SnapshotMsg watcher.Snapshot

// Model is the Bubble Tea state — just the latest snapshot and the terminal
// dimensions. All rendering logic is in render.go.
type Model struct {
	snap   watcher.Snapshot
	width  int
	height int
}

func New() Model { return Model{} }

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
	}
	return m, nil
}

func (m Model) View() string {
	header := lipgloss.NewStyle().Bold(true).Render("clarity") + "\n\n"
	return header + RenderSnapshot(m.snap, m.width)
}

// Run starts the Bubble Tea program in alt-screen mode and forwards every
// snapshot from the channel into it. Returns when the user quits or the
// channel is closed.
func Run(snapshots <-chan watcher.Snapshot) error {
	p := tea.NewProgram(New(), tea.WithAltScreen())

	go func() {
		for snap := range snapshots {
			p.Send(SnapshotMsg(snap))
		}
	}()

	_, err := p.Run()
	return err
}
