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
	if opts.Stage == "" {
		return "", fmt.Errorf("stage is required")
	}
	if !validStages[opts.Stage] {
		return "", fmt.Errorf("stage must be 'ci' or 'deploy', got %q", opts.Stage)
	}
	if opts.Status == "" {
		return "", fmt.Errorf("status is required")
	}
	if !validStatuses[opts.Status] {
		return "", fmt.Errorf("status must be 'started', 'passed', 'failed' or 'skipped', got %q", opts.Status)
	}
	if opts.Time.IsZero() {
		opts.Time = time.Now()
	}
	if opts.Remote == "" {
		opts.Remote = "origin"
	}

	sha := opts.SHA
	if sha == "" {
		s, err := resolveSHA(opts.RepoPath)
		if err != nil {
			return "", err
		}
		sha = s
	}

	event := clarityrefs.Event{
		Stage:  opts.Stage,
		Status: opts.Status,
		Time:   opts.Time,
		CI:     ci.Detect(),
	}
	if err := clarityrefs.WriteEvent(opts.RepoPath, opts.Remote, sha, event); err != nil {
		return "", err
	}
	return sha, nil
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
