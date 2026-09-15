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
	// Flows is one derived view per deploy flow, in display order. Always
	// non-empty: a repo with no targets has exactly one. Every flow is
	// derived up front so a Renderer can switch between them as local UI
	// state, with no round trip to the Lens and no re-derivation in the
	// rendering layer.
	Flows []FlowView
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

// DeriveView builds View from Snapshot under the given LeadTimeMode. Pure
// function. Used by the Lens to produce streamed views and by the demo binary
// to derive a View from a hand-built Snapshot without going through Source
// adapters.
//
// The mode decides which commits carry a lead time and what that time is
// measured from. flows are the declared deploy flows from .ezcd.json; pass nil
// to have them discovered from the events instead, which is the zero-setup
// path.
//
// View.Groups / View.Weekly stay whole-repo and ignore targets, so a caller
// that predates flows keeps the numbers it always had.
func DeriveView(snap Snapshot, mode LeadTimeMode, flows []Flow) View {
	groups := GroupCommitsMode(snap.Commits, mode)
	weekly := WeeklyStatsMode(snap, mode)

	resolved := ResolveFlows(snap.Commits, flows)
	for i := range resolved {
		f := resolved[i]
		// The overwhelmingly common case is one flow claiming everything, and
		// filtering then produces exactly the commits we already grouped.
		// Deriving it twice would double the work on every snapshot for every
		// repo that has no targets at all, so reuse it.
		// ResolveFlows gives every unclaimed target a flow of its own, so a
		// single resolved flow is one that already claims every deploy
		// present — filtering would return exactly these commits. Pinned by
		// TestResolveFlows_SingleFlowClaimsEveryDeploy, because the
		// short-circuit is only safe while that stays true.
		//
		// Candidacy breaks it independently of flow count: a lone flow can
		// still have commits that ship nothing it deploys, and those must
		// lose their lead time. Only skip the work when nothing has been
		// reported, which is every repo that doesn't use the feature.
		if len(resolved) == 1 && !hasCandidacy(snap.Commits) {
			resolved[i].Groups = groups
			resolved[i].Weekly = weekly
			resolved[i].Deploy = CurrentStageStatus(snap.Commits, "deploy")
			continue
		}

		scoped := commitsForFlow(snap.Commits, f.Flow)
		flowSnap := snap
		flowSnap.Commits = scoped

		resolved[i].Groups = GroupCommitsForFlow(scoped, mode, f.Flow)
		resolved[i].Weekly = WeeklyStatsForFlow(flowSnap, mode, f.Flow)
		resolved[i].Deploy = CurrentStageStatus(scoped, "deploy")
	}

	return View{
		Snapshot: snap,
		Groups:   groups,
		Weekly:   weekly,
		Header:   buildHeaderStatus(snap.Commits),
		Flows:    resolved,
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
	return BuildSnapshotWithScope(commits, events, nil)
}

// BuildSnapshotWithScope is BuildSnapshot including candidacy records. A
// Source that can read them passes them here; one that can't passes nil, which
// means every commit is a candidate for every flow.
func BuildSnapshotWithScope(commits []Commit, events Events, scope ScopeBySHA) Snapshot {
	joined := make([]CommitView, len(commits))
	for i, c := range commits {
		joined[i] = CommitView{
			SHA:     c.SHA,
			Subject: c.Subject,
			Author:  c.Author,
			Time:    c.Time,
			Events:  events[c.SHA],
			Scope:   scope[c.SHA],
		}
	}
	return Snapshot{Commits: joined}
}

// hasCandidacy reports whether any commit carries a candidacy record, which is
// what decides whether the single-flow short-circuit is safe.
func hasCandidacy(commits []CommitView) bool {
	for _, c := range commits {
		if len(c.Scope) > 0 {
			return true
		}
	}
	return false
}
