package main

import (
	"fmt"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

// Frame is a single demo step: the snapshot to display and how long to hold
// it before advancing.
type Frame struct {
	Snapshot watcher.Snapshot
	Hold     time.Duration
}

// PreludeLine is one line written to the regular terminal BEFORE the TUI
// takes over the alt-screen. Used to stage the "user typed `git clarity` at
// the shell" framing the viewer sees at the start of every recording.
type PreludeLine struct {
	Text      string
	Delay     time.Duration // wait BEFORE printing this line
	Typing    bool          // print one rune at a time so it looks like a real keystroke stream
	NoNewline bool          // suppress the trailing newline (used for the prompt itself)
}

// Scenario is a named sequence of frames driven through the real TUI for
// recording.
type Scenario struct {
	Name         string
	Repo         string        // shown in the header (in place of a real repo name)
	Prelude      []PreludeLine // shell prompt + typed command before the TUI starts
	InitialDelay time.Duration // hold the "Loading…" state before the first snapshot
	Frames       []Frame
}

// shellPrompt builds a colored bash-style prompt: green user@host, blue
// path, default-foreground "$". Inline ANSI escapes (no lipgloss) so the
// recording's byte stream is deterministic regardless of terminal probing.
func shellPrompt(user, host, path string) string {
	return fmt.Sprintf("\x1b[32m%s@%s\x1b[0m:\x1b[34m%s\x1b[0m$ ", user, host, path)
}

// sharedPrelude is the standard "user typed `git clarity` at the shell"
// intro every scenario uses — keeps the recordings visually consistent so
// viewers see the same framing across the happy path / failure variants.
var sharedPrelude = []PreludeLine{
	{Text: shellPrompt("you", "workstation", "~/Projects/your-app"), Delay: 400 * time.Millisecond, NoNewline: true},
	{Text: "git clarity", Delay: 300 * time.Millisecond, Typing: true},
}

// demoBase is the virtual "now" anchor for all times in the scenarios. The
// playback loop updates the TUI's clock to track demoBase + (real elapsed
// since playback started), so lead-time timers count up at real-world pace
// while the data refers to fixed offsets from this anchor.
var demoBase = time.Date(2026, 5, 9, 9, 0, 0, 0, time.UTC)

func at(off time.Duration) time.Time { return demoBase.Add(off) }

func ev(stage, status string, off time.Duration) clarityrefs.Event {
	return clarityrefs.Event{Stage: stage, Status: status, Time: at(off)}
}

// commit is a small constructor to keep scenario data readable.
func commit(sha, author, subject string, commitOffset time.Duration, events ...clarityrefs.Event) watcher.CommitView {
	return watcher.CommitView{
		SHA:     sha,
		Author:  author,
		Subject: subject,
		Time:    at(commitOffset),
		Events:  events,
	}
}

// happyPath walks through every section of the redesigned TUI:
//   - frame 1: a steady state with NeedsCI, NextDeploy and Deployed all populated
//   - frame 2: a build completes — a commit moves from NeedsCI to NextDeploy
//   - frame 3: a deploy starts — the Next deploy header annotates "deploying…"
//   - frame 4: the deploy completes — the NextDeploy batch fix-forwards into Deployed
//
// An older deploy (ivy, 8 days back) sits in the previous ISO week so the
// Deployed section renders both the merged "Deployed … W<this-week>" header
// AND a standalone "W<last-week>" divider above the older batch.
var happyPath = Scenario{
	Name:         "happy-path",
	Repo:         "your-app",
	Prelude:      sharedPrelude,
	InitialDelay: 800 * time.Millisecond,
	Frames: []Frame{
		// Frame 1 — steady state.
		// NeedsCI: bob (no events), dave (build started)
		// NextDeploy: alice, carol (built but not yet deployed)
		// Deployed: frank (production), grace (older history)
		{
			Snapshot: watcher.Snapshot{Commits: []watcher.CommitView{
				commit("bob01", "bob", "add billing endpoint", -30*time.Second),
				commit("dave02", "dave", "update dependencies", -2*time.Minute,
					ev("ci", "started", -90*time.Second),
				),
				commit("alice3", "alice", "refactor user model", -5*time.Minute,
					ev("ci", "passed", -4*time.Minute),
				),
				commit("carol4", "carol", "tweak homepage", -10*time.Minute,
					ev("ci", "passed", -9*time.Minute),
				),
				commit("frank5", "frank", "fix payment bug", -25*time.Minute,
					ev("ci", "passed", -24*time.Minute),
					ev("deploy", "passed", -5*time.Minute),
				),
				commit("grace6", "grace", "improve search index", -1*time.Hour,
					ev("ci", "passed", -59*time.Minute),
					ev("deploy", "passed", -30*time.Minute),
				),
				commit("ivy007", "ivy", "rewrite onboarding flow", -8*24*time.Hour-30*time.Minute,
					ev("ci", "passed", -8*24*time.Hour-15*time.Minute),
					ev("deploy", "passed", -8*24*time.Hour),
				),
			}},
			Hold: 2500 * time.Millisecond,
		},

		// Frame 2 — dave's build passes.
		// Build line moves to dave; he joins NextDeploy.
		{
			Snapshot: watcher.Snapshot{Commits: []watcher.CommitView{
				commit("bob01", "bob", "add billing endpoint", -30*time.Second),
				commit("dave02", "dave", "update dependencies", -2*time.Minute,
					ev("ci", "started", -90*time.Second),
					ev("ci", "passed", 2*time.Second),
				),
				commit("alice3", "alice", "refactor user model", -5*time.Minute,
					ev("ci", "passed", -4*time.Minute),
				),
				commit("carol4", "carol", "tweak homepage", -10*time.Minute,
					ev("ci", "passed", -9*time.Minute),
				),
				commit("frank5", "frank", "fix payment bug", -25*time.Minute,
					ev("ci", "passed", -24*time.Minute),
					ev("deploy", "passed", -5*time.Minute),
				),
				commit("grace6", "grace", "improve search index", -1*time.Hour,
					ev("ci", "passed", -59*time.Minute),
					ev("deploy", "passed", -30*time.Minute),
				),
				commit("ivy007", "ivy", "rewrite onboarding flow", -8*24*time.Hour-30*time.Minute,
					ev("ci", "passed", -8*24*time.Hour-15*time.Minute),
					ev("deploy", "passed", -8*24*time.Hour),
				),
			}},
			Hold: 2200 * time.Millisecond,
		},

		// Frame 3 — a deploy kicks off.
		// dave's commit gets a deploy:started event; the section header annotates "deploying…"
		// and the entire batch (dave, alice, carol) is implicitly being deployed.
		{
			Snapshot: watcher.Snapshot{Commits: []watcher.CommitView{
				commit("bob01", "bob", "add billing endpoint", -30*time.Second),
				commit("dave02", "dave", "update dependencies", -2*time.Minute,
					ev("ci", "passed", 2*time.Second),
					ev("deploy", "started", 4*time.Second),
				),
				commit("alice3", "alice", "refactor user model", -5*time.Minute,
					ev("ci", "passed", -4*time.Minute),
				),
				commit("carol4", "carol", "tweak homepage", -10*time.Minute,
					ev("ci", "passed", -9*time.Minute),
				),
				commit("frank5", "frank", "fix payment bug", -25*time.Minute,
					ev("ci", "passed", -24*time.Minute),
					ev("deploy", "passed", -5*time.Minute),
				),
				commit("grace6", "grace", "improve search index", -1*time.Hour,
					ev("ci", "passed", -59*time.Minute),
					ev("deploy", "passed", -30*time.Minute),
				),
				commit("ivy007", "ivy", "rewrite onboarding flow", -8*24*time.Hour-30*time.Minute,
					ev("ci", "passed", -8*24*time.Hour-15*time.Minute),
					ev("deploy", "passed", -8*24*time.Hour),
				),
			}},
			Hold: 2000 * time.Millisecond,
		},

		// Frame 4 — deploy completes.
		// dave's deploy passes; deploy line moves to dave and the whole NextDeploy
		// batch (alice, carol) gets fix-forwarded into Deployed at dave's deploy time.
		// frank stays frozen at his OWN earlier deploy time (own-deploy wins).
		{
			Snapshot: watcher.Snapshot{Commits: []watcher.CommitView{
				commit("bob01", "bob", "add billing endpoint", -30*time.Second),
				commit("dave02", "dave", "update dependencies", -2*time.Minute,
					ev("ci", "passed", 2*time.Second),
					ev("deploy", "started", 4*time.Second),
					ev("deploy", "passed", 7*time.Second),
				),
				commit("alice3", "alice", "refactor user model", -5*time.Minute,
					ev("ci", "passed", -4*time.Minute),
				),
				commit("carol4", "carol", "tweak homepage", -10*time.Minute,
					ev("ci", "passed", -9*time.Minute),
				),
				commit("frank5", "frank", "fix payment bug", -25*time.Minute,
					ev("ci", "passed", -24*time.Minute),
					ev("deploy", "passed", -5*time.Minute),
				),
				commit("grace6", "grace", "improve search index", -1*time.Hour,
					ev("ci", "passed", -59*time.Minute),
					ev("deploy", "passed", -30*time.Minute),
				),
				commit("ivy007", "ivy", "rewrite onboarding flow", -8*24*time.Hour-30*time.Minute,
					ev("ci", "passed", -8*24*time.Hour-15*time.Minute),
					ev("deploy", "passed", -8*24*time.Hour),
				),
			}},
			Hold: 1800 * time.Millisecond,
		},
	},
}

// deployFailure shows the "failed deploy → newer deploy starts → merge"
// transition from the README's TBD model:
//   - frame 1: alice's deploy is in flight
//   - frame 2: alice's deploy fails — a "deploy failed" batch appears
//   - frame 3: bob lands and starts a newer deploy — alice is absorbed into
//     bob's now-deploying batch (no separate failed group anymore)
var deployFailure = Scenario{
	Name:         "deploy-failure",
	Repo:         "your-app",
	Prelude:      sharedPrelude,
	InitialDelay: 800 * time.Millisecond,
	Frames: []Frame{
		// Frame 1 — alice is mid-deploy.
		{
			Snapshot: watcher.Snapshot{Commits: []watcher.CommitView{
				commit("alice1", "alice", "refactor user model", -3*time.Minute,
					ev("ci", "passed", -150*time.Second),
					ev("deploy", "started", -30*time.Second),
				),
				commit("frank2", "frank", "fix payment bug", -25*time.Minute,
					ev("ci", "passed", -24*time.Minute),
					ev("deploy", "passed", -8*time.Minute),
				),
			}},
			Hold: 2200 * time.Millisecond,
		},

		// Frame 2 — alice's deploy fails. Standalone failed batch.
		{
			Snapshot: watcher.Snapshot{Commits: []watcher.CommitView{
				commit("alice1", "alice", "refactor user model", -3*time.Minute,
					ev("ci", "passed", -150*time.Second),
					ev("deploy", "started", -30*time.Second),
					ev("deploy", "failed", 3*time.Second),
				),
				commit("frank2", "frank", "fix payment bug", -25*time.Minute,
					ev("ci", "passed", -24*time.Minute),
					ev("deploy", "passed", -8*time.Minute),
				),
			}},
			Hold: 2400 * time.Millisecond,
		},

		// Frame 3 — bob lands as the fix-forward and starts deploying. alice's
		// failed batch is absorbed into bob's "deploying…" batch.
		{
			Snapshot: watcher.Snapshot{Commits: []watcher.CommitView{
				commit("bob003", "bob", "patch token validation", -10*time.Second,
					ev("ci", "passed", 6*time.Second),
					ev("deploy", "started", 8*time.Second),
				),
				commit("alice1", "alice", "refactor user model", -3*time.Minute,
					ev("ci", "passed", -150*time.Second),
					ev("deploy", "started", -30*time.Second),
					ev("deploy", "failed", 3*time.Second),
				),
				commit("frank2", "frank", "fix payment bug", -25*time.Minute,
					ev("ci", "passed", -24*time.Minute),
					ev("deploy", "passed", -8*time.Minute),
				),
			}},
			Hold: 2200 * time.Millisecond,
		},
	},
}

// ciFailure shows the "CI breaks then a newer commit fix-forwards it"
// transition. This is the core TBD signal the tool is designed to surface:
//   - frame 1: steady state, everything green
//   - frame 2: bob lands at HEAD, his CI starts (header stays green —
//     only resolved events drive the badge)
//   - frame 3: bob's CI fails — header badge flips red, his row in HEAD
//     shows a red ✗
//   - frame 4: dave lands a fix on top, his CI starts (header still red,
//     newest resolved event is bob's failure)
//   - frame 5: dave's CI passes — header back to green; bob's red ✗
//     becomes stale (a newer commit passed) and dims to gray as both
//     dave and bob move into CI Passed under the new build line
var ciFailure = Scenario{
	Name:         "ci-failure",
	Repo:         "your-app",
	Prelude:      sharedPrelude,
	InitialDelay: 800 * time.Millisecond,
	Frames: []Frame{
		// Frame 1 — steady state, header green/green.
		{
			Snapshot: watcher.Snapshot{Commits: []watcher.CommitView{
				commit("alice1", "alice", "refactor user model", -10*time.Minute,
					ev("ci", "passed", -9*time.Minute),
					ev("deploy", "passed", -3*time.Minute),
				),
				commit("frank2", "frank", "fix payment bug", -25*time.Minute,
					ev("ci", "passed", -24*time.Minute),
					ev("deploy", "passed", -15*time.Minute),
				),
			}},
			Hold: 2200 * time.Millisecond,
		},

		// Frame 2 — bob lands at HEAD, CI starts. Header stays green
		// because the LATEST RESOLVED ci event is still alice's pass.
		{
			Snapshot: watcher.Snapshot{Commits: []watcher.CommitView{
				commit("bob03", "bob", "add billing endpoint", -30*time.Second,
					ev("ci", "started", -15*time.Second),
				),
				commit("alice1", "alice", "refactor user model", -10*time.Minute,
					ev("ci", "passed", -9*time.Minute),
					ev("deploy", "passed", -3*time.Minute),
				),
				commit("frank2", "frank", "fix payment bug", -25*time.Minute,
					ev("ci", "passed", -24*time.Minute),
					ev("deploy", "passed", -15*time.Minute),
				),
			}},
			Hold: 2000 * time.Millisecond,
		},

		// Frame 3 — bob's CI FAILS. Header badge flips red. Bob's row
		// in HEAD picks up the red ✗ icon.
		{
			Snapshot: watcher.Snapshot{Commits: []watcher.CommitView{
				commit("bob03", "bob", "add billing endpoint", -30*time.Second,
					ev("ci", "started", -15*time.Second),
					ev("ci", "failed", 5*time.Second),
				),
				commit("alice1", "alice", "refactor user model", -10*time.Minute,
					ev("ci", "passed", -9*time.Minute),
					ev("deploy", "passed", -3*time.Minute),
				),
				commit("frank2", "frank", "fix payment bug", -25*time.Minute,
					ev("ci", "passed", -24*time.Minute),
					ev("deploy", "passed", -15*time.Minute),
				),
			}},
			Hold: 2500 * time.Millisecond,
		},

		// Frame 4 — dave lands a fix on top. His CI is started; bob is
		// still failed. Header stays red — newest resolved is bob's fail.
		{
			Snapshot: watcher.Snapshot{Commits: []watcher.CommitView{
				commit("dave04", "dave", "fix the billing bug", 8*time.Second,
					ev("ci", "started", 10*time.Second),
				),
				commit("bob03", "bob", "add billing endpoint", -30*time.Second,
					ev("ci", "started", -15*time.Second),
					ev("ci", "failed", 5*time.Second),
				),
				commit("alice1", "alice", "refactor user model", -10*time.Minute,
					ev("ci", "passed", -9*time.Minute),
					ev("deploy", "passed", -3*time.Minute),
				),
				commit("frank2", "frank", "fix payment bug", -25*time.Minute,
					ev("ci", "passed", -24*time.Minute),
					ev("deploy", "passed", -15*time.Minute),
				),
			}},
			Hold: 1800 * time.Millisecond,
		},

		// Frame 5 — dave's CI PASSES. Header back to green. dave becomes
		// the new build line; bob (older than dave) is fix-forwarded out
		// of HEAD into CI Passed. Bob's individual ci event is still
		// "failed" but stale, so his row icon dims to gray ✗.
		{
			Snapshot: watcher.Snapshot{Commits: []watcher.CommitView{
				commit("dave04", "dave", "fix the billing bug", 8*time.Second,
					ev("ci", "started", 10*time.Second),
					ev("ci", "passed", 20*time.Second),
				),
				commit("bob03", "bob", "add billing endpoint", -30*time.Second,
					ev("ci", "started", -15*time.Second),
					ev("ci", "failed", 5*time.Second),
				),
				commit("alice1", "alice", "refactor user model", -10*time.Minute,
					ev("ci", "passed", -9*time.Minute),
					ev("deploy", "passed", -3*time.Minute),
				),
				commit("frank2", "frank", "fix payment bug", -25*time.Minute,
					ev("ci", "passed", -24*time.Minute),
					ev("deploy", "passed", -15*time.Minute),
				),
			}},
			Hold: 2500 * time.Millisecond,
		},
	},
}

var allScenarios = map[string]*Scenario{
	"happy-path":     &happyPath,
	"deploy-failure": &deployFailure,
	"ci-failure":     &ciFailure,
}
