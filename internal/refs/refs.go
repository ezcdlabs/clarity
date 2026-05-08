package refs

import (
	"fmt"
	"os/exec"

	"github.com/ezcdlabs/clarity/internal/gitenv"
)

// PushRef pushes a local ref to a remote ref. Returns an error if the push is
// rejected (e.g. not a fast-forward).
func PushRef(repoPath, remote, localRef, remoteRef string) error {
	return git(repoPath, "push", remote, localRef+":"+remoteRef)
}

// DeleteRef deletes a ref from the remote.
func DeleteRef(repoPath, remote, remoteRef string) error {
	return git(repoPath, "push", remote, "--delete", remoteRef)
}

// FetchRef fetches a single remote ref into a local ref.
func FetchRef(repoPath, remote, remoteRef, localRef string) error {
	return git(repoPath, "fetch", remote, remoteRef+":"+localRef)
}

func git(repoPath string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v: %w\n%s", args, err, out)
	}
	return nil
}
