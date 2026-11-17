// Package trustcmd implements `git clarity trust`: showing the configured
// exec source command and recording the user's approval of exactly it.
//
// Approval is a human act, so the command never records anything unless a
// person typed the confirmation in an interactive terminal. Agents with a
// shell could write the git config entry directly — nothing can stop a
// hand that already holds the pen — but this command will not be the
// automation that does it, and projects can deny it in their agent
// permission settings besides.
package trustcmd

import (
	"bufio"
	"fmt"
	"io"

	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/configload"
	"github.com/ezcdlabs/clarity/internal/trust"
)

// confirmWord is what the user must type, exactly. A single letter is a
// reflex; a word is a decision. Case-sensitive, because the prompt shows
// the exact string to type and "close enough" is not a property approvals
// should have.
const confirmWord = "trust"

// Options configures Run.
type Options struct {
	RepoPath string
	In       io.Reader // the user's answers; required when Interactive
	Out      io.Writer // required
	// Interactive is whether a human can actually answer — stdin and
	// stdout are a terminal. The caller decides, because testability and
	// honesty both want the TTY probe outside this package.
	Interactive bool
}

// Run shows the configured exec source command and records approval of it.
func Run(opts Options) error {
	cfg, err := configload.Load(opts.RepoPath)
	if err != nil {
		return fmt.Errorf("trust: %w", err)
	}
	if cfg.SourceExec == nil {
		fmt.Fprintf(opts.Out, "nothing to trust — %s on %s configures no exec source (clarity.source is not %q)\n",
			config.ClarityFileName, cfg.Branch, config.SourceExecName)
		return nil
	}
	argv := cfg.SourceExec.Argv

	if ok, err := trust.IsTrusted(opts.RepoPath, argv); err != nil {
		return fmt.Errorf("trust: %w", err)
	} else if ok {
		fmt.Fprintf(opts.Out, "already trusted: %s\n", trust.Display(argv))
		return nil
	}

	if !opts.Interactive {
		return fmt.Errorf("trust: approving a source command is a human decision — run `git clarity trust` in an interactive terminal")
	}

	fmt.Fprintf(opts.Out, "%s on %s configures this events source command:\n\n", config.ClarityFileName, cfg.Branch)
	fmt.Fprintf(opts.Out, "    %s\n\n", trust.Display(argv))
	if previous, ok := trust.RecordedCommand(opts.RepoPath); ok {
		fmt.Fprintf(opts.Out, "the configured command has changed since it was last approved:\n")
		fmt.Fprintf(opts.Out, "    previously approved: %s\n\n", previous)
	}
	fmt.Fprintf(opts.Out, "clarity will run it, feed it the trunk commits on stdin, and treat\n")
	fmt.Fprintf(opts.Out, "everything it prints as true. It runs with your permissions, and if it\n")
	fmt.Fprintf(opts.Out, "invokes code from this repository, approving it also approves future\n")
	fmt.Fprintf(opts.Out, "versions of that code.\n\n")
	fmt.Fprintf(opts.Out, "type %q to approve exactly this command, anything else to decline: ", confirmWord)

	answer, _ := bufio.NewReader(opts.In).ReadString('\n')
	if answer != confirmWord+"\n" && answer != confirmWord {
		fmt.Fprintf(opts.Out, "\nnot trusted — nothing recorded\n")
		return nil
	}
	if err := trust.Record(opts.RepoPath, argv); err != nil {
		return fmt.Errorf("trust: %w", err)
	}
	fmt.Fprintf(opts.Out, "\ntrusted: %s\n", trust.Display(argv))
	fmt.Fprintf(opts.Out, "recorded in this clone's .git/config only — a fresh clone asks again, and any change to the command asks again\n")
	return nil
}
