// Command demo plays scripted snapshots through clarity's real TUI for the
// purpose of recording demonstration GIFs. Because the demo drives the same
// renderer the production binary uses, the recording can never drift from
// the real visual behaviour.
//
// Usage:
//
//	demo --play happy-path
//
// Or via the recording script:
//
//	./scripts/record-demo.sh happy-path
package main

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ezcdlabs/clarity/internal/tui"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

func main() {
	name := "happy-path"
	if len(os.Args) >= 3 && os.Args[1] == "--play" {
		name = os.Args[2]
	} else if len(os.Args) >= 2 && os.Args[1] != "--play" {
		fmt.Fprintln(os.Stderr, "usage: demo --play <scenario>")
		fmt.Fprintln(os.Stderr, "available scenarios:")
		for k := range allScenarios {
			fmt.Fprintf(os.Stderr, "  %s\n", k)
		}
		os.Exit(1)
	}

	s, ok := allScenarios[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown scenario: %q\n", name)
		os.Exit(1)
	}

	if err := play(s); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// play runs the scenario through the real TUI. The snapshots channel feeds
// scripted frames; once the last frame's hold elapses we Send a synthetic
// "q" key so the alt-screen exits cleanly and the recording terminates.
func play(s *Scenario) error {
	snapshots := make(chan watcher.Snapshot, 1)
	p := tui.NewProgram(s.Branch, snapshots)

	go func() {
		// Initial loading delay — viewers see the "Loading…" state briefly
		// before content appears.
		time.Sleep(s.InitialDelay)
		for _, f := range s.Frames {
			snapshots <- f.Snapshot
			time.Sleep(f.Hold)
		}
		close(snapshots)
		// Synthetic quit — Bubble Tea's alt-screen would otherwise stay
		// resident, leaving the recording hanging on the last frame.
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	}()

	_, err := p.Run()
	return err
}
