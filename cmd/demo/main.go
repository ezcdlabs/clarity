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

	tea "charm.land/bubbletea/v2"
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
//
// The TUI's clock is anchored at demoBase plus elapsed real time, so
// lead-time timers tick at real pace while scenario data uses fixed offsets
// from a stable reference. This is the same trick pushq's demo uses.
func play(s *Scenario) error {
	scenarioStart := time.Now()
	nowFn := func() time.Time { return demoBase.Add(time.Since(scenarioStart)) }

	snapshots := make(chan watcher.Snapshot, 1)
	p := tui.NewProgramWithClock(s.Repo, snapshots, nowFn)

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
		// v2: KeyMsg is an interface; synthesize a 'q' press via KeyPressMsg
		// (Code is a rune, Text is the textual form for String() matching).
		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	}()

	_, err := p.Run()
	return err
}
