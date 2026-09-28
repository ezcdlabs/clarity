package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ezcdlabs/clarity/internal/core"
)

// WeeklyModel drives `git clarity metrics`.
//
// Interactive but not live — two different things that are easy to conflate.
// Live means a watcher polling the remote; interactive means an event loop
// responding to keys. This is the second without the first: one snapshot is
// read at startup and never refreshed, so there is no tick, no re-fetch, and
// no staleness to signal.
//
// It is still a Bubble Tea program because the deploy strip is a control.
// Rendering a tab bar while requiring a flag to change flows would be showing
// something that does not work. Switching costs nothing: every flow is
// already derived on the View, so selection is local UI state with no round
// trip to the Lens.
type WeeklyModel struct {
	view     core.View
	selected string
	width    int
	height   int
	offset   int // first week on screen, for scrolling a long history
}

// NewWeeklyModel returns a model over an already-derived view.
func NewWeeklyModel(view core.View, selected string, width, height int) WeeklyModel {
	m := WeeklyModel{view: view, width: width, height: height}
	if len(view.Flows) > 0 {
		m.selected = view.Flows[0].Name
	}
	if selected != "" {
		if i, ok := core.MatchFlow(view.Flows, selected); ok {
			m.selected = view.Flows[i].Name
		}
	}
	return m
}

// SelectedFlow reports which flow is on screen.
func (m WeeklyModel) SelectedFlow() string { return m.selected }

func (m WeeklyModel) Init() tea.Cmd { return nil }

func (m WeeklyModel) flowIndex() int {
	for i, f := range m.view.Flows {
		if f.Name == m.selected {
			return i
		}
	}
	return 0
}

func (m WeeklyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch key := msg.String(); key {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "tab", "right", "l":
			if n := len(m.view.Flows); n > 1 {
				m.selected = m.view.Flows[(m.flowIndex()+1)%n].Name
			}
			return m, nil
		case "shift+tab", "left", "h":
			if n := len(m.view.Flows); n > 1 {
				m.selected = m.view.Flows[(m.flowIndex()-1+n)%n].Name
			}
			return m, nil
		case "down", "j":
			// Clamped, so holding the key cannot walk off the end. Unbounded,
			// it left the view rendering "no deploys recorded yet" over a repo
			// full of them, and took as many presses to undo as it took to
			// get there.
			if m.offset < m.maxOffset() {
				m.offset++
			}
			return m, nil
		case "up", "k":
			if m.offset > 0 {
				m.offset--
			}
			return m, nil
		case "g":
			m.offset = 0
			return m, nil
		default:
			if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
				if i := int(key[0] - '1'); i < len(m.view.Flows) {
					m.selected = m.view.Flows[i].Name
				}
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m WeeklyModel) View() tea.View {
	v := tea.NewView(m.render())
	// v2 sets terminal features declaratively on the View each frame rather
	// than as a program option.
	v.AltScreen = true
	return v
}

// maxOffset is the furthest the history can scroll: far enough to bring the
// oldest week into view, and no further.
func (m WeeklyModel) maxOffset() int {
	total := 0
	for _, f := range m.view.Flows {
		if f.Name == m.selected {
			total = len(f.Weekly)
		}
	}
	if n := total - m.visibleWeeks(); n > 0 {
		return n
	}
	return 0
}

// visibleWeeks is how many week rows fit, leaving room for the strip, the
// header, the axis and the footer.
func (m WeeklyModel) visibleWeeks() int {
	chrome := 4 // header line, axis, footer, and the blank above the footer
	if len(m.view.Flows) > 1 {
		chrome += 2 // strip plus its blank line
	}
	n := m.height - chrome
	if n < 1 {
		return 1
	}
	return n
}

func (m WeeklyModel) render() string {
	flows := m.view.Flows
	if len(flows) == 0 {
		return "no deploy flows\n"
	}

	// Scroll by trimming the flow's weeks rather than the rendered lines, so
	// the axis and header stay put while the history moves under them.
	windowed := make([]core.FlowView, len(flows))
	copy(windowed, flows)
	limit := m.visibleWeeks()
	for i := range windowed {
		weeks := windowed[i].Weekly
		// Clamped here as well as on the key: switching to a flow with a
		// shorter history would otherwise strand the offset past its end.
		start := m.offset
		if max := len(weeks) - limit; start > max {
			start = max
		}
		if start < 0 {
			start = 0
		}
		end := start + limit
		if end > len(weeks) {
			end = len(weeks)
		}
		windowed[i].Weekly = weeks[start:end]
	}

	shown := m.view
	shown.Flows = windowed
	body := RenderWeekly(shown, m.selected, m.width)
	return body + "\n" + m.footer()
}

func (m WeeklyModel) footer() string {
	var keys []string
	if len(m.view.Flows) > 1 {
		keys = append(keys, "tab switch deploy")
	}
	total := 0
	for _, f := range m.view.Flows {
		if f.Name == m.selected {
			total = len(f.Weekly)
		}
	}
	if total > m.visibleWeeks() {
		// The way back is advertised alongside the way down. Scrolling a long
		// history is easy; remembering an undocumented key to undo it is not.
		keys = append([]string{"↑↓ scroll", "g top"}, keys...)
	}
	return dimStyle().Render("  "+strings.Join(keys, "   ")) + "\n"
}

// RunWeekly starts the interactive weekly view over a single view.
func RunWeekly(ctx context.Context, view core.View, selected string) error {
	if selected != "" {
		if _, ok := core.MatchFlow(view.Flows, selected); !ok {
			return fmt.Errorf("no deploy flow named %q — this repo has: %s",
				selected, strings.Join(core.FlowNames(view.Flows), ", "))
		}
	}
	// Force the lazy background-colour detection to fire BEFORE bubbletea
	// claims stdin, for the same reason the live TUI does: the OSC round trip
	// would otherwise happen inside the event loop, racing bubbletea for the
	// same fd and blocking the first frame.
	detectTerminal()
	p := tea.NewProgram(
		NewWeeklyModel(view, selected, 100, 30),
		tea.WithContext(ctx),
	)
	_, err := p.Run()
	return err
}
