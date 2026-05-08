package watcher

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/clock"
	"github.com/ezcdlabs/clarity/internal/gitenv"
)

// Options configures a Watch session.
type Options struct {
	RepoPath string
	Remote   string        // defaults to "origin"
	Branch   string        // defaults to "main"
	Interval time.Duration // defaults to 5s
	Limit    int           // max commits per snapshot; defaults to 50
	Clock    clock.Clock   // defaults to clock.Real()
}

// Watch starts a polling goroutine and returns a channel of snapshots. The
// first snapshot is emitted immediately after a fetch; subsequent snapshots
// are only emitted when the branch tip or events ref has moved on the remote.
// The channel is closed when ctx is cancelled.
func Watch(ctx context.Context, opts Options) <-chan Snapshot {
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

	out := make(chan Snapshot, 1)
	go func() {
		defer close(out)

		branchRemoteRef := "refs/heads/" + opts.Branch
		watched := []string{branchRemoteRef, clarityrefs.EventsRef}

		// Initial: fetch + emit, regardless of whether anything moved.
		_ = fetchAll(opts.RepoPath, opts.Remote, opts.Branch)
		if !emit(ctx, opts, out) {
			return
		}
		lastRefs, _ := lsRemote(opts.RepoPath, opts.Remote, watched...)

		for {
			select {
			case <-ctx.Done():
				return
			case <-opts.Clock.After(opts.Interval):
			}

			current, err := lsRemote(opts.RepoPath, opts.Remote, watched...)
			if err != nil {
				continue
			}
			if refsEqual(current, lastRefs) {
				continue
			}

			_ = fetchAll(opts.RepoPath, opts.Remote, opts.Branch)
			if !emit(ctx, opts, out) {
				return
			}
			lastRefs = current
		}
	}()
	return out
}

// emit builds a snapshot and sends it on out. Returns false if ctx was
// cancelled while sending (caller should exit).
func emit(ctx context.Context, opts Options, out chan<- Snapshot) bool {
	snap, err := BuildSnapshot(opts.RepoPath, opts.Branch, opts.Limit)
	if err != nil {
		return true // skip this cycle but keep watching
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
	// Events ref is best-effort: the ref may not exist yet on a fresh repo.
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
