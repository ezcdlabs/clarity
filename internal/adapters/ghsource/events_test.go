package ghsource_test

import (
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/adapters/ghsource"
	"github.com/ezcdlabs/clarity/internal/config"
)

// stageJobs is a tiny helper that builds the shared-array JobSet shape
// from a list of names — saves a per-test JSON round-trip.
func stageJobs(names ...string) config.JobSet {
	var js config.JobSet
	// JobSet's exported behaviour is read-only; the only way to set
	// both halves is via UnmarshalJSON. We construct a tiny JSON
	// document for that.
	data := []byte(`[`)
	for i, n := range names {
		if i > 0 {
			data = append(data, ',')
		}
		data = append(data, '"')
		data = append(data, []byte(n)...)
		data = append(data, '"')
	}
	data = append(data, ']')
	if err := js.UnmarshalJSON(data); err != nil {
		panic(err)
	}
	return js
}

// TestDeriveEvents_AllJobsPass: every matching completed-job has
// conclusion=success → one passed event at max(completed_at). The
// derivation MUST NOT emit a separate failed/skipped event when a
// later run conclusion is the actual signal.
func TestDeriveEvents_AllJobsPass(t *testing.T) {
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute)
	runs := []ghsource.Run{{
		ID: 1, HeadSHA: "abc", Workflow: "CI",
		Jobs: []ghsource.Job{
			{Name: "Test", Status: "completed", Conclusion: "success", StartedAt: start, CompletedAt: end},
			{Name: "Lint", Status: "completed", Conclusion: "success", StartedAt: start, CompletedAt: end.Add(time.Minute)},
		},
	}}

	got := ghsource.DeriveEvents("ci", runs, stageJobs("Test", "Lint"))
	if len(got["abc"]) != 2 {
		t.Fatalf("expected 2 events for abc (started + passed), got %d: %+v", len(got["abc"]), got["abc"])
	}
	if !hasEvent(got["abc"], "ci", "started") {
		t.Errorf("expected ci:started, got %+v", got["abc"])
	}
	terminal := findEvent(got["abc"], "ci", "passed")
	if terminal == nil {
		t.Fatalf("expected ci:passed, got %+v", got["abc"])
	}
	if !terminal.Time.Equal(end.Add(time.Minute)) {
		t.Errorf("expected passed at max(completed_at)=%v, got %v", end.Add(time.Minute), terminal.Time)
	}
}

// TestDeriveEvents_AnyJobFails: failure-shaped conclusions
// (failure/timed_out/cancelled) win — even one failing job means
// stage.failed, regardless of the others.
func TestDeriveEvents_AnyJobFails(t *testing.T) {
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	runs := []ghsource.Run{{
		ID: 1, HeadSHA: "abc", Workflow: "CI",
		Jobs: []ghsource.Job{
			{Name: "Test", Status: "completed", Conclusion: "success", StartedAt: start, CompletedAt: start.Add(time.Minute)},
			{Name: "Lint", Status: "completed", Conclusion: "failure", StartedAt: start, CompletedAt: start.Add(2 * time.Minute)},
		},
	}}

	got := ghsource.DeriveEvents("ci", runs, stageJobs("Test", "Lint"))
	if !hasEvent(got["abc"], "ci", "failed") {
		t.Errorf("expected ci:failed when any job fails, got %+v", got["abc"])
	}
	if hasEvent(got["abc"], "ci", "passed") {
		t.Errorf("must not emit ci:passed alongside ci:failed, got %+v", got["abc"])
	}
}

// TestDeriveEvents_StillRunning_NoTerminalEvent: when at least one
// completed-set job is still in flight (conclusion is empty), we have
// no aggregatable terminal verdict. Started event still fires, but no
// passed / failed.
func TestDeriveEvents_StillRunning_NoTerminalEvent(t *testing.T) {
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	runs := []ghsource.Run{{
		ID: 1, HeadSHA: "abc", Workflow: "CI",
		Jobs: []ghsource.Job{
			{Name: "Test", Status: "completed", Conclusion: "success", StartedAt: start, CompletedAt: start.Add(time.Minute)},
			{Name: "Lint", Status: "in_progress", Conclusion: "", StartedAt: start},
		},
	}}

	got := ghsource.DeriveEvents("ci", runs, stageJobs("Test", "Lint"))
	if !hasEvent(got["abc"], "ci", "started") {
		t.Errorf("expected ci:started while in flight, got %+v", got["abc"])
	}
	if hasEvent(got["abc"], "ci", "passed") || hasEvent(got["abc"], "ci", "failed") {
		t.Errorf("must not emit terminal event while any matching job is still running, got %+v", got["abc"])
	}
}

