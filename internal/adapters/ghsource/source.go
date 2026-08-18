package ghsource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"time"

	"github.com/ezcdlabs/clarity/internal/cache"
	"github.com/ezcdlabs/clarity/internal/clock"
	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/core"
	"github.com/ezcdlabs/clarity/internal/gitlog"
)

// DefaultInterval is the poll cadence for the GitHub API. Slower than
// refsource's ~5s because the gh API costs more to hit and GitHub
// Actions runs change less frequently than git refs.
const DefaultInterval = 30 * time.Second

// DefaultLookback is how far back we fetch on first run / empty cache.
// Long enough to cover a full quarter of activity; the cache then keeps
// it warm for subsequent runs.
const DefaultLookback = 90 * 24 * time.Hour

// Options configures a Source.
type Options struct {
	RepoPath string
	RepoName string // stamped onto every emitted Snapshot
	Branch   string
	Limit    int
	// Mapping is the clarity.github section from .ezcd.json. At least
	// one of Mapping.CI / Mapping.Deploy must be non-nil; a mapping
	// where both are nil produces no events.
	Mapping *config.GitHubConfig
	// Cache persists raw runs to <cache-dir>/github-runs.json.gz so
	// repeated polls don't refetch the same data from GitHub.
	Cache *cache.File
	// Client is the GH API client. Tests inject a fake; production
	// uses NewCLIClient.
	Client GHClient
	// Interval defaults to DefaultInterval.
	Interval time.Duration
	// Lookback bounds the lookback window on first poll; defaults to
	// DefaultLookback.
	Lookback time.Duration
	// Clock defaults to clock.Real().
	Clock clock.Clock
	// Logger receives non-fatal diagnostic lines from the polling
	// loop — ListRuns failures, transient gh-CLI hiccups. nil ⇒
	// silent. The CLI plumbs this at a file in <cache-dir> so TUI
	// users can debug without the alt-screen rendering being
	// corrupted by mid-frame writes to stderr.
	Logger io.Writer
}

// Source is the GH-Actions Source adapter. Satisfies core.Source.
type Source struct {
	opts  Options
	cache map[int64]Run // run ID → run; loaded from disk + updated on poll
}

// New returns a configured Source. The cache file is loaded eagerly so
// the first poll can be incremental from where the last process left
// off.
func New(opts Options) (*Source, error) {
	if opts.RepoPath == "" {
		return nil, fmt.Errorf("ghsource: RepoPath is required")
	}
	if opts.Mapping == nil {
		return nil, fmt.Errorf("ghsource: Mapping is required")
	}
	if opts.Client == nil {
		return nil, fmt.Errorf("ghsource: Client is required")
	}
	if opts.Branch == "" {
		opts.Branch = "main"
	}
	if opts.Interval == 0 {
		opts.Interval = DefaultInterval
	}
	if opts.Lookback == 0 {
		opts.Lookback = DefaultLookback
	}
	if opts.Clock == nil {
		opts.Clock = clock.Real()
	}
	if opts.RepoName == "" {
		opts.RepoName = filepath.Base(opts.RepoPath)
	}
	if opts.Logger == nil {
		opts.Logger = io.Discard
	}
	s := &Source{opts: opts, cache: map[int64]Run{}}
	if opts.Cache != nil {
		s.loadCache()
	}
	return s, nil
}

// Watch starts a polling goroutine and returns a channel of snapshots.
// The first snapshot lands after the initial fetch (uses cache if
// present); each subsequent tick polls for runs updated since the
// previous high-water mark and emits a fresh Snapshot. Closes when
// ctx is cancelled.
func (s *Source) Watch(ctx context.Context) <-chan core.Snapshot {
	out := make(chan core.Snapshot, 1)
	go func() {
		defer close(out)

		s.pollOnce()
		if !s.emit(ctx, out) {
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.opts.Clock.After(s.opts.Interval):
			}
			s.pollOnce()
			if !s.emit(ctx, out) {
				return
			}
		}
	}()
	return out
}

