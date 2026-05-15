package tui

import (
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

// Groupings buckets a snapshot's commits into the trunk-based-development
// lifecycle stages. Within each bucket commits keep their original git-log
// order (newest-first); only the SECTION (or batch) a commit falls in
// changes as supersedence moves the build and deploy lines.
type Groupings struct {
	// Head: landed on main but not yet passed CI.
	Head []watcher.CommitView

	// CIPassed: built green, no in-flight deploy associated yet. Idle commits
	// that are queued for the next deploy.
	CIPassed []watcher.CommitView

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
}

// DeployBatch is a subgroup within Deployed (or InFlight): one deploy
// attempt's commits plus the status and time of the deploy event that
// anchors it.
type DeployBatch struct {
	Status  string // "started" | "passed" | "failed"
	Time    time.Time
	Commits []watcher.CommitView
}

// GroupCommits classifies commits (newest-first) into lifecycle groups. The
// section boundary between CI Passed and Deployed is the deploy:passed line —
// in-flight (started/failed) batches stay above the line as InFlight, sitting
// at the bottom of CI Passed in the rendered TUI.
func GroupCommits(commits []watcher.CommitView) Groupings {
	g := Groupings{
		ciLine:           -1,
		deployPassedLine: -1,
		deployedAt:       make([]time.Time, len(commits)),
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
	var ciRange []watcher.CommitView
	var deployedRange []watcher.CommitView
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
	var inFlightCommits []watcher.CommitView
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
func computeBatches(commits []watcher.CommitView) []DeployBatch {
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
			Commits: []watcher.CommitView{c},
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

// LeadTime returns the elapsed time from the commit's authoring to either
// the deploy that pushed it to production (frozen=true) or the current moment
// (frozen=false). Returns ok=false when the commit time is unknown.
func (g Groupings) LeadTime(index int, commitTime, now time.Time) (time.Duration, bool, bool) {
	if commitTime.IsZero() {
		return 0, false, false
	}
	if index >= 0 && index < len(g.deployedAt) && !g.deployedAt[index].IsZero() {
		return g.deployedAt[index].Sub(commitTime), true, true
	}
	return now.Sub(commitTime), false, true
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