// TestDeriveEvents_FiltersJobsByName: jobs outside the configured set
// don't influence the verdict, even if they failed. Important when the
// user only cares about a subset of the workflow.
func TestDeriveEvents_FiltersJobsByName(t *testing.T) {
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	runs := []ghsource.Run{{
		ID: 1, HeadSHA: "abc", Workflow: "CI",
		Jobs: []ghsource.Job{
			{Name: "Test", Status: "completed", Conclusion: "success", StartedAt: start, CompletedAt: start.Add(time.Minute)},
			{Name: "Lint", Status: "completed", Conclusion: "failure", StartedAt: start, CompletedAt: start.Add(time.Minute)},
		},
	}}

	got := ghsource.DeriveEvents("ci", runs, stageJobs("Test"))
	if !hasEvent(got["abc"], "ci", "passed") {
		t.Errorf("Lint's failure must not affect ci:passed for Test-only mapping, got %+v", got["abc"])
	}
	if hasEvent(got["abc"], "ci", "failed") {
		t.Errorf("must not emit ci:failed for an off-list job, got %+v", got["abc"])
	}
}

// TestDeriveEvents_DistinctStartedAndCompletedSets: when the started
// and completed lists differ, started-set jobs alone power the started
// event, and completed-set jobs alone power the terminal event.
func TestDeriveEvents_DistinctStartedAndCompletedSets(t *testing.T) {
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	runs := []ghsource.Run{{
		ID: 1, HeadSHA: "abc", Workflow: "CI",
		Jobs: []ghsource.Job{
			// Started signal: "queue" — but doesn't gate the terminal.
			{Name: "queue", Status: "completed", Conclusion: "success", StartedAt: start, CompletedAt: start.Add(10 * time.Second)},
			// Completed signal: "test" — terminal verdict.
			{Name: "test", Status: "completed", Conclusion: "success", StartedAt: start.Add(time.Minute), CompletedAt: start.Add(5 * time.Minute)},
		},
	}}

	var js config.JobSet
	if err := js.UnmarshalJSON([]byte(`{"started":["queue"],"completed":["test"]}`)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	got := ghsource.DeriveEvents("ci", runs, js)
	started := findEvent(got["abc"], "ci", "started")
	if started == nil {
		t.Fatalf("expected ci:started, got %+v", got["abc"])
	}
	if !started.Time.Equal(start) {
		t.Errorf("ci:started.Time: expected %v (queue's started_at), got %v", start, started.Time)
	}
	if !hasEvent(got["abc"], "ci", "passed") {
		t.Errorf("expected ci:passed from completed-set, got %+v", got["abc"])
	}
}

// TestDeriveEvents_NoMatchingJobs: a run whose jobs don't match any of
// the configured names produces no events at all for that SHA — not
// even a started.
func TestDeriveEvents_NoMatchingJobs(t *testing.T) {
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	runs := []ghsource.Run{{
		ID: 1, HeadSHA: "abc", Workflow: "CI",
		Jobs: []ghsource.Job{
			{Name: "BuildDocs", Status: "completed", Conclusion: "success", StartedAt: start, CompletedAt: start.Add(time.Minute)},
		},
	}}

	got := ghsource.DeriveEvents("ci", runs, stageJobs("Test"))
	if len(got) != 0 {
		t.Errorf("expected no events when no jobs match, got %+v", got)
	}
}

// --- helpers ---------------------------------------------------------

func hasEvent(events []clarityrefs.Event, stage, status string) bool {
	return findEvent(events, stage, status) != nil
}

func findEvent(events []clarityrefs.Event, stage, status string) *clarityrefs.Event {
	for i := range events {
		if events[i].Stage == stage && events[i].Status == status {
			return &events[i]
		}
	}
	return nil
}
