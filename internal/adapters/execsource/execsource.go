// Package execsource is the Source adapter that reads clarity events from
// a configured local command instead of the events ref — the pattern of
// git credential helpers and kubectl exec plugins. Clarity stays
// transport-agnostic and server-free; each team brings a helper for its
// own store, and everything the helper prints is what clarity treats as
// true, because only the helper knows its store's writers.
//
// Protocol v1, one-shot and polled: the command is exec'd directly with
// its configured argument list plus `--protocol 1` — there is no shell,
// ever. stdin carries the trunk commit shas clarity is about to display,
// one per line, newest first, so the helper needs no git knowledge and
// its query stays bounded. stdout is one event JSON object per line
// ({"sha","stage","status","ts","target","ci"}); exit 0 means the output
// is complete; anything else — a non-zero exit, a timeout, unparseable
// output — is a failed read. A failed poll keeps the last good snapshot
// on screen marked Degraded, with the helper's stderr as the reason,
// instead of the silent watcher it would otherwise be.
//
// The command comes from shared configuration, so it never runs without
// this clone's human having approved the exact argv — see internal/trust.
// That gate is the caller's (buildSource checks before constructing this
// adapter); nothing in this package consults trust, so nothing in it may
// be reachable from an unapproved config.
//
// Implements core.Source and checkcmd.TrunkReader.
package execsource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/clock"
	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/core"
	"github.com/ezcdlabs/clarity/internal/gitenv"
	"github.com/ezcdlabs/clarity/internal/gitlog"
	"github.com/ezcdlabs/clarity/internal/trust"
)

// Options configures a Source.
type Options struct {
	RepoPath string
	Remote   string        // defaults to "origin"
	Branch   string        // defaults to "main"
	RepoName string        // defaults to filepath.Base(RepoPath)
	Interval time.Duration // defaults to 5s
	Limit    int           // max commits per snapshot; defaults to 50
	Clock    clock.Clock   // defaults to clock.Real()
	// Argv is the helper command, exec'd exactly as given. Required.
	Argv []string
	// Timeout bounds one helper run. Defaults to config.DefaultExecTimeout.
	Timeout time.Duration
}

// Source polls by running the helper and emits a Snapshot per run.
// Satisfies core.Source.
type Source struct {
	opts Options
	// lastGood is the newest snapshot a successful run produced, re-emitted
	// with a Degraded reason while the helper is failing.
	lastGood *core.Snapshot
}

// New returns a configured Source. The caller has already checked trust;
// an empty argv is a programming error, not a config state, and is
// refused.
func New(opts Options) (*Source, error) {
	if len(opts.Argv) == 0 {
		return nil, fmt.Errorf("exec source: empty command")
	}
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
	if opts.Timeout == 0 {
		opts.Timeout = config.DefaultExecTimeout
	}
	if opts.Clock == nil {
		opts.Clock = clock.Real()
	}
	if opts.RepoName == "" {
		opts.RepoName = filepath.Base(opts.RepoPath)
	}
	return &Source{opts: opts}, nil
}

// Validate pre-flights the source: the branch must resolve, and the
// helper must complete one real run. "Degrade quietly" only holds once a
// first state exists — a helper that cannot run at all must fail here,
// loudly, with its stderr, rather than hang a blank dashboard emitting
// nothing.
func (s *Source) Validate(ctx context.Context) error {
	_ = fetchBranch(s.opts.RepoPath, s.opts.Remote, s.opts.Branch)
	commits, _, err := gitlog.Walk(s.opts.RepoPath, s.opts.Branch, gitlog.Resolve(s.opts.Limit))
	if err != nil {
		return err
	}
	if _, err := s.runHelper(ctx, shasOf(commits)); err != nil {
		return fmt.Errorf("exec source %s: %w", trust.Display(s.opts.Argv), err)
	}
	return nil
}

// Watch runs the helper on each poll and emits the result. Unlike the
// ref source there is no cheap "did anything move?" probe — the helper's
// store can change without any git ref moving — so every tick runs it
// and emits. The branch is still only fetched when ls-remote says its
// tip moved, because the fetch is the expensive half.
func (s *Source) Watch(ctx context.Context) <-chan core.Snapshot {
	out := make(chan core.Snapshot, 1)
	go func() {
		defer close(out)

		_ = fetchBranch(s.opts.RepoPath, s.opts.Remote, s.opts.Branch)
		lastTip, _ := branchTip(s.opts.RepoPath, s.opts.Remote, s.opts.Branch)
		if !s.emit(ctx, out) {
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.opts.Clock.After(s.opts.Interval):
			}
			if tip, err := branchTip(s.opts.RepoPath, s.opts.Remote, s.opts.Branch); err == nil && tip != lastTip {
				_ = fetchBranch(s.opts.RepoPath, s.opts.Remote, s.opts.Branch)
				lastTip = tip
			}
			if !s.emit(ctx, out) {
				return
			}
		}
	}()
	return out
}

