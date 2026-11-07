package initcmd

import (
	"os/exec"
	"strings"

	"github.com/ezcdlabs/clarity/internal/gitenv"
	"github.com/ezcdlabs/clarity/internal/gitlog"
)

// DetectTrunk guesses the repository's trunk branch, for the init wizard's
// branch prompt to offer as its default. It is a suggestion, never an
// answer — the user confirms or overrides it at the prompt — which is why
// every failure path degrades to a weaker guess instead of an error, and
// why, unlike the runtime chain (gitlog.DetectTrunk), the checked-out
// branch is an acceptable last guess here.
//
// origin/HEAD (the remote's default branch, recorded at clone time) outranks
// the checked-out branch, because init can be run from a feature branch; a
// repo with no remote falls back to whatever is checked out; and "" means
// git had no answer at all, leaving the caller its own default.
func DetectTrunk(repoPath string) string {
	if branch := gitlog.RemoteDefaultBranch(repoPath); branch != "" {
		return branch
	}
	return gitShortRef(repoPath, "HEAD")
}

// gitShortRef resolves a symbolic ref to its short name, or "" when it
// doesn't resolve — detached HEAD or no repository.
func gitShortRef(repoPath, ref string) string {
	cmd := exec.Command("git", "symbolic-ref", "--short", ref)
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
