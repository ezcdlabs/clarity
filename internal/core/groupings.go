package core

import (
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
)

// Groupings buckets a snapshot's commits into the trunk-based-development
// lifecycle stages. Within each bucket commits keep their original git-log
// order (newest-first); only the SECTION (or batch) a commit falls in
// changes as supersedence moves the build and deploy lines.
type Groupings struct {
	// Head: landed on main but not yet passed CI.
	Head []CommitView

	// CIPassed: built green, no in-flight deploy associated yet. Idle commits
	// that are queued for the next deploy.
	CIPassed []CommitView

	// InFlight sits visually at the bottom of the CI Passed section: any
	// deploy attempt that hasn't successfully landed yet (status "started"
	// or stuck-failed). Failed batches with a newer non-failed attempt are
	// merged into that newer batch.
	InFlight []DeployBatch

	// Deployed: only completed deploys (deploy:passed). Each batch is one
	// successful deploy event, with the commits it pushed to production.
	Deployed []DeployBatch

	// indices into the original commits slice; -1 if absent.
	ciLine           int // newest commit with ci:passed
	deployPassedLine int // newest commit with deploy:passed

	// deployedAt[i] is the deploy time at which commit i first reached
	// production — its own deploy:passed time, or the oldest newer commit's
	// deploy:passed time if fix-forwarded. Zero when commit i has not yet
	// been deployed.
	deployedAt []time.Time

	// leadStart[i] is the instant commit i's lead time is measured from,
	// per the configured LeadTimeMode. Zero means the commit has no lead
	// time: it renders without one and contributes nothing to the weekly
	// average. Precomputed here so the per-row display and the weekly
	// aggregate cannot disagree about which commits count.
	leadStart []time.Time
}

// DeployBatch is a subgroup within Deployed (or InFlight): one deploy
// attempt's commits plus the status and time of the deploy event that
// anchors it.
type DeployBatch struct {
	Status  string // "started" | "passed" | "failed"
	Time    time.Time
	Commits []CommitView
}

// GroupCommits classifies commits (newest-first) into lifecycle groups. The
// section boundary between CI Passed and Deployed is the deploy:passed line —
// in-flight (started/failed) batches stay above the line as InFlight, sitting
// at the bottom of CI Passed in the rendered TUI.
// GroupCommits groups under DefaultLeadTimeMode. Grouping itself doesn't
// depend on the lead time mode — only which commits carry a lead time does —
// so callers that don't configure one (the demo binary, most tests) use this.
func GroupCommits(commits []CommitView) Groupings {
	return GroupCommitsMode(commits, DefaultLeadTimeMode)
}

// GroupCommitsMode classifies commits (newest-first) into lifecycle groups,
// computing lead time starts under the given mode.
func GroupCommitsMode(commits []CommitView, mode LeadTimeMode) Groupings {
	g := Groupings{
		ciLine:           -1,
		deployPassedLine: -1,
		deployedAt:       make([]time.Time, len(commits)),
		leadStart:        leadStarts(commits, mode),
	}

	// Pass 1 — compute the lines and per-commit deployedAt.
	var lastSeenDeployPassed time.Time
	for i, c := range commits {
		var ownDeployPassed time.Time
		for _, e := range c.Events {
			if e.Stage == "deploy" && e.Status == "passed" && e.Time.After(ownDeployPassed) {
				ownDeployPassed = e.Time
			}
		}
		if !ownDeployPassed.IsZero() {
			g.deployedAt[i] = ownDeployPassed
			lastSeenDeployPassed = ownDeployPassed
		} else if !lastSeenDeployPassed.IsZero() {
			g.deployedAt[i] = lastSeenDeployPassed
		}
		latest := latestPerStage(c.Events)
		if g.ciLine == -1 && latest["ci"] == "passed" {
			g.ciLine = i
		}
		if g.deployPassedLine == -1 && latest["deploy"] == "passed" {
			g.deployPassedLine = i
		}
	}

	// effectiveCILine: a passing deploy implies a passing build, even if no
	// CI event was reported. Treat the deploy line as the CI line in that case.
	effectiveCILine := g.ciLine
	if effectiveCILine == -1 && g.deployPassedLine != -1 {
		effectiveCILine = g.deployPassedLine
	}

	// Pass 2 — classify into Head / CI-Passed range / Deployed range.
	var ciRange []CommitView
	var deployedRange []CommitView
	for i, c := range commits {
		switch {
		case g.deployPassedLine != -1 && i >= g.deployPassedLine:
			deployedRange = append(deployedRange, c)
		case effectiveCILine != -1 && i >= effectiveCILine:
			ciRange = append(ciRange, c)
		default:
			g.Head = append(g.Head, c)
		}
	}

	// Pass 3 — within the CI-Passed range, split bare/idle commits at the top
	// (newer than any deploy event) from in-flight batches at the bottom.
	// Walking newest → oldest: until we encounter the first deploy event,
	// commits are idle; from there on they belong to an in-flight batch.
	var seenDeploy bool
	var inFlightCommits []CommitView
	for _, c := range ciRange {
		if !seenDeploy && !hasDeployEvent(c.Events) {
			g.CIPassed = append(g.CIPassed, c)
			continue
		}
		seenDeploy = true
		inFlightCommits = append(inFlightCommits, c)
	}
	g.InFlight = computeBatches(inFlightCommits)

	// Pass 4 — Deployed batches (only deploy:passed anchors; failed entries
	// older than the line are fix-forwarded into newer passed batches).
	g.Deployed = computeBatches(deployedRange)

	return g
}

