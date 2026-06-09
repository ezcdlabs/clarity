// Package gitlog is a small shared helper for the commit-walk step
// every local-git Source adapter needs. refsource and ghsource both
// read commits from the same local repository — the only difference is
// where they get their events from. This package owns the gogit details
// so neither adapter has to.
package gitlog

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ezcdlabs/clarity/internal/core"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// DefaultLimit is the commit-count cap used when callers pass limit <= 0.
// Kept aligned with refsource's pre-extraction default so behaviour is
// unchanged.
const DefaultLimit = 50

// Walk walks up to limit commits starting from the given branch and
// returns them newest-first. The branch is resolved against the local
// repository: the remote-tracking ref (refs/remotes/origin/<branch>) is
// preferred because the Source's fetch step keeps it current; the local
// branch ref is the fallback. Callers are responsible for fetching
// before calling — Walk does not touch the network.
func Walk(repoPath, branch string, limit int) ([]core.Commit, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	hash, err := resolveBranch(repo, branch)
	if err != nil {
		return nil, err
	}

	iter, err := repo.Log(&gogit.LogOptions{From: hash})
	if err != nil {
		return nil, fmt.Errorf("log: %w", err)
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
		return nil, fmt.Errorf("walk log: %w", err)
	}
	return commits, nil
}

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
