package clarityrefs

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ezcdlabs/clarity/internal/gitenv"
)

// ReadTrunkFile returns the blob at path on the trunk's tip — the shared,
// committed truth, never the working tree. The remote-tracking ref
// (refs/remotes/origin/<branch>) is authoritative whenever it exists: an
// unpushed local commit is not shared truth yet, and a checkout of some
// other branch must not change what clarity reads. The local branch head
// answers only for a repository with no remote-tracking ref at all.
//
// ok=false means the file is not on the trunk (or the branch resolves
// nowhere), both of which are ordinary states rather than errors.
func ReadTrunkFile(repoPath, branch, path string) ([]byte, bool, error) {
	for _, ref := range []string{"refs/remotes/origin/" + branch, "refs/heads/" + branch} {
		exists, err := refExists(repoPath, ref)
		if err != nil {
			return nil, false, err
		}
		if !exists {
			continue
		}
		return readRefPath(repoPath, ref, path)
	}
	return nil, false, nil
}

// HasTrunkRef reports whether any local ref can answer for the trunk —
// the remote-tracking ref or the local branch head. False is the shape of
// a CI job checked out from a tag, which fetched no branch at all.
func HasTrunkRef(repoPath, branch string) (bool, error) {
	for _, ref := range []string{"refs/remotes/origin/" + branch, "refs/heads/" + branch} {
		exists, err := refExists(repoPath, ref)
		if err != nil || exists {
			return exists, err
		}
	}
	return false, nil
}

// FetchTrunkTip fetches just the trunk's tip into the remote-tracking ref,
// at depth 1 — the caller wants one blob out of one tree, not history.
// Meant for repositories where HasTrunkRef is false (a tag checkout);
// callers with a trunk ref must not use it, because the depth-1 fetch
// records a shallow boundary, which is noise in a developer's full clone
// and nothing in a CI workspace that is discarded after the job.
func FetchTrunkTip(repoPath, remote, branch string) error {
	if remote == "" {
		remote = "origin"
	}
	refspec := "+refs/heads/" + branch + ":refs/remotes/" + remote + "/" + branch
	cmd := exec.Command("git", append(noAutoGC, "fetch", "--depth=1", remote, refspec)...)
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("fetch %s from %s: %w\n%s", branch, remote, err,
			gitenv.Redact(strings.TrimSpace(string(out))))
	}
	return nil
}

// readRefPath reads <ref>:<path> as a blob. A path absent from the tree is
// ok=false; everything else that stops the read is an error.
func readRefPath(repoPath, ref, path string) ([]byte, bool, error) {
	// --verify --quiet exits 1 silently for a path that is not in the
	// tree, which is the ordinary state this probe exists to detect. The
	// ref itself was verified by the caller, so the ambiguity resolveRef
	// documents (a ref present but unreadable) cannot hide here.
	probe := exec.Command("git", "rev-parse", "--verify", "--quiet", ref+":"+path)
	probe.Dir = repoPath
	probe.Env = gitenv.Clean()
	out, err := probe.Output()
	if err != nil {
		return nil, false, nil
	}
	blob := strings.TrimSpace(string(out))
	if blob == "" {
		return nil, false, nil
	}

	cmd := exec.Command("git", "cat-file", "blob", blob)
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		return nil, false, fmt.Errorf("read %s:%s: %w: %s", ref, path, err,
			gitenv.Redact(strings.TrimSpace(stderr.String())))
	}
	return data, true, nil
}
