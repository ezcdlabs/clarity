// Package report implements the `git clarity report <stage> <status>`
// subcommand: resolve the commit SHA, build the event payload (core fields
// plus an opportunistic CI metadata block), and append it to the events ref.
package report

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/ci"
	"github.com/ezcdlabs/clarity/internal/gitenv"
)

// Options configures a single report invocation.
type Options struct {
	RepoPath string
	Remote   string    // defaults to "origin"
	Stage    string    // required
	Status   string    // required
	Time     time.Time // defaults to time.Now()
	SHA      string    // defaults to env (GITHUB_SHA / CI_COMMIT_SHA), then HEAD
}

// validStages is the closed set of stage names clarity recognises. Trunk-based
// development cares about exactly two state transitions — code passes CI, and
// code reaches production — so the report subcommand rejects anything else
// rather than letting custom stage names drift into the events ref.
var validStages = map[string]bool{
	"ci":     true,
	"deploy": true,
}

// validStatuses is the closed set of statuses recognised per stage.
var validStatuses = map[string]bool{
	"started": true,
	"passed":  true,
	"failed":  true,
	"skipped": true,
}

// Run resolves the SHA, attaches CI metadata, and writes one event. Returns
// the SHA the event was attached to so callers (e.g. the binary) can echo it
// back to the user for confirmation.
func Run(opts Options) (string, error) {
	if err := validateStageStatus(opts.Stage, opts.Status); err != nil {
		return "", err
	}
	opts, err := Resolve(opts)
	if err != nil {
		return "", err
	}

	event := clarityrefs.Event{
		Stage:  opts.Stage,
		Status: opts.Status,
		Time:   opts.Time,
		CI:     ci.Detect(),
	}
	if err := clarityrefs.WriteEvent(opts.RepoPath, opts.Remote, opts.SHA, event); err != nil {
		return "", err
	}
	return opts.SHA, nil
}

// Resolve fills in the values Run would otherwise derive internally: the
// commit SHA (from the CI environment, else HEAD), the event timestamp, and
// the remote. Exported so a caller can learn what a report is *about* to
// write before it writes it — which is what makes CommandLine able to echo an
// invocation that reproduces the event exactly.
//
// Resolving is idempotent: applying it to already-resolved Options is a
// no-op, so Run can call it unconditionally.
func Resolve(opts Options) (Options, error) {
	if opts.Time.IsZero() {
		opts.Time = time.Now()
	}
	if opts.Remote == "" {
		opts.Remote = "origin"
	}
	if opts.SHA == "" {
		sha, err := resolveSHA(opts.RepoPath)
		if err != nil {
			return opts, err
		}
		opts.SHA = sha
	}
	return opts, nil
}

// CommandLine renders opts as the fully-explicit `git clarity report`
// invocation that writes this exact event.
//
// The point is that it can be copy-pasted to recover from a failed report. An
// event's on-disk payload stores its timestamp as Unix seconds, so a
// second-precision RFC3339 --at round-trips exactly, and event filenames are
// content-addressed — meaning a re-run from the same environment produces the
// same file and collapses into a no-op rather than a duplicate. Sub-second
// precision is dropped for the same reason: keeping it would print a command
// that writes a different event from the one it claims to reproduce.
//
// The timestamp is rendered in UTC whatever the runner's zone. The event
// records an instant, so the offset carries no information, and a stable
// rendering keeps pasted commands comparable across machines.
func CommandLine(opts Options) string {
	return fmt.Sprintf("git clarity report --sha %s --at %s %s %s",
		opts.SHA,
		opts.Time.UTC().Truncate(time.Second).Format(time.RFC3339),
		opts.Stage,
		opts.Status,
	)
}

