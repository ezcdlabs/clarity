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
// config (.clarity on the trunk)", "fetching master from origin", …). Shown
// in place of the old bare "Loading", so a stall names its stage instead
// of lying that progress is being made.
type StartupPhaseMsg string

// StartupErrorMsg ends startup: the dashboard cannot start, and the error
// — with whatever hint the orchestrator attached — renders in-frame until
// the user quits.
type StartupErrorMsg struct{ Err error }

// WelcomeMsg switches to the welcome pane: the repo has no clarity config,
// so instead of a dashboard where every commit sits meaninglessly
// "before CI", the user gets told what is missing and offered the setup.
type WelcomeMsg struct {
	// Branch is the detected trunk, so the pane can show the repo was
	// understood even though nothing is configured.
	Branch string
}

// SetupDoneMsg switches the welcome pane to its done state: setup wrote
// .clarity into the working tree, and the pane now shows — and keeps
// showing until acknowledged — that committing and pushing the file is
// what finishes setup. A transient phase line would scroll that away, and
// an instruction nobody saw is an instruction nobody follows.
type SetupDoneMsg struct {
	// Branch is the trunk the file must land on.
	Branch string
}

// WelcomeChoice is what the user picked on the welcome pane. Exit is
// handled inside the Model (it just quits); only the choices that need
// the orchestrator to act cross the channel.
type WelcomeChoice string

// WelcomeSetup asks the orchestrator to run setup: write .clarity into
// the working tree, then report back with a SetupDoneMsg.
const WelcomeSetup WelcomeChoice = "setup"

// WelcomeContinue acknowledges the setup-done pane: the user has read the
// commit-and-push instruction and the orchestrator may carry on into the
// dashboard.
const WelcomeContinue WelcomeChoice = "continue"

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
// tab) move the selection, enter confirms, q still quits. In the done
// state there is one action — acknowledging the commit instruction — so
// enter is it.
func (m Model) updateWelcome(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.welcomeDone {
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "enter":
			m.welcome, m.welcomeDone = false, false
			m.startupPhase = "starting the dashboard"
			if m.choices != nil {
				select {
				case m.choices <- WelcomeContinue:
				default:
				}
			}
		}
		m.viewport.SetContent(m.renderBody())
		return m, nil
	}
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
		m.startupPhase = "setting up — writing .clarity"
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
	b.WriteString("  Setting up writes a .clarity file at the repo root, for you to commit\n")
	b.WriteString("  and push — on the trunk it is reviewed and protected like any other\n")
	b.WriteString("  change, and it marks the repo as onboarded. Your pipeline then reports\n")
	b.WriteString("  events with `git clarity report` — and `git clarity onboard` prints a\n")
	b.WriteString("  prompt that hands that wiring to an AI agent.\n\n")
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

// renderSetupDone draws the pane shown after setup wrote .clarity: the
// commit-and-push instruction, held on screen until acknowledged, because
// it is the one step clarity cannot do for the user — pushing to the
// trunk itself would bypass the review the file moved there to get.
func renderSetupDone(branch string) string {
	var b strings.Builder
	b.WriteString("\n  " + lipgloss.NewStyle().Bold(true).Render(".clarity written") + "\n\n")
	b.WriteString("  Clarity's config lives in that file, read from " + branch + ". Commit and\n")
	b.WriteString("  push it to finish setup:\n\n")
	b.WriteString("    git add .clarity && git commit -m \"clarity: set up\" && git push\n\n")
	b.WriteString("  The dashboard works meanwhile; the file's values apply once it lands\n")
	b.WriteString("  on " + branch + ".\n\n")
	b.WriteString("  " + lipgloss.NewStyle().Bold(true).Foreground(colorBlue).Render("> Continue to the dashboard") + "\n")
	b.WriteString("\n  enter continue · q quit\n")
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