// pollOnce updates s.cache with the latest runs for each mapped
// workflow. The `since` filter uses the cache's max(UpdatedAt) so we
// only ask GitHub for changes; first run with an empty cache uses
// Lookback as the window.
func (s *Source) pollOnce() {
	since := s.maxUpdatedAt()
	if since.IsZero() {
		since = time.Now().Add(-s.opts.Lookback)
	}
	for _, m := range s.mappings() {
		runs, err := s.opts.Client.ListRuns(m.Workflow, s.opts.Branch, since)
		if err != nil {
			// Polling errors are not fatal — keep the stale cache,
			// try again next tick. (A connectivity blip shouldn't
			// kill the TUI.) But do log them so the user can debug
			// "where are my events?" by tailing the log file.
			fmt.Fprintf(s.opts.Logger, "%s ListRuns(%q, %q): %v\n",
				time.Now().UTC().Format(time.RFC3339), m.Workflow, s.opts.Branch, err)
			continue
		}
		for _, r := range runs {
			s.cache[r.ID] = r
		}
	}
	if s.opts.Cache != nil {
		s.saveCache()
	}
}

// emit builds a Snapshot from the current cache + local git log and
// sends it. Returns false when ctx was cancelled mid-send so the
// caller exits.
func (s *Source) emit(ctx context.Context, out chan<- core.Snapshot) bool {
	commits, err := gitlog.Walk(s.opts.RepoPath, s.opts.Branch, s.opts.Limit)
	if err != nil {
		// Skip this tick; keep watching.
		return true
	}
	events := s.deriveAllEvents()
	snap := core.BuildSnapshot(commits, events)
	snap.RepoName = s.opts.RepoName
	select {
	case out <- snap:
		return true
	case <-ctx.Done():
		return false
	}
}

// deriveAllEvents walks the cached runs once per configured stage and
// merges the per-stage outputs into a single SHA-keyed map.
func (s *Source) deriveAllEvents() core.Events {
	runs := s.runsSlice()
	merged := core.Events{}
	mergeStage := func(stage string, m *config.StageMapping) {
		if m == nil {
			return
		}
		stageRuns := filterByWorkflow(runs, m.Workflow)
		for sha, evs := range DeriveEvents(stage, stageRuns, m.Jobs) {
			merged[sha] = append(merged[sha], evs...)
		}
	}
	if s.opts.Mapping != nil {
		mergeStage("ci", s.opts.Mapping.CI)
		mergeStage("deploy", s.opts.Mapping.Deploy)
	}
	// Per-SHA events sorted by Time so downstream lens / renderers
	// see a stable chronological order regardless of map iteration.
	for sha := range merged {
		evs := merged[sha]
		sort.SliceStable(evs, func(i, j int) bool { return evs[i].Time.Before(evs[j].Time) })
		merged[sha] = evs
	}
	return merged
}

// maxUpdatedAt returns the freshest UpdatedAt currently in cache, or
// the zero time when the cache is empty.
func (s *Source) maxUpdatedAt() time.Time {
	var max time.Time
	for _, r := range s.cache {
		if r.UpdatedAt.After(max) {
			max = r.UpdatedAt
		}
	}
	return max
}

// mappings returns the set of distinct stage mappings to poll for.
func (s *Source) mappings() []*config.StageMapping {
	var out []*config.StageMapping
	if s.opts.Mapping == nil {
		return nil
	}
	if s.opts.Mapping.CI != nil {
		out = append(out, s.opts.Mapping.CI)
	}
	if s.opts.Mapping.Deploy != nil && (s.opts.Mapping.CI == nil ||
		s.opts.Mapping.Deploy.Workflow != s.opts.Mapping.CI.Workflow) {
		// Skip duplicate workflow names — one ListRuns call covers
		// both stages when they share a workflow.
		out = append(out, s.opts.Mapping.Deploy)
	}
	return out
}

func (s *Source) runsSlice() []Run {
	out := make([]Run, 0, len(s.cache))
	for _, r := range s.cache {
		out = append(out, r)
	}
	return out
}

func filterByWorkflow(runs []Run, workflow string) []Run {
	out := make([]Run, 0, len(runs))
	for _, r := range runs {
		if r.Workflow == workflow {
			out = append(out, r)
		}
	}
	return out
}

// loadCache replaces s.cache with the contents of the on-disk file.
// Missing or corrupted cache is treated as empty — we'll repopulate on
// the first poll.
func (s *Source) loadCache() {
	data, exists, err := s.opts.Cache.Read()
	if err != nil || !exists {
		return
	}
	var runs []Run
	if err := json.Unmarshal(data, &runs); err != nil {
		return
	}
	for _, r := range runs {
		s.cache[r.ID] = r
	}
}

// saveCache atomically writes the in-memory cache to disk so the next
// process starts warm. Errors are swallowed — the network call is the
// expensive part; the cache is an optimization.
func (s *Source) saveCache() {
	runs := s.runsSlice()
	data, err := json.Marshal(runs)
	if err != nil {
		return
	}
	_ = s.opts.Cache.Write(data)
}