// emit runs one poll and sends its outcome: the fresh snapshot, or the
// last good one marked Degraded when the run failed. Before any first
// state exists the commits still render — all "before CI" — under the
// same marker, which beats a blank screen and cannot be mistaken for a
// healthy empty repo.
func (s *Source) emit(ctx context.Context, out chan<- core.Snapshot) bool {
	snap, err := s.snapshot(ctx)
	if err == nil {
		s.lastGood = &snap
	} else {
		if s.lastGood != nil {
			snap = *s.lastGood
		} else if commits, more, walkErr := gitlog.Walk(s.opts.RepoPath, s.opts.Branch, gitlog.Resolve(s.opts.Limit)); walkErr == nil {
			snap = core.BuildSnapshotWithScope(commits, nil, nil)
			snap.RepoName = s.opts.RepoName
			snap.Truncated = more
			snap.Limit = gitlog.Resolve(s.opts.Limit)
		} else {
			return true // nothing renderable at all; try again next tick
		}
		snap.Degraded = oneLine(err.Error())
	}
	select {
	case out <- snap:
		return true
	case <-ctx.Done():
		return false
	}
}

// Once fetches, runs the helper once, and returns the snapshot or the
// error — the strict form `git clarity check` reads through, where a
// failure must come back as "could not tell" rather than as stale data.
func (s *Source) Once() (core.Snapshot, error) {
	if err := fetchBranch(s.opts.RepoPath, s.opts.Remote, s.opts.Branch); err != nil {
		return core.Snapshot{}, err
	}
	return s.snapshot(context.Background())
}

// snapshot walks the branch and joins the helper's events onto it.
func (s *Source) snapshot(ctx context.Context) (core.Snapshot, error) {
	limit := gitlog.Resolve(s.opts.Limit)
	commits, more, err := gitlog.Walk(s.opts.RepoPath, s.opts.Branch, limit)
	if err != nil {
		return core.Snapshot{}, err
	}
	events, err := s.runHelper(ctx, shasOf(commits))
	if err != nil {
		return core.Snapshot{}, err
	}
	snap := core.BuildSnapshotWithScope(commits, core.Events(events), nil)
	snap.RepoName = s.opts.RepoName
	snap.Truncated = more
	snap.Limit = limit
	return snap, nil
}

// helperLine is one protocol-v1 stdout line.
type helperLine struct {
	SHA    string            `json:"sha"`
	Stage  string            `json:"stage"`
	Status string            `json:"status"`
	Ts     int64             `json:"ts"`
	Target string            `json:"target"`
	CI     map[string]string `json:"ci"`
}

// runHelper performs one protocol run: shas in, events out.
func (s *Source) runHelper(ctx context.Context, shas []string) (map[string][]clarityrefs.Event, error) {
	ctx, cancel := context.WithTimeout(ctx, s.opts.Timeout)
	defer cancel()

	argv := append(append([]string{}, s.opts.Argv...), "--protocol", "1")
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = s.opts.RepoPath
	cmd.Stdin = strings.NewReader(strings.Join(shas, "\n") + "\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// The deadline must bind even when the helper leaves a child holding
	// its pipes: killing the helper alone leaves Wait blocked on the
	// orphan's copy of stdout, and a "timed out" that takes as long as the
	// hang it reports is no timeout at all.
	cmd.WaitDelay = time.Second

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("helper timed out after %s%s", s.opts.Timeout, stderrSuffix(stderr))
	}
	if err != nil {
		return nil, fmt.Errorf("helper failed: %w%s", err, stderrSuffix(stderr))
	}

	events := map[string][]clarityrefs.Event{}
	for i, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var raw helperLine
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			return nil, fmt.Errorf("helper output line %d is not an event: %v (%s)", i+1, err, excerpt(line))
		}
		if raw.SHA == "" || raw.Stage == "" || raw.Status == "" || raw.Ts <= 0 {
			return nil, fmt.Errorf("helper output line %d is missing sha, stage, status or ts (%s)", i+1, excerpt(line))
		}
		events[raw.SHA] = append(events[raw.SHA], clarityrefs.Event{
			Stage:  raw.Stage,
			Status: raw.Status,
			Time:   time.Unix(raw.Ts, 0),
			Target: raw.Target,
			CI:     raw.CI,
		})
	}
	return events, nil
}

func shasOf(commits []core.Commit) []string {
	shas := make([]string, 0, len(commits))
	for _, c := range commits {
		shas = append(shas, c.SHA)
	}
	return shas
}

// stderrSuffix renders captured stderr for inclusion in an error, bounded
// and flattened — it ends up on a Degraded one-liner.
func stderrSuffix(stderr bytes.Buffer) string {
	msg := oneLine(strings.TrimSpace(stderr.String()))
	if msg == "" {
		return ""
	}
	return ": " + excerpt(msg)
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
}

func excerpt(s string) string {
	const max = 300
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// fetchBranch updates the remote-tracking branch. Best-effort callers
// ignore the error; Once propagates it.
func fetchBranch(repoPath, remote, branch string) error {
	cmd := exec.Command("git", "fetch", remote,
		"+refs/heads/"+branch+":refs/remotes/"+remote+"/"+branch)
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("fetch %s: %w\n%s", branch, err, gitenv.Redact(string(out)))
	}
	return nil
}

// branchTip reads the remote's current tip for the branch, the cheap
// "did anything move?" probe that decides whether to fetch.
func branchTip(repoPath, remote, branch string) (string, error) {
	cmd := exec.Command("git", "ls-remote", remote, "refs/heads/"+branch)
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
