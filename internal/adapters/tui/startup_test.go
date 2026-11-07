package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestStartup_PhaseReplacesTheBareLoading: while the source is being
// prepared the body names the step actually running, so a stall is
// visibly a stalled fetch (or config read), never a mystery "Loading".
func TestStartup_PhaseReplacesTheBareLoading(t *testing.T) {
	m := New()
	m = update(t, m, StartupPhaseMsg("fetching master from origin"))
	body := m.renderBody()
	if !strings.Contains(body, "fetching master from origin") {
		t.Errorf("body does not show the running phase:\n%s", body)
	}
}

// TestWelcome_OffersSetupAndExit: a repo with no clarity config gets a
// welcome pane with two selectable options — no hidden hotkeys, the
// choices are on screen and picked with arrows + enter.
func TestWelcome_OffersSetupAndExit(t *testing.T) {
	m := New()
	m = update(t, m, WelcomeMsg{Branch: "master"})
	body := m.renderBody()
	for _, want := range []string{"Set up", "Exit", "master", "refs/clarity/config"} {
		if !strings.Contains(body, want) {
			t.Errorf("welcome pane is missing %q:\n%s", want, body)
		}
	}
}

// TestWelcome_EnterOnSetupSendsTheChoice: the default selection is
// "Set up this repo", and enter hands the choice to the orchestrator.
func TestWelcome_EnterOnSetupSendsTheChoice(t *testing.T) {
	choices := make(chan WelcomeChoice, 1)
	m := New().WithStartupChoices(choices)
	m = update(t, m, WelcomeMsg{Branch: "main"})
	m = update(t, m, KeyMsg("enter"))

	select {
	case got := <-choices:
		if got != WelcomeSetup {
			t.Errorf("choice = %q, want %q", got, WelcomeSetup)
		}
	default:
		t.Fatal("enter on the default option sent no choice")
	}
	// The pane is replaced by progress — the user acted, something must
	// visibly be happening.
	if body := m.renderBody(); !strings.Contains(body, "setting up") {
		t.Errorf("after choosing setup the body should show progress:\n%s", body)
	}
}

// TestWelcome_ArrowToExitQuits: down moves the selection to Exit, and
// enter there quits the program rather than setting anything up.
func TestWelcome_ArrowToExitQuits(t *testing.T) {
	choices := make(chan WelcomeChoice, 1)
	m := New().WithStartupChoices(choices)
	m = update(t, m, WelcomeMsg{Branch: "main"})

	mm, _ := m.Update(KeyMsg("down"))
	m = mm.(Model)
	mm, cmd := m.Update(KeyMsg("enter"))
	m = mm.(Model)

	if cmd == nil {
		t.Fatal("enter on Exit returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("enter on Exit should quit, got %T", cmd())
	}
	select {
	case got := <-choices:
		t.Errorf("Exit sent a choice %q; it must not", got)
	default:
	}
}

// TestWelcome_SelectionWraps: two options, so down twice is back where
// you started — the selector never runs off the end.
func TestWelcome_SelectionWraps(t *testing.T) {
	m := New()
	m = update(t, m, WelcomeMsg{Branch: "main"})
	m = update(t, m, KeyMsg("down"))
	m = update(t, m, KeyMsg("down"))

	mm, cmd := m.Update(KeyMsg("enter"))
	_ = mm
	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatal("selection did not wrap back to Set up — enter quit instead")
		}
	}
}

// TestStartupError_RendersInFrame: a startup failure (branch resolves
// nowhere, remote unreachable) is shown inside the TUI with its hint,
// not swallowed into a blank screen.
func TestStartupError_RendersInFrame(t *testing.T) {
	m := New()
	m = update(t, m, StartupErrorMsg{Err: errors.New("events source: no ref found for branch \"main\"\nthis repository's trunk looks like \"develop\"")})
	body := m.renderBody()
	for _, want := range []string{"no ref found", "develop", "q to quit"} {
		if !strings.Contains(body, want) {
			t.Errorf("error pane is missing %q:\n%s", want, body)
		}
	}
}

// TestViewMsg_EndsStartupStates: the first view replaces whatever startup
// was showing — phase line or welcome pane — with the dashboard.
func TestViewMsg_EndsStartupStates(t *testing.T) {
	m := New()
	m = update(t, m, StartupPhaseMsg("reading events"))
	m = update(t, m, ViewMsg{})
	if body := m.renderBody(); strings.Contains(body, "reading events") {
		t.Errorf("phase line survived the first view:\n%s", body)
	}
}

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	mm, _ := m.Update(msg)
	out, ok := mm.(Model)
	if !ok {
		t.Fatalf("Update returned a %T, not a Model", mm)
	}
	return out
}
