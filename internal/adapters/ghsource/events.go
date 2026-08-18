package ghsource

import (
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/config"
)

// zeroTime is the sentinel "not set" timestamp the per-run accumulators
// start with; using a named value keeps the intent clearer than
// scattered `time.Time{}` literals.
var zeroTime time.Time

// DeriveEvents turns one stage's collection of GH Actions runs into
// clarityrefs events keyed by commit SHA. Pure function — no I/O, no
// time-of-day dependencies. The mapping between GH job statuses and
// clarity event statuses is the load-bearing logic:
//
//   - Started signal: min(started_at) across jobs whose names are in
//     jobs.Started().
//   - Terminal signal: max(completed_at) across jobs whose names are in
//     jobs.Completed(). Conclusion aggregation:
//     any of {failure, timed_out, cancelled} → "failed"
//     all "success"                          → "passed"
//     any still-running (Conclusion == "")    → no terminal event
//     (the run hasn't decided)
//     anything else (skipped, neutral, …)    → no terminal event
//
// Per-run events are appended to the per-SHA slice, so the same SHA
// retried on a new run produces additional events in chronological
// order — which is exactly what clarity's lens consumes downstream.
func DeriveEvents(stage string, runs []Run, jobs config.JobSet) map[string][]clarityrefs.Event {
	out := map[string][]clarityrefs.Event{}
	startedSet := nameSet(jobs.Started())
	completedSet := nameSet(jobs.Completed())

	for _, run := range runs {
		// Started signal: min(started_at) across started-set jobs in this run.
		var startedAt = zeroTime
		for _, j := range run.Jobs {
			if !startedSet[j.Name] {
				continue
			}
			if j.StartedAt.IsZero() {
				continue
			}
			if startedAt.IsZero() || j.StartedAt.Before(startedAt) {
				startedAt = j.StartedAt
			}
		}

		// Terminal signal: max(completed_at) across completed-set jobs +
		// conclusion aggregation.
		terminalStatus, terminalTime := aggregateTerminal(run.Jobs, completedSet)

		// Nothing matched at all — don't manufacture entries for this SHA.
		if startedAt.IsZero() && terminalStatus == "" {
			continue
		}
		if !startedAt.IsZero() {
			out[run.HeadSHA] = append(out[run.HeadSHA], clarityrefs.Event{
				Stage:  stage,
				Status: "started",
				Time:   startedAt,
			})
		}
		if terminalStatus != "" {
			out[run.HeadSHA] = append(out[run.HeadSHA], clarityrefs.Event{
				Stage:  stage,
				Status: terminalStatus,
				Time:   terminalTime,
			})
		}
	}
	return out
}

// aggregateTerminal returns the stage's terminal verdict for one run.
// Empty status = no aggregatable verdict yet (still running, or no
// completed-set jobs matched).
func aggregateTerminal(jobs []Job, completedSet map[string]bool) (string, time.Time) {
	var (
		anyMatched bool
		anyFailed  bool
		allPassed  = true
		latest     time.Time
	)
	for _, j := range jobs {
		if !completedSet[j.Name] {
			continue
		}
		anyMatched = true
		switch j.Conclusion {
		case "":
			// Still running — no verdict for this run.
			return "", zeroTime
		case "success":
			// Doesn't change anyFailed.
		case "failure", "timed_out", "cancelled":
			anyFailed = true
			allPassed = false
		default:
			// neutral / skipped / action_required — no terminal verdict
			// (don't mark passed; don't claim failed). The run as a
			// whole has no aggregatable status for this stage.
			allPassed = false
		}
		if j.CompletedAt.After(latest) {
			latest = j.CompletedAt
		}
	}
	if !anyMatched {
		return "", zeroTime
	}
	if anyFailed {
		return "failed", latest
	}
	if allPassed {
		return "passed", latest
	}
	// Matched, but the verdict is neither passed nor failed.
	return "", zeroTime
}

// nameSet turns a slice into a O(1) lookup set.
func nameSet(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out
}
