// Package watcher polls a git remote and builds commit-centric snapshots
// joining the local branch log to clarity events. It is the data layer the
// TUI subscribes to.
package watcher

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

// Snapshot is re-exported from core for backwards compatibility while the
// adapter migration is in progress; it will be removed in step 3 when
// watcher itself moves under internal/adapters/refsource.
type Snapshot = core.Snapshot

// CommitView same — re-exported alias during the migration.
type CommitView = core.CommitView

// BuildSnapshot walks up to limit commits starting from the given branch and
// joins each one to its events from the local clarity events ref. Pure: does
// not fetch from the remote — callers are responsible for that.
func BuildSnapshot(repoPath, branch string, limit int) (Snapshot, error) {
	if limit <= 0 {
		limit = 50
	}
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return Snapshot{}, fmt.Errorf("open repo: %w", err)
	}

	hash, err := resolveBranch(repo, branch)
	if err != nil {
		return Snapshot{}, err
	}

	eventsByCommit, err := clarityrefs.ReadAllEvents(repoPath)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read events: %w", err)
	}

	iter, err := repo.Log(&gogit.LogOptions{From: hash})
	if err != nil {
		return Snapshot{}, fmt.Errorf("log: %w", err)
	}

	var stop = errors.New("limit reached")
	var commits []CommitView
	err = iter.ForEach(func(c *object.Commit) error {
		if len(commits) >= limit {
			return stop
		}
		sha := c.Hash.String()
		commits = append(commits, CommitView{
			SHA:     sha,
			Subject: firstLine(c.Message),
			Author:  c.Author.Name,
			Time:    c.Author.When,
			Events:  eventsByCommit[sha],
		})
		return nil
	})
	if err != nil && !errors.Is(err, stop) {
		return Snapshot{}, fmt.Errorf("walk log: %w", err)
	}

	return Snapshot{Commits: commits}, nil
}

// resolveBranch tries the remote-tracking branch first (refs/remotes/origin/X)
// and falls back to the local branch (refs/heads/X). The remote-tracking ref
// is preferred because the watcher's fetch step keeps it current; the local
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
