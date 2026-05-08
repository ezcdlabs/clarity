package main

import (
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

// Frame is a single demo step: the snapshot to display and how long to hold
// it before advancing. The very first frame's Hold runs *after* the initial
// "Loading…" pause so viewers see the loading state before content appears.
type Frame struct {
	Snapshot watcher.Snapshot
	Hold     time.Duration
}

// Scenario is a named sequence of frames driven through the real TUI for
// recording.
type Scenario struct {
	Name      string
	Branch    string
	InitialDelay time.Duration // hold the "Loading…" state for this long before the first snapshot
	Frames    []Frame
}

// fixed reference time so timestamps in the demo are reproducible across runs.
var demoBase = time.Date(2026, 5, 9, 9, 0, 0, 0, time.UTC)

func ev(stage, status string, offset time.Duration) clarityrefs.Event {
	return clarityrefs.Event{Stage: stage, Status: status, Time: demoBase.Add(offset)}
}

// happyPath shows the most common arc:
//   - initial paint with a mix of states (passed / running / failed / no-events)
//   - a running build transitions to passed
//   - a brand-new commit lands at the top with build still running
//   - that build then completes
var happyPath = Scenario{
	Name:         "happy-path",
	Branch:       "main",
	InitialDelay: 800 * time.Millisecond,
	Frames: []Frame{
		// Frame 1 — first paint
		{
			Snapshot: watcher.Snapshot{Commits: []watcher.CommitView{
				{SHA: "a1b2c3d", Author: "alice", Subject: "refactor user model",
					Time: demoBase.Add(-30 * time.Minute),
					Events: []clarityrefs.Event{
						ev("build", "passed", -29*time.Minute),
						ev("deploy", "passed", -28*time.Minute),
					}},
				{SHA: "b2c3d4e", Author: "dave", Subject: "update dependencies",
					Time: demoBase.Add(-12 * time.Minute),
					Events: []clarityrefs.Event{
						ev("build", "started", -11*time.Minute),
					}},
				{SHA: "c3d4e5f", Author: "eve", Subject: "new search index",
					Time: demoBase.Add(-25 * time.Minute),
					Events: []clarityrefs.Event{
						ev("build", "failed", -24*time.Minute),
					}},
				{SHA: "d4e5f6a", Author: "frank", Subject: "tweak homepage",
					Time: demoBase.Add(-45 * time.Minute),
					Events: []clarityrefs.Event{
						ev("build", "passed", -44*time.Minute),
						ev("deploy", "passed", -43*time.Minute),
					}},
				{SHA: "e5f6a7b", Author: "grace", Subject: "wip notes",
					Time: demoBase.Add(-50 * time.Minute),
				},
			}},
			Hold: 2200 * time.Millisecond,
		},

		// Frame 2 — dave's build passes
		{
			Snapshot: watcher.Snapshot{Commits: []watcher.CommitView{
				{SHA: "a1b2c3d", Author: "alice", Subject: "refactor user model",
					Events: []clarityrefs.Event{
						ev("build", "passed", -29*time.Minute),
						ev("deploy", "passed", -28*time.Minute),
					}},
				{SHA: "b2c3d4e", Author: "dave", Subject: "update dependencies",
					Events: []clarityrefs.Event{
						ev("build", "started", -11*time.Minute),
						ev("build", "passed", -10*time.Minute),
					}},
				{SHA: "c3d4e5f", Author: "eve", Subject: "new search index",
					Events: []clarityrefs.Event{
						ev("build", "failed", -24*time.Minute),
					}},
				{SHA: "d4e5f6a", Author: "frank", Subject: "tweak homepage",
					Events: []clarityrefs.Event{
						ev("build", "passed", -44*time.Minute),
						ev("deploy", "passed", -43*time.Minute),
					}},
				{SHA: "e5f6a7b", Author: "grace", Subject: "wip notes"},
			}},
			Hold: 1800 * time.Millisecond,
		},

		// Frame 3 — new commit lands at the top, build kicking off
		{
			Snapshot: watcher.Snapshot{Commits: []watcher.CommitView{
				{SHA: "f6a7b8c", Author: "bob", Subject: "add billing endpoint",
					Events: []clarityrefs.Event{
						ev("build", "started", 0),
					}},
				{SHA: "a1b2c3d", Author: "alice", Subject: "refactor user model",
					Events: []clarityrefs.Event{
						ev("build", "passed", -29*time.Minute),
						ev("deploy", "passed", -28*time.Minute),
					}},
				{SHA: "b2c3d4e", Author: "dave", Subject: "update dependencies",
					Events: []clarityrefs.Event{
						ev("build", "started", -11*time.Minute),
						ev("build", "passed", -10*time.Minute),
					}},
				{SHA: "c3d4e5f", Author: "eve", Subject: "new search index",
					Events: []clarityrefs.Event{
						ev("build", "failed", -24*time.Minute),
					}},
				{SHA: "d4e5f6a", Author: "frank", Subject: "tweak homepage",
					Events: []clarityrefs.Event{
						ev("build", "passed", -44*time.Minute),
						ev("deploy", "passed", -43*time.Minute),
					}},
			}},
			Hold: 1800 * time.Millisecond,
		},

		// Frame 4 — bob's build + deploy both pass
		{
			Snapshot: watcher.Snapshot{Commits: []watcher.CommitView{
				{SHA: "f6a7b8c", Author: "bob", Subject: "add billing endpoint",
					Events: []clarityrefs.Event{
						ev("build", "started", 0),
						ev("build", "passed", 30*time.Second),
						ev("deploy", "passed", 90*time.Second),
					}},
				{SHA: "a1b2c3d", Author: "alice", Subject: "refactor user model",
					Events: []clarityrefs.Event{
						ev("build", "passed", -29*time.Minute),
						ev("deploy", "passed", -28*time.Minute),
					}},
				{SHA: "b2c3d4e", Author: "dave", Subject: "update dependencies",
					Events: []clarityrefs.Event{
						ev("build", "started", -11*time.Minute),
						ev("build", "passed", -10*time.Minute),
					}},
				{SHA: "c3d4e5f", Author: "eve", Subject: "new search index",
					Events: []clarityrefs.Event{
						ev("build", "failed", -24*time.Minute),
					}},
				{SHA: "d4e5f6a", Author: "frank", Subject: "tweak homepage",
					Events: []clarityrefs.Event{
						ev("build", "passed", -44*time.Minute),
						ev("deploy", "passed", -43*time.Minute),
					}},
			}},
			Hold: 1500 * time.Millisecond,
		},
	},
}

var allScenarios = map[string]*Scenario{
	"happy-path": &happyPath,
}
