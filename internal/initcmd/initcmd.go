// Package initcmd implements `git clarity init`: the act of setting a
// repository up for clarity, which is writing refs/clarity/config and
// pushing it.
//
// There is nothing to ask: the trunk is detected from the repository, the
// lead time mode has a default, and the events source defaults to the
// repo's own events ref — so init is a one-shot, idempotent command rather
// than a wizard. It exists as a command (and not only as the TUI's welcome
// screen) for the caller that cannot press enter in a TUI: a script, a CI
// bootstrap, or the agent following the `git clarity onboard` prompt.
package initcmd

import (
	"fmt"
	"io"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/config"
)

// Options configures Run.
type Options struct {
	RepoPath string
	Remote   string    // defaults to "origin"
	Out      io.Writer // progress and confirmation; required
}

// Run sets the repository up: when refs/clarity/config already exists it
// reports so and touches nothing — choices someone already made must
// survive a re-run — otherwise it writes the default config and pushes.
//
// The push is the one requirement init has: setup is a write to the shared
// repository, so it needs push permission, once, by whoever sets the repo
// up. The config travels to everyone else by fetch from then on.
func Run(opts Options) error {
	remote := opts.Remote
	if remote == "" {
		remote = "origin"
	}

	// The remote's truth, not a stale local ref: init answers "is this repo
	// set up", and a fetch that fails must surface rather than let init
	// re-write a config that already exists upstream.
	if err := clarityrefs.FetchConfigRef(opts.RepoPath, remote); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	if _, ok, err := clarityrefs.ReadConfigFile(opts.RepoPath); err != nil {
		return fmt.Errorf("init: %w", err)
	} else if ok {
		fmt.Fprintf(opts.Out, "clarity is already set up on this repo (%s exists) — nothing to do\n", clarityrefs.ConfigRef)
		return nil
	}

	if err := clarityrefs.WriteConfigFile(opts.RepoPath, remote, config.DefaultClarityConfigFile(), "clarity: set up"); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	fmt.Fprintf(opts.Out, "clarity is set up: %s written and pushed to %s\n", clarityrefs.ConfigRef, remote)
	fmt.Fprintf(opts.Out, "events will light the dashboard up — instrument your pipeline with `git clarity report`, or run `git clarity onboard` to hand the job to an agent\n")
	return nil
}
