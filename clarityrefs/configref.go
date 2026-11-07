package clarityrefs

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/ezcdlabs/clarity/internal/gitenv"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ConfigRef is the ref that marks a repository as set up for clarity and
// carries its shared configuration. Existence is the setup signal — one
// raceless, binary fact the TUI's welcome screen keys off — and the file
// inside travels with `git fetch` exactly as the events do, so a client
// with no checkout (the mobile app) reads the same config everyone else
// does. The ref is a commit chain: every config change commits on top of
// the previous one, so plain `git log refs/clarity/config` is the audit
// trail.
const ConfigRef = "refs/clarity/config"

// ConfigFileName is the single file inside the config ref's tree. Named so
// every reader — CLI through this package, mobile through go-git — asks
// for the same path.
const ConfigFileName = "config"

// ReadConfigFile returns the config file from the local config ref.
// ok=false means the ref does not exist — the repo has not been set up —
// which is an ordinary state, not an error. Reads are local-only; callers
// fetch first (FetchConfigRef), best-effort, so offline still answers
// with the last config fetched.
func ReadConfigFile(repoPath string) ([]byte, bool, error) {
	head, ok, err := resolveRef(repoPath, ConfigRef)
	if err != nil || !ok {
		return nil, false, err
	}
	paths, hashes, err := listTree(repoPath, head.String())
	if err != nil {
		return nil, false, err
	}
	for i, p := range paths {
		if p != ConfigFileName {
			continue
		}
		contents, err := catFileBatch(repoPath, hashes[i:i+1])
		if err != nil {
			return nil, false, err
		}
		return contents[0], true, nil
	}
	// The ref exists but holds no config file: still set up — the ref's
	// existence is the signal — just with nothing configured beyond the
	// defaults.
	return nil, true, nil
}

// FetchConfigRef updates the local config ref from the remote. A remote
// with no config ref yet is not an error: it is every repo that has not
// been set up. A remote that could not be reached is, and comes back as a
// *FetchError naming the remote it tried.
func FetchConfigRef(repoPath, remote string) error {
	if remote == "" {
		remote = "origin"
	}
	refspec := "+" + ConfigRef + ":" + ConfigRef
	out, err := fetchRefspec(repoPath, remote, refspec)
	if err == nil {
		return nil
	}
	if strings.Contains(out, "couldn't find remote ref") {
		return nil // not set up yet; nothing to fetch
	}
	return &FetchError{
		Remote: remote,
		URL:    remoteURL(repoPath, remote),
		Ref:    ConfigRef,
		Output: gitenv.Redact(out),
		Err:    err,
	}
}

// WriteConfigFile replaces the config ref's content with the given file and
// pushes it, chaining the new commit onto whatever the remote already has so
// the ref keeps its history. Identical content is a no-op — re-running
// setup must be free. The same optimistic loop as the events writer: fetch,
// commit on the fetched tip, push, and on a fast-forward rejection go
// around again on top of whoever won.
func WriteConfigFile(repoPath, remote string, data []byte, message string) error {
	if remote == "" {
		remote = "origin"
	}
	defer deleteLocalRef(repoPath, ConfigRef)
	for attempt := 0; ; attempt++ {
		// A remote with no config ref is fine. A remote that could not be
		// reached is not: continuing would rebuild the ref parentless, and a
		// push that somehow landed would discard the config history.
		if err := FetchConfigRef(repoPath, remote); err != nil {
			return err
		}

		repo, err := gogit.PlainOpen(repoPath)
		if err != nil {
			return fmt.Errorf("open repo: %w", err)
		}

		parentHash, hasParent, err := resolveRef(repoPath, ConfigRef)
		if err != nil {
			return fmt.Errorf("read config ref: %w", err)
		}

		st := writeStorer(repo)
		treeHash, err := buildTree(st, map[string][]byte{ConfigFileName: data})
		if err != nil {
			return fmt.Errorf("build tree: %w", err)
		}
		if hasParent {
			if parentTree, ok := commitTree(repoPath, parentHash); ok && parentTree == treeHash {
				return nil // already says exactly this
			}
		}

		sig := &object.Signature{Name: "clarity", Email: "clarity@local", When: time.Now()}
		commit := &object.Commit{
			Author:    *sig,
			Committer: *sig,
			Message:   message,
			TreeHash:  treeHash,
		}
		if hasParent {
			commit.ParentHashes = []plumbing.Hash{parentHash}
		}
		enc := st.NewEncodedObject()
		if err := commit.Encode(enc); err != nil {
			return fmt.Errorf("encode commit: %w", err)
		}
		commitHash, err := storeObject(st, enc)
		if err != nil {
			return fmt.Errorf("store commit: %w", err)
		}
		ref := plumbing.NewHashReference(plumbing.ReferenceName(ConfigRef), commitHash)
		if err := repo.Storer.SetReference(ref); err != nil {
			return fmt.Errorf("set local ref: %w", err)
		}

		pushErr := pushRef(repoPath, remote, ConfigRef)
		if pushErr == nil {
			return nil
		}
		if isFastForwardRejected(pushErr) && attempt < maxWriteRetries {
			_ = deleteLocalRef(repoPath, ConfigRef)
			continue
		}
		return fmt.Errorf("push config ref: %w", pushErr)
	}
}

// pushRef pushes one local ref to the same name on the remote. --no-verify
// skips the user's pre-push hook for the same reason the events push does:
// clarity's refs are internal bookkeeping, not user code, and a hook gating
// real pushes shouldn't be able to block them.
func pushRef(repoPath, remote, ref string) error {
	cmd := exec.Command("git", append(noAutoGC, "push", "--no-verify", remote, ref+":"+ref)...)
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, gitenv.Redact(string(out)))
	}
	return nil
}

// deleteLocalRef removes a local ref so the next operation starts from a
// fresh fetch of the remote's truth.
func deleteLocalRef(repoPath, ref string) error {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return err
	}
	return repo.Storer.RemoveReference(plumbing.ReferenceName(ref))
}
