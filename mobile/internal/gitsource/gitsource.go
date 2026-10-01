// Package gitsource reads a repository the client fetched itself.
//
// The CLI is handed a working copy made by someone else's checkout — a
// partial clone, a shared object store, a shallow graft — and cannot know
// which, so it reads through the git binary, which does. A mobile client has
// no git binary and no checkout, but it also has no such problem: it creates
// the store, chooses the refspecs and asks for the depth, so the shape is the
// one it asked for. That is what makes go-git the right tool here and the
// wrong one there.
//
// Nothing is checked out. Two targeted fetches — the events ref in full,
// because it is small and every blob is wanted, and the branch at a depth —
// then everything is read out of the object store.
package gitsource

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/core"
	gogit "github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage"

	"github.com/go-git/go-billy/v5/memfs"
)

// DefaultDepth is how much history a sync fetches when none is given.
// Deliberately more than a status view needs: the weekly metrics want several
// whole weeks, and a fetch is cheap compared to asking the user to wait again.
const DefaultDepth = 200

// Repo is one tracked repository: an object store and the remote it syncs
// from. There is no working tree.
type Repo struct {
	remote string
	branch string
	repo   *gogit.Repository
}

// Open prepares a repository over the given storage. The storage may be
// in-memory or file-backed; nothing here assumes either.
func Open(st storage.Storer, remote, branch string) (*Repo, error) {
	if remote == "" {
		return nil, errors.New("remote is required")
	}
	if branch == "" {
		branch = config.DefaultBranch
	}

	repo, err := gogit.Init(st, memfs.New())
	if errors.Is(err, gogit.ErrRepositoryAlreadyExists) {
		repo, err = gogit.Open(st, memfs.New())
	}
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	// Idempotent: reopening an existing store must not fail on the remote
	// already being there, and the URL may legitimately have changed.
	if _, err := repo.Remote(gogit.DefaultRemoteName); err != nil {
		if _, err := repo.CreateRemote(&gogitconfig.RemoteConfig{
			Name: gogit.DefaultRemoteName,
			URLs: []string{remote},
		}); err != nil {
			return nil, fmt.Errorf("configure remote: %w", err)
		}
	}
	return &Repo{remote: remote, branch: branch, repo: repo}, nil
}

// SyncOptions configures one fetch.
type SyncOptions struct {
	// Depth bounds the branch history fetched. Zero means DefaultDepth; a
	// negative value means unbounded, which is almost never what a phone
	// wants.
	Depth int
	Auth  transport.AuthMethod
}

// Sync fetches what the views need and nothing else.
func (r *Repo) Sync(ctx context.Context, opts SyncOptions) error {
	depth := opts.Depth
	if depth == 0 {
		depth = DefaultDepth
	}
	if depth < 0 {
		depth = 0 // go-git reads 0 as "no limit"
	}

	// The events ref whole. It holds one small JSON file per event, and the
	// views read every one, so there is nothing to gain by bounding it — and
	// a depth on this ref would cut off history the weekly view needs.
	err := r.repo.FetchContext(ctx, &gogit.FetchOptions{
		RemoteName: gogit.DefaultRemoteName,
		RefSpecs:   []gogitconfig.RefSpec{refspec(clarityrefs.EventsRef)},
		Auth:       opts.Auth,
	})
	if err := ignoreBenign(err); err != nil {
		return fmt.Errorf("fetch %s: %w", clarityrefs.EventsRef, err)
	}

	branchRef := "refs/heads/" + r.branch
	err = r.repo.FetchContext(ctx, &gogit.FetchOptions{
		RemoteName: gogit.DefaultRemoteName,
		RefSpecs:   []gogitconfig.RefSpec{refspec(branchRef)},
		Depth:      depth,
		Auth:       opts.Auth,
	})
	if err := ignoreBenign(err); err != nil {
		return fmt.Errorf("fetch %s: %w", branchRef, err)
	}
	return nil
}

func refspec(ref string) gogitconfig.RefSpec {
	return gogitconfig.RefSpec("+" + ref + ":" + ref)
}

// ignoreBenign filters the outcomes that are not failures: nothing new to
// fetch, and a ref the remote does not have. A repo that has never reported
// has no events ref, which is its ordinary state rather than an error.
func ignoreBenign(err error) error {
	switch {
	case err == nil,
		errors.Is(err, gogit.NoErrAlreadyUpToDate),
		errors.Is(err, transport.ErrEmptyRemoteRepository):
		return nil
	}
	// go-git reports a missing ref as a couldn't-find-remote-ref error whose
	// type is not exported, so the message is the only handle. It carries the
	// ref name, so this has to be a substring match — an equality check
	// silently never fires, and every repo that had not reported yet would
	// have failed to sync.
	if strings.Contains(err.Error(), "couldn't find remote ref") ||
		errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil
	}
	return err
}

// Config reads .ezcd.json out of the branch tip, with no checkout.
//
// A repo with no config file is not an error — it is most repos — and parses
// to the defaults.
func (r *Repo) Config() (config.Config, error) {
	tip, err := r.tip()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return config.Defaults(), nil
		}
		return config.Config{}, err
	}
	f, err := tip.File(config.FileName)
	if err != nil {
		return config.Defaults(), nil
	}
	body, err := f.Contents()
	if err != nil {
		return config.Config{}, fmt.Errorf("read %s: %w", config.FileName, err)
	}
	return config.Parse([]byte(body))
}

// Snapshot joins the branch history to the events ref, exactly as the CLI's
// source does — through the same core constructor, so the two cannot drift in
// what a snapshot means.
func (r *Repo) Snapshot(limit int) (core.Snapshot, error) {
	commits, more, err := r.walk(limit)
	if err != nil {
		return core.Snapshot{}, err
	}

	files, err := r.eventsFiles()
	if err != nil {
		return core.Snapshot{}, err
	}
	events, scope := clarityrefs.ParseRefFiles(files)

	snap := core.BuildSnapshotWithScope(commits, core.Events(events), core.ScopeBySHA(scope))
	snap.Truncated = more
	snap.Limit = limit
	return snap, nil
}

func (r *Repo) tip() (*object.Commit, error) {
	ref, err := r.repo.Reference(plumbing.ReferenceName("refs/heads/"+r.branch), true)
	if err != nil {
		return nil, err
	}
	return r.repo.CommitObject(ref.Hash())
}

// eventsFiles flattens the events ref tree to path → content. An absent ref
// is a repo that has reported nothing yet, which is empty rather than an
// error.
func (r *Repo) eventsFiles() (map[string][]byte, error) {
	files := map[string][]byte{}

	ref, err := r.repo.Reference(plumbing.ReferenceName(clarityrefs.EventsRef), true)
	if err != nil {
		return files, nil
	}
	commit, err := r.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", clarityrefs.EventsRef, err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("read %s tree: %w", clarityrefs.EventsRef, err)
	}
	err = tree.Files().ForEach(func(f *object.File) error {
		content, err := f.Contents()
		if err != nil {
			return err
		}
		files[f.Name] = []byte(content)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read %s files: %w", clarityrefs.EventsRef, err)
	}
	return files, nil
}
