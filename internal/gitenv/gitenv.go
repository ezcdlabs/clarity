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

// localeVars are the variables that decide which language git speaks. They
// are dropped and replaced rather than merely overridden, so no leftover
// setting can win.
var localeVars = map[string]bool{
	"LC_ALL":      true,
	"LC_MESSAGES": true,
	"LANG":        true,
	"LANGUAGE":    true,
}

// Clean returns a copy of os.Environ with git-context variables stripped,
// plus GIT_TERMINAL_PROMPT=0 to prevent git from blocking on credential
// prompts, and a C message locale.
//
// Use this as cmd.Env for any exec.Cmd that calls git, to avoid inheriting
// GIT_DIR and friends when clarity is itself invoked as a git subcommand.
//
// The locale is pinned because clarity classifies several git outcomes by
// matching git's own text: "couldn't find remote ref" to recognise a repo
// that has never reported anything, "unknown option" to fall back when git
// is too old for a flag, plus the push-rejection and damaged-object
// matchers. Every one of those strings is translated, so under a non-English
// locale they stop matching and the behaviour they guard silently inverts —
// most sharply on the first report in a repo, where failing to recognise
// "no events ref yet" turns an ordinary first run into a hard error.
//
// LANGUAGE is cleared as well as LC_ALL set: gettext consults it first, so
// leaving it in place would re-translate the messages LC_ALL=C pins.
func Clean() []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+3)
	for _, e := range env {
		key := e
		if i := strings.IndexByte(e, '='); i >= 0 {
			key = e[:i]
		}
		if gitInheritedVars[key] || localeVars[key] {
			continue
		}
		out = append(out, e)
	}
	out = append(out, "GIT_TERMINAL_PROMPT=0", "LC_ALL=C", "LANGUAGE=")
	return out
}
