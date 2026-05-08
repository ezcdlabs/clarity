package gitenv

import (
	"os"
	"strings"
)

// gitInheritedVars are environment variables that git sets when it invokes
// external commands (e.g. "git clarity"). Subprocesses that in turn invoke git
// must not inherit these, otherwise git operates on the wrong directory.
var gitInheritedVars = map[string]bool{
	"GIT_DIR":       true,
	"GIT_WORK_TREE": true,
	"GIT_PREFIX":    true,
}

// Clean returns a copy of os.Environ with git-context variables stripped,
// plus GIT_TERMINAL_PROMPT=0 to prevent git from blocking on credential prompts.
// Use this as cmd.Env for any exec.Cmd that calls git, to avoid inheriting
// GIT_DIR and friends when clarity is itself invoked as a git subcommand.
func Clean() []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		key := e
		if i := strings.IndexByte(e, '='); i >= 0 {
			key = e[:i]
		}
		if !gitInheritedVars[key] {
			out = append(out, e)
		}
	}
	out = append(out, "GIT_TERMINAL_PROMPT=0")
	return out
}
