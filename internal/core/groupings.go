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

	// Deployed: only completed deliveries (deploy:passed, or deploy:noop —
	// the pipeline found production already matching). Each batch is one
	// terminal deploy event, with the commits it carried to production.
	Deployed []DeployBatch

	// indices into the original commits slice; -1 if absent.
	ciLine           int // newest commit with ci:passed
	deployPassedLine int // newest commit with a completed deploy (passed or noop)

	// deployedAt[i] is the deploy time at which commit i first reached
	// production — its own completed deploy's time (passed, or noop: the
	// pipeline verified production already matched), or the oldest newer
	// commit's if fix-forwarded. Zero when commit i has not yet been
	// deployed.
	deployedAt []time.Time

	// leadStart[i] is the instant commit i's lead time is measured from,
	// per the configured LeadTimeMode. Zero means the commit has no lead
	// time: it renders without one and contributes nothing to the weekly
	// average. Precomputed here so the per-row display and the weekly
	// aggregate cannot disagree about which commits count.
	leadStart []time.Time

	// unaffected marks commits resolved unaffected for the flow this grouping
	// was derived for (nil when derived without a flow). Per-commit truth —
	// its own candidacy record — unlike the per-run OR that decides whether a
	// lead-time sample survives. See Unaffected.
	unaffected []bool
}

// DeployBatch is a subgroup within Deployed (or InFlight): one deploy
// attempt's commits plus the status and time of the deploy event that
// anchors it.
type DeployBatch struct {
	Status  string // "started" | "passed" | "failed" | "noop"
	Time    time.Time
	Commits []CommitView
}

// deployCompleted reports whether a deploy status is a completed delivery:
// "passed" shipped the commit's tree, "noop" found production already holding
// it. The distinction matters where deploys are counted — a noop is not a
// deploy — but not where delivery is the question: both stop the lead-time
// clock and both put their batch in the Deployed section.
func deployCompleted(status string) bool {
	return status == "passed" || status == "noop"
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

	// Pass 1 — compute the lines and per-commit deployedAt. A noop completes
	// delivery the way a passed deploy does — the pipeline verified production
	// already held this commit's tree — so both move the deploy line and both
	// stamp deployedAt, which is what freezes the lead-time clock.
	var lastSeenDeployDone time.Time
	for i, c := range commits {
		var ownDeployDone time.Time
		for _, e := range c.Events {
			if e.Stage == "deploy" && deployCompleted(e.Status) && e.Time.After(ownDeployDone) {
				ownDeployDone = e.Time
			}
		}
		if !ownDeployDone.IsZero() {
			g.deployedAt[i] = ownDeployDone
			lastSeenDeployDone = ownDeployDone
		} else if !lastSeenDeployDone.IsZero() {
			g.deployedAt[i] = lastSeenDeployDone
		}
		latest := latestPerStage(c.Events)
		if g.ciLine == -1 && latest["ci"] == "passed" {
			g.ciLine = i
		}
		if g.deployPassedLine == -1 && deployCompleted(latest["deploy"]) {
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

// LiveBatchIndex returns the index into Deployed of the batch that is the
// present state of production — the newest completed batch — or -1 when
// nothing has completed.
//
// "Live" is a state claim ("this is production"), not an event claim ("this
// deploy shipped bytes"), so a noop batch on top holds it: the pipeline just
// verified production equals that batch's tree, which is the freshest
// possible statement of the present state. Leaving the marker on the passed
// batch below would put the noop's commits above the live line, reading as
// "not live yet" — and they are live, trivially.
//
// Decided here rather than in each renderer because three independent copies
// of "which batch is the present" would be three chances to disagree, and the
// disagreement would surface as two screens calling different batches live.
//
// Today this is always 0 when Deployed is non-empty — the deploy line is, by
// construction, a commit whose latest deploy completed, so the newest batch's
// anchor is never failed — but the walk stays, so the rule survives a future
// grouping change rather than silently becoming "the first batch, whatever it
// is".
func (g Groupings) LiveBatchIndex() int {
	for i, b := range g.Deployed {
		if deployCompleted(b.Status) {
			return i
		}
	}
	return -1
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
// hasDeployEvent and latestDeployEvent both ignore "skipped": the status is
// retired from the write path, but old refs still carry it, and a skip ships
// nothing — letting it anchor a batch rendered an unlabeled group whose
// commit read as waiting to ship forever. A commit whose only deploy events
// are skips groups exactly like a commit with none. ("This commit does not
// affect this target" is candidacy, not an event — see report unaffected.)
func hasDeployEvent(events []clarityrefs.Event) bool {
	for _, e := range events {
		if e.Stage == "deploy" && e.Status != "skipped" {
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
		if e.Stage != "deploy" || e.Status == "skipped" {
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

// GroupCommitsForFlow is GroupCommitsMode with candidacy applied: a commit
// that contains no change this flow ships keeps its place in the log but
// carries no lead time.
//
// Dropping the lead time rather than the commit is deliberate, and it reuses
// the mechanism `reported` and `pipeline` already use for a commit with no
// events: a zero leadStart. The intent is to stop a commit skewing the number,
// not to hide that it shipped — a web-only commit really is in the iOS build,
// it just says nothing about how fast iOS changes reach users.
//
// How a record applies depends on what a sample stands for in the mode. Under
// `all`, every commit is its own sample, so its own record decides. Under
// `pipeline` and `reported`, one event-bearing commit holds the sample for a
// whole pipeline run — CI ran on the pushed tip, and the event-less commits
// swept below it went through CI in that same run — so the tip's record alone
// cannot decide: the tip of a push can be unaffected while an older commit in
// the same push holds the change this flow ships, and that run still
// delivered it. The sample survives if ANY commit the run carried is a
// candidate. Silence means candidate here as everywhere, so a swept commit
// with no record keeps its run counting — one partially-reported push can
// only err towards counting, the same direction the candidacy tie-break errs.
func GroupCommitsForFlow(commits []CommitView, mode LeadTimeMode, f Flow) Groupings {
	g := GroupCommitsMode(commits, mode)
	g.unaffected = make([]bool, len(commits))
	for i, c := range commits {
		g.unaffected[i] = !IsCandidate(c.Scope, f)
	}
	if mode != LeadPipeline && mode != LeadReported {
		for i := range commits {
			if g.unaffected[i] {
				g.leadStart[i] = time.Time{}
			}
		}
		return g
	}
	// One sample per run: commits are newest-first, so a run is an
	// event-bearing commit plus the event-less commits that follow it, ending
	// before the next event-bearing commit (the previous run's tip).
	for i, c := range commits {
		if len(c.Events) == 0 {
			continue
		}
		candidate := !g.unaffected[i]
		for j := i + 1; j < len(commits) && len(commits[j].Events) == 0 && !candidate; j++ {
			candidate = !g.unaffected[j]
		}
		if !candidate {
			g.leadStart[i] = time.Time{}
		}
	}
	return g
}

// Unaffected reports whether the commit at index was resolved unaffected for
// the flow this grouping was derived for — its own record, not the per-run
// OR that decides lead-time survival, because a marker marks the commit
// itself. False for a grouping derived without a flow (GroupCommits /
// GroupCommitsMode): with no flow there is no target to be unaffected for.
// Renderers need this because a commit with no measurement and a commit
// resolved unaffected both have no lead time, and blank and the ∅ marker
// are different answers.
func (g Groupings) Unaffected(index int) bool {
	if index < 0 || index >= len(g.unaffected) {
		return false
	}
	return g.unaffected[index]
}