// FailureError wraps a failed report with the command that puts the event
// back.
//
// A dropped report is not a self-correcting condition: nothing retries it
// later, so the stage stays unrecorded and the TUI shows the commit as
// in-flight indefinitely — a deploy that finished hours ago still spinning.
// The failure is also usually transient and environmental (a push race, a
// network blip), so the recovery is genuinely just running the same event
// again, and the message says so with a command rather than a description.
//
// The cause stays unwrappable, so callers can still inspect it.
func FailureError(opts Options, err error) error {
	// Trimmed because the cause is usually git's own multi-line stderr,
	// which ends in blank lines that would otherwise push the recovery
	// command away from the text introducing it.
	cause := strings.TrimSpace(err.Error())
	return &failureError{
		cause: err,
		msg: fmt.Sprintf(
			"failed to report %s %s: %s\n\n"+
				"The event was not recorded — %s will keep showing as in-flight in\n"+
				"`git clarity` until it is. Re-run this once the problem is fixed:\n\n"+
				"  %s\n",
			opts.Stage, opts.Status, cause,
			shortSHA(opts.SHA),
			CommandLine(opts),
		),
	}
}

// failureError carries the rendered advice while keeping the underlying cause
// inspectable. A plain fmt.Errorf can't do both: %w embeds the cause's text
// verbatim, blank lines and all.
type failureError struct {
	msg   string
	cause error
}

func (e *failureError) Error() string { return e.msg }
func (e *failureError) Unwrap() error { return e.cause }

// shortSHA abbreviates a commit SHA for prose, matching how the TUI and the
// report confirmation line display one.
func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// validateStageStatus enforces the closed sets shared by Run and RunBatch.
func validateStageStatus(stage, status string) error {
	if stage == "" {
		return fmt.Errorf("stage is required")
	}
	if !validStages[stage] {
		return fmt.Errorf("stage must be 'ci' or 'deploy', got %q", stage)
	}
	if status == "" {
		return fmt.Errorf("status is required")
	}
	if !validStatuses[status] {
		return fmt.Errorf("status must be 'started', 'passed', 'failed' or 'skipped', got %q", status)
	}
	return nil
}

// BatchEvent is one input event for a batch invocation. All fields are
// required — backfill callers must supply explicit SHA and timestamp since
// the live environment doesn't reflect the historical run.
type BatchEvent struct {
	SHA    string
	Time   time.Time
	Stage  string
	Status string
}

// BatchOptions configures a RunBatch invocation.
type BatchOptions struct {
	RepoPath string
	Remote   string // defaults to "origin"
}

// RunBatch validates each event, groups them by SHA, and writes them all in
// a single commit + push to the events ref. Intended for backfill / migration
// paths — live reporting should keep using Run so each event lands as its own
// audit-able commit. Backfill events do not get CI metadata auto-attached:
// the local env doesn't describe the historical pipeline that produced them.
func RunBatch(opts BatchOptions, events []BatchEvent) error {
	if len(events) == 0 {
		return nil
	}
	if opts.Remote == "" {
		opts.Remote = "origin"
	}
	eventsBySHA := make(map[string][]clarityrefs.Event, len(events))
	for i, be := range events {
		if be.SHA == "" {
			return fmt.Errorf("event %d: sha is required", i)
		}
		if be.Time.IsZero() {
			return fmt.Errorf("event %d: time is required", i)
		}
		if err := validateStageStatus(be.Stage, be.Status); err != nil {
			return fmt.Errorf("event %d: %w", i, err)
		}
		eventsBySHA[be.SHA] = append(eventsBySHA[be.SHA], clarityrefs.Event{
			Stage:  be.Stage,
			Status: be.Status,
			Time:   be.Time,
		})
	}
	return clarityrefs.WriteEvents(opts.RepoPath, opts.Remote, eventsBySHA)
}

// resolveSHA prefers CI-provided commit SHAs over git HEAD, since CI runners
// often check out a detached HEAD that doesn't match the commit being tested.
func resolveSHA(repoPath string) (string, error) {
	for _, env := range []string{"GITHUB_SHA", "CI_COMMIT_SHA"} {
		if v := os.Getenv(env); v != "" {
			return v, nil
		}
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
