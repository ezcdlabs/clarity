package main

import (
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

// Scenario is a named sequence of frames driven through the real TUI for
// recording.
type Scenario struct {
	Name         string
	Branch       string
	InitialDelay time.Duration // hold the "Loading…" state before the first snapshot
	Frames       []Frame
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
var happyPath = Scenario{
	Name:         "happy-path",
	Branch:       "main",
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
					ev("build", "started", -90*time.Second),
				),
				commit("alice3", "alice", "refactor user model", -5*time.Minute,
					ev("build", "passed", -4*time.Minute),
				),
				commit("carol4", "carol", "tweak homepage", -10*time.Minute,
					ev("build", "passed", -9*time.Minute),
				),
				commit("frank5", "frank", "fix payment bug", -25*time.Minute,
					ev("build", "passed", -24*time.Minute),
					ev("deploy", "passed", -5*time.Minute),
				),
				commit("grace6", "grace", "improve search index", -1*time.Hour,
					ev("build", "passed", -59*time.Minute),
					ev("deploy", "passed", -30*time.Minute),
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
					ev("build", "started", -90*time.Second),
					ev("build", "passed", 2*time.Second),
				),
				commit("alice3", "alice", "refactor user model", -5*time.Minute,
					ev("build", "passed", -4*time.Minute),
				),
				commit("carol4", "carol", "tweak homepage", -10*time.Minute,
					ev("build", "passed", -9*time.Minute),
				),
				commit("frank5", "frank", "fix payment bug", -25*time.Minute,
					ev("build", "passed", -24*time.Minute),
					ev("deploy", "passed", -5*time.Minute),
				),
				commit("grace6", "grace", "improve search index", -1*time.Hour,
					ev("build", "passed", -59*time.Minute),
					ev("deploy", "passed", -30*time.Minute),
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
					ev("build", "passed", 2*time.Second),
					ev("deploy", "started", 4*time.Second),
				),
				commit("alice3", "alice", "refactor user model", -5*time.Minute,
					ev("build", "passed", -4*time.Minute),
				),
				commit("carol4", "carol", "tweak homepage", -10*time.Minute,
					ev("build", "passed", -9*time.Minute),
				),
				commit("frank5", "frank", "fix payment bug", -25*time.Minute,
					ev("build", "passed", -24*time.Minute),
					ev("deploy", "passed", -5*time.Minute),
				),
				commit("grace6", "grace", "improve search index", -1*time.Hour,
					ev("build", "passed", -59*time.Minute),
					ev("deploy", "passed", -30*time.Minute),
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
					ev("build", "passed", 2*time.Second),
					ev("deploy", "started", 4*time.Second),
					ev("deploy", "passed", 7*time.Second),
				),
				commit("alice3", "alice", "refactor user model", -5*time.Minute,
					ev("build", "passed", -4*time.Minute),
				),
				commit("carol4", "carol", "tweak homepage", -10*time.Minute,
					ev("build", "passed", -9*time.Minute),
				),
				commit("frank5", "frank", "fix payment bug", -25*time.Minute,
					ev("build", "passed", -24*time.Minute),
					ev("deploy", "passed", -5*time.Minute),
				),
				commit("grace6", "grace", "improve search index", -1*time.Hour,
					ev("build", "passed", -59*time.Minute),
					ev("deploy", "passed", -30*time.Minute),
				),
			}},
			Hold: 1800 * time.Millisecond,
		},
	},
}

var allScenarios = map[string]*Scenario{
	"happy-path": &happyPath,
}
