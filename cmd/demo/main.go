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
	"github.com/ezcdlabs/clarity/internal/adapters/tui"
	"github.com/ezcdlabs/clarity/internal/core"
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

// play runs the scenario in two phases:
//  1. Prelude — print the (typed) shell prompt + command to the regular
//     terminal so viewers see how the tool is invoked. Phase 1 happens
//     BEFORE the TUI claims the alt-screen.
//  2. TUI — the views channel feeds scripted frames; once the last
//     frame's hold elapses we Send a synthetic "q" key so the alt-screen
//     exits cleanly and the recording terminates.
//
// The TUI's clock is anchored at demoBase plus elapsed real time *since
// the TUI started* (not since play() started), so lead-time timers begin
// at the scenario's reference offsets regardless of how long the prelude
// took. Same trick pushq's demo uses.
//
// Scripted Snapshots are passed through core.DeriveView inline instead of
// being routed through a fake Source + core.Lens. The lens path would buy
// us an extra goroutine and channel-close coordination to do exactly the
// same one-line transform — DeriveView is pure, so the demo just calls it.
func play(s *Scenario) error {
	playPrelude(s.Prelude)

	scenarioStart := time.Now()
	nowFn := func() time.Time { return demoBase.Add(time.Since(scenarioStart)) }

	views := make(chan core.View, 1)
	p := tui.NewProgramWithClock(views, nowFn)

	go func() {
		// Initial loading delay — viewers see the "Loading…" state briefly
		// before content appears.
		time.Sleep(s.InitialDelay)
		for _, f := range s.Frames {
			snap := f.Snapshot
			if snap.RepoName == "" {
				snap.RepoName = s.Repo
			}
			views <- core.DeriveView(snap, core.DefaultLeadTimeMode)
			time.Sleep(f.Hold)
		}
		close(views)
		// Synthetic quit — Bubble Tea's alt-screen would otherwise stay
		// resident, leaving the recording hanging on the last frame.
		// v2: KeyMsg is an interface; synthesize a 'q' press via KeyPressMsg
		// (Code is a rune, Text is the textual form for String() matching).
		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	}()

	_, err := p.Run()
	playPostlude(s.Prelude)
	return err
}

// playPostlude reprints the shell prompt after the TUI exits so the
// recording ends back "at the shell" — without it, alt-screen exits to
// reveal the still-visible typed command, which reads like the program
// hung. Held briefly so asciinema captures the fresh prompt as the final
// frame instead of cutting off mid-restore.
func playPostlude(prelude []PreludeLine) {
	if len(prelude) == 0 {
		return
	}
	const trailingPause = 600 * time.Millisecond
	for _, line := range prelude {
		if line.NoNewline {
			fmt.Print(line.Text)
			break
		}
	}
	time.Sleep(trailingPause)
}

// playPrelude streams the prelude to stdout BEFORE the TUI starts. Typing
// lines are emitted one rune at a time at roughly the typing speed used by
// pushq's recordings (45ms inter-keystroke) so the recorded session reads
// like a real keyboard rather than an instant paste. Holds for a short beat
// at the end so viewers can read the full command before the alt-screen
// takes over — and so the recording feels like a real "press Enter and the
// program loads", not an instant flicker.
func playPrelude(lines []PreludeLine) {
	if len(lines) == 0 {
		return
	}
	const typingPause = 45 * time.Millisecond
	const trailingPause = 700 * time.Millisecond
	for _, line := range lines {
		if line.Delay > 0 {
			time.Sleep(line.Delay)
		}
		switch {
		case line.Typing:
			runes := []rune(line.Text)
			for i, ch := range runes {
				fmt.Print(string(ch))
				if i < len(runes)-1 {
					time.Sleep(typingPause)
				}
			}
			fmt.Println()
		case line.NoNewline:
			fmt.Print(line.Text)
		default:
			fmt.Println(line.Text)
		}
	}
	time.Sleep(trailingPause)
}
