// Package initcmd implements `git clarity init`: the act of setting a
// repository up for clarity, which is writing a `.clarity` file for the
// user to commit and push to the trunk.
//
// There is nothing to ask: the trunk is detected from the repository, the
// lead time mode has a default, and the events source defaults to the
// repo's own events ref — so init is a one-shot, idempotent command rather
// than a wizard. It exists as a command (and not only as the TUI's welcome
// screen) for the caller that cannot press enter in a TUI: a script, a CI
// bootstrap, or the agent following the `git clarity onboard` prompt.
//
// Init deliberately pushes nothing. The config lives on the trunk so that
// review and branch protection cover it, and a tool that pushed to the
// trunk would be bypassing exactly that. Writing the file is clarity's
// job; landing it is an ordinary commit, owned by whoever runs this.
package initcmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/configload"
	"github.com/ezcdlabs/clarity/internal/gitlog"
)

// Options configures Run.
type Options struct {
	RepoPath string
	Out      io.Writer // progress and confirmation; required
}

// Run sets the repository up. When `.clarity` is already on the trunk it
// reports so and touches nothing; when the file is already in the working
// tree it repeats what remains to be done rather than clobbering a setup
// mid-flight. Otherwise it writes the file — seeded from the legacy
// refs/clarity/config when that is where the repo's config still lives, so
// migration carries choices over instead of resetting them — and leaves
// the commit to the user.
func Run(opts Options) error {
	branch := gitlog.DetectTrunk(opts.RepoPath)
	if branch == "" {
		branch = DetectTrunk(opts.RepoPath)
	}
	if branch == "" {
		branch = config.DefaultBranch
	}

	stored, origin, err := configload.Stored(opts.RepoPath, branch)
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	if origin == configload.OriginTrunkFile {
		fmt.Fprintf(opts.Out, "clarity is already set up on this repo (%s is on %s) — nothing to do\n",
			config.ClarityFileName, branch)
		return nil
	}

	path := filepath.Join(opts.RepoPath, config.ClarityFileName)
	if _, statErr := os.Stat(path); statErr == nil {
		fmt.Fprintf(opts.Out, "%s is already written — commit and push it to %s to finish setup\n",
			config.ClarityFileName, branch)
		return nil
	}

	content := config.DefaultClarityConfigFile()
	migrated := origin == configload.OriginConfigRef && len(bytes.TrimSpace(stored)) > 0
	if migrated {
		content = stored
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("init: write %s: %w", config.ClarityFileName, err)
	}

	if migrated {
		fmt.Fprintf(opts.Out, "wrote %s from the legacy %s — commit and push it to %s; the ref is ignored once the file lands\n",
			config.ClarityFileName, clarityrefs.ConfigRef, branch)
	} else {
		fmt.Fprintf(opts.Out, "wrote %s — commit and push it to %s to finish setup\n",
			config.ClarityFileName, branch)
	}
	fmt.Fprintf(opts.Out, "events will light the dashboard up — instrument your pipeline with `git clarity report`, or run `git clarity onboard` to hand the job to an agent\n")
	return nil
}
