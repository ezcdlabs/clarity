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
	// CIPassed: built green, no deploy event of any kind yet.
	CIPassed []watcher.CommitView
	// Deployed is split into batches — one per distinct deploy attempt.
	// A batch with status "started" is currently deploying; "passed" is a
	// completed deploy; "failed" is a deploy that failed and has no newer
	// attempt to fix-forward it (otherwise it would have been merged into
	// the newer batch).
	Deployed []DeployBatch

	// indices into the original commits slice; -1 if absent.
	buildLine          int // newest commit with build:passed
	deployPassedLine   int // newest commit with deploy:passed (used for stale-stage check)
	deployBoundaryLine int // newest commit with ANY deploy event (used for section boundary)

	// deployedAt[i] is the deploy time at which commit i first reached
	// production — its own deploy:passed time, or the oldest newer commit's
	// deploy:passed time if fix-forwarded. Zero when commit i has not yet
	// been deployed.
	deployedAt []time.Time
}

// DeployBatch is a subgroup within Deployed: one deploy attempt's commits
// plus the status and time of the deploy event that anchors it.
type DeployBatch struct {
	Status  string // "started" | "passed" | "failed"
	Time    time.Time
	Commits []watcher.CommitView
}

// GroupCommits classifies commits (newest-first) into lifecycle groups. The
// Deployed section subgroups its commits into batches per deploy attempt;
// failed deploys with a newer attempt are merged into that newer batch.
func GroupCommits(commits []watcher.CommitView) Groupings {
	g := Groupings{
		buildLine:          -1,
		deployPassedLine:   -1,
		deployBoundaryLine: -1,
		deployedAt:         make([]time.Time, len(commits)),
	}

	// Pass 1 — compute the three lines and per-commit deployedAt.
	var lastSeenDeployPassed time.Time
	for i, c := range commits {
		var ownDeployPassed time.Time
		var hasAnyDeploy bool
		for _, e := range c.Events {
			if e.Stage != "deploy" {
				continue
			}
			hasAnyDeploy = true
			if e.Status == "passed" && e.Time.After(ownDeployPassed) {
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
		if g.buildLine == -1 && latest["build"] == "passed" {
			g.buildLine = i
		}
		if g.deployPassedLine == -1 && latest["deploy"] == "passed" {
			g.deployPassedLine = i
		}
		if g.deployBoundaryLine == -1 && hasAnyDeploy {
			g.deployBoundaryLine = i
		}
	}

	// effectiveBuildLine: a deploy event implies a passing build, even if no
	// build event was reported. Treat the deploy boundary as the build line
	// when no real build:passed exists.
	effectiveBuildLine := g.buildLine
	if effectiveBuildLine == -1 && g.deployBoundaryLine != -1 {
		effectiveBuildLine = g.deployBoundaryLine
	}

	// Pass 2 — classify into Head / CIPassed / a "deployed range" that
	// will be subdivided into batches.
	var deployedRange []watcher.CommitView
	for i, c := range commits {
		switch {
		case g.deployBoundaryLine != -1 && i >= g.deployBoundaryLine:
			deployedRange = append(deployedRange, c)
		case effectiveBuildLine != -1 && i >= effectiveBuildLine:
			g.CIPassed = append(g.CIPassed, c)
		default:
			g.Head = append(g.Head, c)
		}
	}

	// Pass 3 — split deployedRange into batches with the failed-merge rule.
	g.Deployed = computeBatches(deployedRange)

	return g
}

// computeBatches walks the Deployed range newest → oldest. A commit with a
// non-failed deploy event opens a new batch; a commit with no deploy event
// joins the current (newer) batch; a commit with a failed deploy joins the
// current batch (i.e. is fix-forwarded), unless there is no newer batch yet,
// in which case it starts its own visible "failed" batch.
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
	case "build":
		return g.buildLine != -1 && index > g.buildLine
	case "deploy":
		return g.deployPassedLine != -1 && index > g.deployPassedLine
	}
	return false
}

// totalDeployed sums all commits across Deployed batches. Useful for
// summary checks where batch shape doesn't matter.
func (g Groupings) totalDeployed() int {
	n := 0
	for _, b := range g.Deployed {
		n += len(b.Commits)
	}
	return n
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
