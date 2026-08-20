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

// Resolve returns the commit cap Walk will actually apply for limit,
// substituting DefaultLimit for a non-positive one. Exported because
// callers record the cap alongside the snapshot they build: resolving it
// themselves keeps the number they report identical to the number the walk
// used, instead of a raw 0 the renderer would go on to quote back at the
// user as "--limit 0 reached".
func Resolve(limit int) int {
	if limit <= 0 {
		return DefaultLimit
	}
	return limit
}

// Walk walks up to limit commits starting from the given branch and
// returns them newest-first. The branch is resolved against the local
// repository: the remote-tracking ref (refs/remotes/origin/<branch>) is
// preferred because the Source's fetch step keeps it current; the local
// branch ref is the fallback. Callers are responsible for fetching
// before calling — Walk does not touch the network.
//
// The second return reports whether the history continues past the limit.
// It cannot be inferred from the returned slice — `len(commits) == limit`
// is equally true of a repo that ends there and one with a thousand more
// commits below the cut — so the walk steps one commit past the limit to
// find out. That commit is not included in the result; the extra step is
// what the renderers use to tell "this is all of it" apart from "your
// --limit stopped here".
func Walk(repoPath, branch string, limit int) ([]core.Commit, bool, error) {
	limit = Resolve(limit)
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, false, fmt.Errorf("open repo: %w", err)
	}

	hash, err := resolveBranch(repo, branch)
	if err != nil {
		return nil, false, err
	}

	iter, err := repo.Log(&gogit.LogOptions{From: hash})
	if err != nil {
		return nil, false, fmt.Errorf("log: %w", err)
	}

	var stop = errors.New("limit reached")
	var commits []core.Commit
	more := false
	err = iter.ForEach(func(c *object.Commit) error {
		if len(commits) >= limit {
			// Reaching this callback at all means a commit exists past the
			// limit. Record that and stop without collecting it.
			more = true
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
		return nil, false, fmt.Errorf("walk log: %w", err)
	}
	return commits, more, nil
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