// computeBatches walks the given range newest → oldest. A commit with a
// non-failed deploy event opens a new batch; a commit with no deploy event
// joins the current (newer) batch; a commit with a failed deploy joins the
// current batch (fix-forwarded) when the current batch is non-failed,
// otherwise it starts its own visible failed batch.
func computeBatches(commits []CommitView) []DeployBatch {
	var batches []DeployBatch
	var current *DeployBatch
	for _, c := range commits {
		latestEvent, hasDeploy := latestDeployEvent(c.Events)
		if !hasDeploy {
			if current != nil {
				current.Commits = append(current.Commits, c)
			}
			continue
		}
		// A failed deploy fix-forwards into a newer non-failed batch (started
		// or passed). Two failures in a row are independent failures — they
		// stay as separate visible groups.
		if latestEvent.Status == "failed" && current != nil && current.Status != "failed" {
			current.Commits = append(current.Commits, c)
			continue
		}
		if current != nil {
			batches = append(batches, *current)
		}
		current = &DeployBatch{
			Status:  latestEvent.Status,
			Time:    latestEvent.Time,
			Commits: []CommitView{c},
		}
	}
	if current != nil {
		batches = append(batches, *current)
	}
	return batches
}

// DeployedAtIndex returns the deploy time the commit at index will appear
// to have under fix-forward semantics: its own deploy:passed time, or — if
// it has none — the time of the newest newer commit that did get a passed
// deploy. Zero when neither applies (commit is newer than every passed
// deploy in the snapshot). Exposed so callers like WeeklyStats can bucket
// commits by the same deploy-week the per-row lead time was computed from.
func (g Groupings) DeployedAtIndex(index int) time.Time {
	if index < 0 || index >= len(g.deployedAt) {
		return time.Time{}
	}
	return g.deployedAt[index]
}

// LeadTime returns the elapsed time from where the configured LeadTimeMode
// starts measuring commit[index] to either the deploy that pushed it to
// production (frozen=true) or the current moment (frozen=false).
//
// ok=false means the commit has no lead time and should render without one:
// its start is unknown, or the mode excludes it, or the frozen interval would
// not be positive.
//
// That last case is dropped rather than clamped, because every way of
// producing it is a data artefact rather than a fast delivery. A rebased or
// amended commit can carry an author date later than the deploy that shipped
// it. A commit can inherit a deploy from a newer commit that landed earlier.
// And in LeadPipeline a team reporting only terminal events has its start
// land on the deploy itself, making the interval exactly zero — the mode has
// nothing to measure, which is not the same as measuring nothing. Letting any
// of them through would drag the average below what the pipeline actually
// achieves.
//
// The live (unfrozen) branch is deliberately laxer: a timer that has just
// started legitimately reads zero, and will tick.
func (g Groupings) LeadTime(index int, now time.Time) (time.Duration, bool, bool) {
	if index < 0 || index >= len(g.leadStart) {
		return 0, false, false
	}
	start := g.leadStart[index]
	if start.IsZero() {
		return 0, false, false
	}
	if deployedAt := g.deployedAt[index]; !deployedAt.IsZero() {
		if !deployedAt.After(start) {
			return 0, false, false
		}
		return deployedAt.Sub(start), true, true
	}
	if now.Before(start) {
		return 0, false, false
	}
	return now.Sub(start), false, true
}

// leadStartAt returns the lead time start for commit index, or the zero time
// when it has none. Used by WeeklyStats so the aggregate counts exactly the
// commits the per-row renderer shows a lead time for.
func (g Groupings) leadStartAt(index int) time.Time {
	if index < 0 || index >= len(g.leadStart) {
		return time.Time{}
	}
	return g.leadStart[index]
}

// IsStaleStage reports whether commit[index]'s status for the given stage
// has been superseded by a newer commit's success on that same stage.
func (g Groupings) IsStaleStage(index int, stage string) bool {
	switch stage {
	case "ci":
		return g.ciLine != -1 && index > g.ciLine
	case "deploy":
		return g.deployPassedLine != -1 && index > g.deployPassedLine
	}
	return false
}

// totalDeployed sums all commits across Deployed batches.
func (g Groupings) totalDeployed() int {
	n := 0
	for _, b := range g.Deployed {
		n += len(b.Commits)
	}
	return n
}

// totalInFlight sums all commits across in-flight batches.
func (g Groupings) totalInFlight() int {
	n := 0
	for _, b := range g.InFlight {
		n += len(b.Commits)
	}
	return n
}

// hasDeployEvent reports whether the commit has any deploy event of any kind.
func hasDeployEvent(events []clarityrefs.Event) bool {
	for _, e := range events {
		if e.Stage == "deploy" {
			return true
		}
	}
	return false
}

// latestDeployEvent returns the newest deploy event on the commit, or false
// when there are none.
func latestDeployEvent(events []clarityrefs.Event) (clarityrefs.Event, bool) {
	var latest clarityrefs.Event
	found := false
	for _, e := range events {
		if e.Stage != "deploy" {
			continue
		}
		if !found || e.Time.After(latest.Time) {
			latest = e
			found = true
		}
	}
	return latest, found
}

func latestPerStage(events []clarityrefs.Event) map[string]string {
	out := map[string]string{}
	latestTime := map[string]time.Time{}
	for _, e := range events {
		if t, ok := latestTime[e.Stage]; !ok || e.Time.After(t) {
			latestTime[e.Stage] = e.Time
			out[e.Stage] = e.Status
		}
	}
	return out
}
