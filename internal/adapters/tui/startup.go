package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Startup states: everything the TUI can show before the first View
// arrives. The program starts the moment the command is typed — anything
// between "typed the command" and "seeing something" is where trust dies —
// so config reading, fetching and validation all happen behind a live
// frame, reporting here instead of to a stderr the alt-screen would eat.

// StartupPhaseMsg names the startup step now running ("reading clarity
// config (refs/clarity/config)", "fetching master from origin", …). Shown
// in place of the old bare "Loading", so a stall names its stage instead
// of lying that progress is being made.
type StartupPhaseMsg string

// StartupErrorMsg ends startup: the dashboard cannot start, and the error
// — with whatever hint the orchestrator attached — renders in-frame until
// the user quits.
type StartupErrorMsg struct{ Err error }

// WelcomeMsg switches to the welcome pane: the repo has no clarity config
// ref, so instead of a dashboard where every commit sits meaninglessly
// "before CI", the user gets told what is missing and offered the setup.
type WelcomeMsg struct {
	// Branch is the detected trunk, so the pane can show the repo was
	// understood even though nothing is configured.
	Branch string
}

// WelcomeChoice is what the user picked on the welcome pane. Exit is
// handled inside the Model (it just quits); only the choice that needs
// the orchestrator to act crosses the channel.
type WelcomeChoice string

// WelcomeSetup asks the orchestrator to run setup: write
// refs/clarity/config and push it, then carry on into the dashboard.
const WelcomeSetup WelcomeChoice = "setup"

// welcomeOptions are the selectable actions, in render order. On-screen
// options navigated with arrows and confirmed with enter — never bare
// hotkeys, which are invisible and instant.
var welcomeOptions = [...]string{"Set up this repo", "Exit"}

// WithStartupChoices returns a copy of m that reports welcome-pane choices
// on ch. The send never blocks (the orchestrator may have gone away); a
// buffered channel is the caller's job.
func (m Model) WithStartupChoices(ch chan<- WelcomeChoice) Model {
	m.choices = ch
	return m
}

// updateWelcome handles keys while the welcome pane is up. Arrows (and
// tab) move the selection, enter confirms, q still quits.
func (m Model) updateWelcome(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "shift+tab":
		m.welcomeIdx = (m.welcomeIdx + len(welcomeOptions) - 1) % len(welcomeOptions)
	case "down", "tab":
		m.welcomeIdx = (m.welcomeIdx + 1) % len(welcomeOptions)
	case "enter":
		if welcomeOptions[m.welcomeIdx] == "Exit" {
			return m, tea.Quit
		}
		m.welcome = false
		m.startupPhase = "setting up — writing refs/clarity/config and pushing it to origin"
		if m.choices != nil {
			select {
			case m.choices <- WelcomeSetup:
			default:
			}
		}
	}
	m.viewport.SetContent(m.renderBody())
	return m, nil
}

// renderWelcome draws the welcome pane. It leads with what clarity did
// figure out (the trunk), so the pane reads as a dashboard that knows the
// repo and is missing only its events — not as an error.
func renderWelcome(branch string, selected int) string {
	var b strings.Builder
	b.WriteString("\n  " + lipgloss.NewStyle().Bold(true).Render("clarity isn't set up on this repository yet") + "\n\n")
	b.WriteString("  The trunk looks like " + branch + ", but there are no CI or deploy events\n")
	b.WriteString("  to light the dashboard up — without them every commit just waits at\n")
	b.WriteString("  \"before CI\".\n\n")
	b.WriteString("  Setting up writes refs/clarity/config and pushes it to origin. That ref\n")
	b.WriteString("  holds clarity's shared config and marks the repo as onboarded; your\n")
	b.WriteString("  pipeline then reports events with `git clarity report` — and\n")
	b.WriteString("  `git clarity onboard` prints a prompt that hands that wiring to an\n")
	b.WriteString("  AI agent.\n\n")
	for i, opt := range welcomeOptions {
		if i == selected {
			b.WriteString("  " + lipgloss.NewStyle().Bold(true).Foreground(colorBlue).Render("> "+opt) + "\n")
		} else {
			b.WriteString("    " + opt + "\n")
		}
	}
	b.WriteString("\n  ↑/↓ choose · enter confirm · q quit\n")
	return b.String()
}

// renderStartupError draws a startup failure in-frame: the error, its
// hint lines, and the way out.
func renderStartupError(err error) string {
	var b strings.Builder
	b.WriteString("\n  " + lipgloss.NewStyle().Bold(true).Foreground(colorRed).Render("clarity could not start") + "\n\n")
	for _, line := range strings.Split(err.Error(), "\n") {
		b.WriteString("  " + line + "\n")
	}
	b.WriteString("\n  press q to quit\n")
	return b.String()
}
