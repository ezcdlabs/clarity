package refs

import (
	"os/exec"
	"strings"

	"github.com/ezcdlabs/clarity/internal/gitenv"
)

// ClarityFetchRefspec is the refspec that brings the events ref into the
// local repository whenever the user runs `git fetch` (no args). The TUI
// adds it on first run so subsequent fetches don't need any clarity-specific
// flags.
const ClarityFetchRefspec = "+refs/clarity/events:refs/clarity/events"

// EnsureClarityFetchRefspec adds ClarityFetchRefspec to remote.<remote>.fetch
// in the repository's git config if it isn't already present and the remote
// has actually published a refs/clarity/events ref. We gate on the remote
// having the ref because `git fetch` (no args) aborts the entire command
// when any configured refspec fails with "couldn't find remote ref" — so
// adding the refspec to a brand-new repo where nobody has run `git clarity
// report` yet would break the user's plain `git fetch`. Once events appear
// on the remote, a subsequent `git clarity` launch re-runs this check and
// the refspec gets added. Idempotent in both directions.
func EnsureClarityFetchRefspec(repoPath, remote string) error {
	if !remoteHasEventsRef(repoPath, remote) {
		return nil
	}

	key := "remote." + remote + ".fetch"
	existing, err := gitConfigGetAll(repoPath, key)
	if err != nil {
		return err
	}
	for _, line := range existing {
		if strings.TrimSpace(line) == ClarityFetchRefspec {
			return nil
		}
	}
	return git(repoPath, "config", "--add", key, ClarityFetchRefspec)
}

// remoteHasEventsRef reports whether the remote currently publishes
// refs/clarity/events. Errors (network, auth, unreachable host) collapse
// to "no" — the TUI continues to launch and re-checks on its next run.
func remoteHasEventsRef(repoPath, remote string) bool {
	cmd := exec.Command("git", "ls-remote", remote, "refs/clarity/events")
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

// gitConfigGetAll returns every value associated with key, or an empty slice
// if the key is unset (`git config` exits 1 in that case).
func gitConfigGetAll(repoPath, key string) ([]string, error) {
	cmd := exec.Command("git", "config", "--get-all", key)
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}
