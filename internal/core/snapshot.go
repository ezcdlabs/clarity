// Package core is the pure data layer that drives clarity's TUI / plain
// renderers. It owns the joined Snapshot shape, the lifecycle grouping
// (HEAD / CI Passed / Deployed), per-commit lead time computation, and
// the DORA weekly throughput aggregation. The core has no I/O — no `os`,
// `net`, `bubbletea`, or `gogit` dependencies — so every transformation
// is unit-testable as a pure function against typed inputs.
package core

import (
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
)

// Snapshot is the joined commits + events data ordered newest-first.
// Produced by Source adapters via BuildSnapshot and consumed by the Lens,
// which runs DeriveView on each one.
type Snapshot struct {
	Commits []CommitView
	// RepoName is the human-readable name of the repository this snapshot
	// was derived from, displayed by Renderers in their header. Set by the
	// Source adapter; not derived from the data.
	RepoName string
	// Truncated reports that the branch has more commits than Commits
	// holds — the walk stopped at Limit rather than at the root commit.
	// Set by the Source adapter, which is the only layer that can see
	// past the cut. Two things depend on it: WeeklyStats drops its
	// oldest bucket (the window boundary cuts through that week, so its
	// counts would understate), and Renderers close the list with a note
	// saying the limit, not the history, is what ended it.
	Truncated bool
	// Limit is the commit cap that produced this snapshot, carried so
	// Renderers can name it in that note. Meaningless when Truncated is
	// false.
	Limit int
}

// Commit is the raw metadata for one commit, before any pipeline events
// have been joined in. Source adapters produce []Commit (typically from
// git log) and pass it to BuildSnapshot to assemble a Snapshot.
type Commit struct {
	SHA     string
	Subject string
	Author  string
	Time    time.Time
}

// CommitView is one commit joined with its pipeline events — the shape
// the lens and renderers walk.
type CommitView struct {
	SHA     string
	Subject string
	Author  string
	Time    time.Time
	Events  []clarityrefs.Event
}
