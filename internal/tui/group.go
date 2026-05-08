package tui

import (
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

// Groupings buckets a snapshot's commits into the trunk-based-development
// lifecycle stages. Within each bucket commits keep their original git-log
// order (newest-first); only the SECTION a commit falls in changes as
// supersedence moves the build and deploy lines.
type Groupings struct {
	NeedsCI    []watcher.CommitView
	NextDeploy []watcher.CommitView
	// Deploying is true when a NextDeploy commit has deploy:started without
	// a corresponding completion — i.e. a deploy is currently in flight.
	Deploying bool
	Deployed  []watcher.CommitView

	// indices into the original commits slice; -1 if absent.
	buildLine  int
	deployLine int

	// deployedAt[i] is the deploy time at which commit i entered production —
	// either its own deploy:passed, or the deploy:passed of a newer commit
	// (fix-forward). Zero when commit i has not yet been deployed.
	deployedAt []time.Time
}

// GroupCommits classifies commits (newest-first) into lifecycle groups using
// fix-forward semantics:
//   - a commit is "deployed" the moment any newer commit has deployed;
//   - a failed build is only globally red if no newer commit has built green.
//
// If deploys exist without any explicit passing build event, the deploy line
// also acts as the build line — successful deployment implies a successful
// build, even if the build event itself wasn't reported.
func GroupCommits(commits []watcher.CommitView) Groupings {
	g := Groupings{
		buildLine:  -1,
		deployLine: -1,
		deployedAt: make([]time.Time, len(commits)),
	}

	// Walk newest → oldest. Each commit's deployedAt carries the time at
	// which it FIRST entered production:
	//   - if it has its own deploy:passed, that's the freeze point;
	//   - otherwise it's fix-forwarded by the OLDEST newer commit that has a
	//     deploy:passed (i.e. the most recently observed one as we walk
	//     newest → oldest).
	// This preserves "as soon as any later commit has been deployed" — a
	// commit's freeze time never moves forward just because something newer
	// deploys later.
	var lastSeenDeploy time.Time
	for i, c := range commits {
		var ownDeploy time.Time
		for _, e := range c.Events {
			if e.Stage == "deploy" && e.Status == "passed" && e.Time.After(ownDeploy) {
				ownDeploy = e.Time
			}
		}
		if !ownDeploy.IsZero() {
			g.deployedAt[i] = ownDeploy
			lastSeenDeploy = ownDeploy
		} else if !lastSeenDeploy.IsZero() {
			g.deployedAt[i] = lastSeenDeploy
		}
		latest := latestPerStage(c.Events)
		if g.buildLine == -1 && latest["build"] == "passed" {
			g.buildLine = i
		}
		if g.deployLine == -1 && latest["deploy"] == "passed" {
			g.deployLine = i
		}
	}

	effectiveBuildLine := g.buildLine
	if effectiveBuildLine == -1 && g.deployLine != -1 {
		effectiveBuildLine = g.deployLine
	}

	for i, c := range commits {
		switch {
		case g.deployLine != -1 && i >= g.deployLine:
			g.Deployed = append(g.Deployed, c)
		case effectiveBuildLine != -1 && i >= effectiveBuildLine:
			g.NextDeploy = append(g.NextDeploy, c)
			if latestPerStage(c.Events)["deploy"] == "started" {
				g.Deploying = true
			}
		default:
			g.NeedsCI = append(g.NeedsCI, c)
		}
	}
	return g
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
// has been superseded by a newer commit's success on that same stage. The
// rendering layer dims stale stage icons so they read as settled history.
func (g Groupings) IsStaleStage(index int, stage string) bool {
	switch stage {
	case "build":
		return g.buildLine != -1 && index > g.buildLine
	case "deploy":
		return g.deployLine != -1 && index > g.deployLine
	}
	return false
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
