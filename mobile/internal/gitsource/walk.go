package gitsource

import (
	"errors"
	"fmt"

	"github.com/ezcdlabs/clarity/internal/core"
	"github.com/ezcdlabs/clarity/internal/gitlog"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"strings"
)

// errStopWalk ends an iteration early without being a failure.
var errStopWalk = errors.New("stop")

// walk returns commits newest-first, and whether history continues past what
// was returned.
//
// It must never let go-git walk freely. A shallow fetch leaves a graft: the
// oldest commit claims parents that were never downloaded, and go-git does
// not read the boundary — Log walks straight through it and fails the whole
// iteration with "object not found", which is precisely the failure that made
// the CLI stop reading through go-git at all.
//
// The difference here is that go-git *records* the boundary even though it
// ignores it, so the client that asked for the depth can be told where the
// history stops. Stopping there turns the same situation from an error into
// an answer.
func (r *Repo) walk(limit int) ([]core.Commit, bool, error) {
	limit = gitlog.Resolve(limit)

	tip, err := r.tip()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			// Nothing fetched yet, or a branch the remote does not have.
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("resolve %s: %w", r.branch, err)
	}

	grafts, err := r.grafts()
	if err != nil {
		return nil, false, err
	}

	iter, err := r.repo.Log(&gogit.LogOptions{From: tip.Hash})
	if err != nil {
		return nil, false, fmt.Errorf("log: %w", err)
	}
	defer iter.Close()

	var (
		commits   []core.Commit
		hitLimit  bool
		hitBottom bool
	)
	err = iter.ForEach(func(c *object.Commit) error {
		if len(commits) >= limit {
			hitLimit = true
			return errStopWalk
		}
		commits = append(commits, core.Commit{
			SHA:     c.Hash.String(),
			Subject: firstLine(c.Message),
			Author:  c.Author.Name,
			Time:    c.Author.When,
		})
		if grafts[c.Hash] {
			// The parents of this commit were never fetched. Stop before
			// asking for them.
			hitBottom = true
			return errStopWalk
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopWalk) {
		return nil, false, fmt.Errorf("walk log: %w", err)
	}

	// Either way the history on the remote continues past what is held: the
	// limit cut it, or the depth did. Saying so is what stops an aggregate
	// built from a partial window being read as the whole repository.
	return commits, hitLimit || hitBottom, nil
}

// grafts is the set of commits whose parents were never fetched.
func (r *Repo) grafts() (map[plumbing.Hash]bool, error) {
	hashes, err := r.repo.Storer.Shallow()
	if err != nil {
		return nil, fmt.Errorf("read shallow boundary: %w", err)
	}
	out := make(map[plumbing.Hash]bool, len(hashes))
	for _, h := range hashes {
		out[h] = true
	}
	return out, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
