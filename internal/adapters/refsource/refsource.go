// Package refsource is the Source adapter that reads commits and
// clarity events from a local git repository's remote-tracking refs.
// It polls the remote with `git ls-remote`, fetches when the branch
// tip or events ref has moved, and emits a core.Snapshot per change.
//
// Implements core.Source.
package refsource

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/clock"
	"github.com/ezcdlabs/clarity/internal/core"
	"github.com/ezcdlabs/clarity/internal/gitenv"
	"github.com/ezcdlabs/clarity/internal/refs"
)

// Options configures a Source.
type Options struct {
	RepoPath string
	Remote   string        // defaults to "origin"
	Branch   string        // defaults to "main"
	Interval time.Duration // defaults to 5s
	Limit    int           // max commits per snapshot; defaults to 50
	Clock    clock.Clock   // defaults to clock.Real()
}

// Source polls the configured remote and emits a Snapshot whenever the
// branch tip or events ref changes. Satisfies core.Source.
type Source struct {
	opts Options
}

// New returns a configured Source. As a side effect it registers the
// clarity fetch refspec on the named remote (idempotent; no-op when the
// remote hasn't published any events yet) so subsequent `git fetch`
// invocations carry the events ref without any per-call refspec flags.
func New(opts Options) (*Source, error) {
	if opts.Remote == "" {
		opts.Remote = "origin"
	}
	if opts.Branch == "" {
		opts.Branch = "main"
	}
	if opts.Interval == 0 {
		opts.Interval = 5 * time.Second
	}
	if opts.Limit == 0 {
		opts.Limit = 50
	}
	if opts.Clock == nil {
		opts.Clock = clock.Real()
	}
	if err := refs.EnsureClarityFetchRefspec(opts.RepoPath, opts.Remote); err != nil {
		return nil, fmt.Errorf("configure clarity fetch refspec: %w", err)
	}
	return &Source{opts: opts}, nil
}

// Watch starts a polling goroutine and returns a channel of snapshots.
// The first snapshot is emitted immediately after the initial fetch;
// subsequent snapshots are only emitted when the branch tip or events
// ref has moved on the remote. The channel is closed when ctx is
// cancelled.
func (s *Source) Watch(ctx context.Context) <-chan core.Snapshot {
	out := make(chan core.Snapshot, 1)
	go func() {
		defer close(out)

		branchRemoteRef := "refs/heads/" + s.opts.Branch
		watched := []string{branchRemoteRef, clarityrefs.EventsRef}

		_ = fetchAll(s.opts.RepoPath, s.opts.Remote, s.opts.Branch)
		if !s.emit(ctx, out) {
			return
		}
		lastRefs, _ := lsRemote(s.opts.RepoPath, s.opts.Remote, watched...)

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.opts.Clock.After(s.opts.Interval):
			}

			current, err := lsRemote(s.opts.RepoPath, s.opts.Remote, watched...)
			if err != nil {
				continue
			}
			if refsEqual(current, lastRefs) {
				continue
			}

			_ = fetchAll(s.opts.RepoPath, s.opts.Remote, s.opts.Branch)
			if !s.emit(ctx, out) {
				return
			}
			lastRefs = current
		}
	}()
	return out
}

func (s *Source) emit(ctx context.Context, out chan<- core.Snapshot) bool {
	snap, err := BuildSnapshot(s.opts.RepoPath, s.opts.Branch, s.opts.Limit)
	if err != nil {
		return true
	}
	select {
	case out <- snap:
		return true
	case <-ctx.Done():
		return false
	}
}

// fetchAll updates the remote-tracking branch and the clarity events ref.
// The branch and events ref are fetched in separate invocations because
// `git fetch` with multiple refspecs aborts the whole command if any one
// of them is missing on the remote (common until the first event is reported).
func fetchAll(repoPath, remote, branch string) error {
	if err := runFetch(repoPath, remote,
		"+refs/heads/"+branch+":refs/remotes/"+remote+"/"+branch); err != nil {
		return fmt.Errorf("fetch branch: %w", err)
	}
	_ = runFetch(repoPath, remote, "+"+clarityrefs.EventsRef+":"+clarityrefs.EventsRef)
	return nil
}

func runFetch(repoPath, remote, refspec string) error {
	cmd := exec.Command("git", "fetch", remote, refspec)
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "couldn't find remote ref") {
			return nil
		}
		return fmt.Errorf("fetch: %w\n%s", err, out)
	}
	return nil
}

// lsRemote returns refname → SHA for the requested refs as the remote
// currently sees them. Missing refs are simply absent from the result.
func lsRemote(repoPath, remote string, refs ...string) (map[string]string, error) {
	args := append([]string{"ls-remote", remote}, refs...)
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			result[fields[1]] = fields[0]
		}
	}
	return result, nil
}

func refsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
