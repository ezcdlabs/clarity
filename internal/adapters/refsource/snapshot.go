package refsource

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// BuildSnapshot walks up to limit commits starting from the given branch
// and joins each one to its events from the local clarity events ref. Pure
// in the sense that it never touches the remote — callers (Source.Watch)
// are responsible for fetching first.
func BuildSnapshot(repoPath, branch string, limit int) (core.Snapshot, error) {
	if limit <= 0 {
		limit = 50
	}
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return core.Snapshot{}, fmt.Errorf("open repo: %w", err)
	}

	hash, err := resolveBranch(repo, branch)
	if err != nil {
		return core.Snapshot{}, err
	}

	eventsByCommit, err := clarityrefs.ReadAllEvents(repoPath)
	if err != nil {
		return core.Snapshot{}, fmt.Errorf("read events: %w", err)
	}

	iter, err := repo.Log(&gogit.LogOptions{From: hash})
	if err != nil {
		return core.Snapshot{}, fmt.Errorf("log: %w", err)
	}

	var stop = errors.New("limit reached")
	var commits []core.Commit
	err = iter.ForEach(func(c *object.Commit) error {
		if len(commits) >= limit {
			return stop
		}
		commits = append(commits, core.Commit{
			SHA:     c.Hash.String(),
			Subject: firstLine(c.Message),
			Author:  c.Author.Name,
			Time:    c.Author.When,
		})
		return nil
	})
	if err != nil && !errors.Is(err, stop) {
		return core.Snapshot{}, fmt.Errorf("walk log: %w", err)
	}

	return core.BuildSnapshot(commits, core.Events(eventsByCommit)), nil
}

// resolveBranch tries the remote-tracking branch first (refs/remotes/origin/X)
// and falls back to the local branch (refs/heads/X). The remote-tracking ref
// is preferred because the Source's fetch step keeps it current; the local
// branch reflects the user's checkout, which may be behind.
func resolveBranch(repo *gogit.Repository, branch string) (plumbing.Hash, error) {
	remote := plumbing.NewRemoteReferenceName("origin", branch)
	if ref, err := repo.Reference(remote, true); err == nil {
		return ref.Hash(), nil
	}
	local := plumbing.NewBranchReferenceName(branch)
	if ref, err := repo.Reference(local, true); err == nil {
		return ref.Hash(), nil
	}
	return plumbing.ZeroHash, fmt.Errorf("no ref found for branch %q (tried %s and %s)",
		branch, remote, local)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
