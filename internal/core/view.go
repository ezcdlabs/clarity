package core

// View is the fully-derived state ready for rendering. Built from a raw
// Snapshot by DeriveView. Renderers consume View directly and never
// re-derive — keeps the rendering layer dumb and the derivation layer the
// single source of grouping / DORA / stale truth.
type View struct {
	Snapshot Snapshot     // the raw joined commits + events
	Groups   Groupings    // HEAD / CIPassed / InFlight / Deployed buckets
	Weekly   []WeekStat   // DORA throughput per ISO week
	Header   HeaderStatus // ci/deploy summary for the top header line
	// Stale signals to Renderers that this View was emitted from a
	// stale-while-revalidate cache and a fresh fetch is still in flight.
	// Renderers can decide whether and how to indicate that visually
	// (TUI shows a "refreshing…" hint; plain mode currently ignores).
	// Set by CachedLens on its initial cache-derived emission; bare
	// Lens always leaves it false.
	Stale bool
}

// HeaderStatus carries the resolved CI / deploy status for the top
// header line: "" / "passed" / "failed". "started" and "skipped" events
// are intentionally collapsed away so header badges hold their colour
// through transient retries (see CurrentStageStatus).
type HeaderStatus struct {
	CI     string
	Deploy string
}

// DeriveView builds View from Snapshot. Pure function. Used by the Lens
// to produce streamed views and by the demo binary to derive a View from
// a hand-built Snapshot without going through Source adapters.
func DeriveView(snap Snapshot) View {
	return View{
		Snapshot: snap,
		Groups:   GroupCommits(snap.Commits),
		Weekly:   WeeklyStats(snap),
		Header:   buildHeaderStatus(snap.Commits),
	}
}

func buildHeaderStatus(commits []CommitView) HeaderStatus {
	return HeaderStatus{
		CI:     CurrentStageStatus(commits, "ci"),
		Deploy: CurrentStageStatus(commits, "deploy"),
	}
}

// BuildSnapshot joins commits with their events by SHA. Every Source
// adapter calls this to assemble its emitted Snapshot — the join is
// identical regardless of where commits and events came from. Pure.
//
// Commits are passed newest-first; the resulting Snapshot preserves that
// order. Events keyed by SHAs not in the commits slice are silently
// dropped (those commits are outside the loaded window).
func BuildSnapshot(commits []Commit, events Events) Snapshot {
	joined := make([]CommitView, len(commits))
	for i, c := range commits {
		joined[i] = CommitView{
			SHA:     c.SHA,
			Subject: c.Subject,
			Author:  c.Author,
			Time:    c.Time,
			Events:  events[c.SHA],
		}
	}
	return Snapshot{Commits: joined}
}
