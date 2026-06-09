// Package ghsource is the Source adapter that reads commits and pipeline
// signals from GitHub Actions. The commit log still comes from the local
// repository (via internal/gitlog) — only the events are derived from
// GH Actions runs.
//
// Implements core.Source.
package ghsource

import "time"

// GHClient abstracts the minimum slice of the GitHub Actions API the
// adapter needs. The real implementation shells out to the `gh` CLI
// (which handles auth via the user's existing GitHub login); tests
// substitute a fake that returns canned payloads.
//
// One method per query so tests can assert which call the source made
// — and so a future on-the-wire client (octokit-go, etc.) can drop in
// without touching the source.
type GHClient interface {
	// ListWorkflows returns every workflow defined on the repo. Used
	// by Source.Validate to check connectivity + that the configured
	// workflow names actually exist.
	ListWorkflows() ([]WorkflowSummary, error)
	// ListRuns returns recent workflow runs for the named workflow on
	// the given branch, ordered by UpdatedAt newest-first. Each run
	// carries its Jobs eagerly so the source doesn't need a second
	// round trip per run. since bounds the lookback window; the
	// production impl currently ignores it (GH's runs endpoint
	// doesn't expose an updated_at filter — we always fetch the most
	// recent page and merge into the cache by run ID).
	ListRuns(workflowName, branch string, since time.Time) ([]Run, error)
}

// WorkflowSummary is the surface of one workflow needed by Validate
// and by the init discovery flow — enough to identify, pick, and look
// up jobs. Lives here (not client_gh.go) so the GHClient interface
// can reference it.
type WorkflowSummary struct {
	ID   int64
	Name string
	Path string
}

// Run is one GitHub Actions workflow run, plus its jobs. The shape is
// deliberately a small subset of the GH API payload — only the fields
// the event derivation reads. Anything else we'd need later (titles,
// URLs) can be added without breaking callers.
type Run struct {
	ID        int64     `json:"id"`
	Workflow  string    `json:"workflow"`
	HeadSHA   string    `json:"head_sha"`
	UpdatedAt time.Time `json:"updated_at"`
	Jobs      []Job     `json:"jobs"`
}

// Job is one job inside a Run.
//
//	Status     "queued" | "in_progress" | "completed"
//	Conclusion "success" | "failure" | "cancelled" | "timed_out" |
//	           "action_required" | "neutral" | "skipped" | "" (still running)
//
// The derivation in events.go treats the two failure-shaped conclusions
// (failure / timed_out / cancelled) as "failed" and "success" as
// "passed"; anything else is ignored for the stage.passed/failed event.
type Job struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}
