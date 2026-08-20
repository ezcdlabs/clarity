# Architecture refactor — hexagonal core, pluggable sources/renderers

**Status: steps 1–8 have landed; steps 9–10 have not.** The port shape, the
core/adapter split, `CachedLens` + `View.Stale`, the `.ezcd.json` loader, the
ghsource adapter and `git clarity init --github` are all in the tree. For how
those behave today, read the code and README.md — this doc is the reasoning
that produced them, kept for the trade-offs it records, and it is not
maintained as a description of the current system.

Still a target: `copy-events` + the `EventWriter` adapter (step 9) and the web
renderer / Docker forwarder (step 10). See [Sequencing](#sequencing-work-plan).

Where the implementation chose differently from the plan, the section says so
inline.

## Motivation

Clarity's actual product is the **lens** — the transformation from a stream
of pipeline events into the HEAD / CI Passed / Deployed grouping, batch
subheaders, fix-forward semantics, lead times, and weekly DORA throughput.
The git-ref storage model is one ingestion strategy among several; framing
the project as "events on a git ref" understates what's actually valuable.

Today the lens lives in `internal/tui` next to bubble-tea rendering code,
which:

- Couples the data-derivation logic to the TUI's package
- Makes it awkward to add a second event source (GitHub Actions API)
- Makes it awkward to add alternative renderers (web UI)
- Makes it awkward to copy events between event stores (e.g. GitHub
  Actions API → `refs/clarity/events` for backfill)

The refactor moves the lens into its own package, defines clean ports for
the things the core depends on, and lets every external system (sources,
renderers) be a pluggable adapter.

## Target architecture — hexagonal / ports & adapters

```
   ┌──────────────────────────┐   Source.Watch    ┌──────────────────────┐   Renderer.Render   ┌──────────┐
   │   Source adapter         │                   │       Lens           │                     │ Renderer │
   │   (refsource / ghsource) │                   │   (in core)          │                     │ adapter  │
   │                          │                   │                      │                     │          │
   │   poll / fetch           │                   │   for each Snapshot: │                     │          │
   │   ↓                      │                   │     DeriveView →     │                     │          │
   │   commits + events       │                   │     View             │                     │          │
   │   ↓                      │ ────Snapshot────► │   send on Views chan │ ──── View ────────► │  bytes   │
   │   core.BuildSnapshot     │                   │                      │                     │          │
   │   ↓                      │                   │                      │                     │          │
   │   Snapshot               │                   │                      │                     │          │
   └──────────────────────────┘                   └──────────────────────┘                     └──────────┘

                     ←─── core (BuildSnapshot here) ───┤                  ├─── core (Lens + DeriveView) ───→
```

The two pure cores sit **on either side of the `Source` port**:

1. **`BuildSnapshot(commits, events)`** is called by the **adapter** to
   produce what the Source port emits. The join from raw inputs to
   `Snapshot` is identical regardless of where commits and events came
   from, so it lives in core and every adapter imports it.
2. **`DeriveView(snapshot)`** is called by the **Lens** to consume what
   the Source port emits. The Lens then sends the resulting `View`
   through the Renderer port.

So the Source port itself carries `Snapshot` values — exactly
`BuildSnapshot`'s output and `DeriveView`'s input. The port is the
boundary between the two cores, not an input to either.

Both functions are pure: no `os`, no `net`, no `bubbletea`, no `gogit`.
Adapters do the I/O; cores do the data transforms.

`EventWriter` is an **auxiliary** outbound type used only by
`git clarity copy-events` (see below) — never imported by `internal/core`.
Writing events somewhere is a separate verb from rendering a view; the
two don't share a code path.

## Ports

### Inbound

```go
type Source interface {
    // Watch starts the adapter's polling lifecycle and returns a channel
    // of Snapshots. The first snapshot lands after the adapter's initial
    // fetch; subsequent ones arrive on the adapter's own cadence whenever
    // it detects a change. Closes when ctx is cancelled.
    Watch(ctx context.Context) <-chan Snapshot
}
```

**One** port for the data the core consumes — not two. Each adapter
owns:

- its own polling cadence (refsource ~5s; ghsource ~30s — the appropriate
  rate for each underlying system),
- its own fetch verb (refsource fetches refs; ghsource hits the GH API),
- the join from raw commits + events into a `Snapshot` (via
  `core.BuildSnapshot`).

The Lens is not responsible for orchestrating fetches or merging
multi-channel updates. Polling is the adapter's concern; the adapter
emits Snapshots when something has actually changed.

For two adapter implementations that both happen to want commits from
the local git repo, a small shared utility helper (`internal/gitlog`) is
imported — not a port, just shared code.

### Outbound

```go
type Renderer interface {
    Render(ctx context.Context, views <-chan View) error
}
```

Same `Renderer` signature for TUI, plain text, web, JSON — adapters choose
how long they consume the channel (TUI blocks; plain reads one and exits).

### Auxiliary (not used by the core)

```go
type EventWriter interface {
    Write(ctx context.Context, events Events) error
}
```

Defined alongside the core ports for type-symmetry, but not depended on
by `internal/core`. Used only by the `copy-events` subcommand. Lives in
a separate file (`internal/core/aux.go` or similar) so the core's import
graph stays clean.

## Core types & functions

```go
type Commit struct {
    SHA, Author, Subject string
    Time                 time.Time
}

// Event is re-exported from clarityrefs (which stays the canonical
// public API for the events ref). The core imports clarityrefs;
// clarityrefs does NOT import internal/core. See "Public API surface"
// below.
type Event = clarityrefs.Event
type Events map[string][]Event // keyed by commit SHA

type Snapshot struct {
    Commits  []CommitView // joined commits + events, newest-first
    RepoName string       // for the Renderer header line
}

type CommitView struct {
    Commit
    Events []Event
}

type View struct {
    Snapshot Snapshot
    Groups   Groupings    // HEAD / CIPassed / InFlight / Deployed buckets
    Weekly   []WeekStat   // DORA throughput per ISO week
    Header   HeaderStatus // ci/deploy summary for the top header line
    Stale    bool         // rendered from cache; fresh refresh in flight
}
```

### Two pure derivations

The core is the home of two pure functions that all adapters and the lens
share:

```go
// BuildSnapshot joins commits with their events by SHA. Every Source
// adapter calls this to produce a Snapshot from its raw inputs. Pure.
func BuildSnapshot(commits []Commit, events map[string][]Event, repoName string) Snapshot

// DeriveView runs the lifecycle grouping, DORA aggregation, header
// status, and per-commit derived state. Called by the Lens and the
// demo binary. Pure.
func DeriveView(snap Snapshot) View
```

Two cores, in the sense the user asked about: one builds the joined
shape, the other derives the renderer-ready shape. Both stateless,
both fully testable as pure functions, both in `internal/core`.

Currently in `internal/tui` and migrating to `internal/core` unchanged:
`GroupCommits`, `LeadTime`, `DeployedAtIndex`, `IsStaleStage`,
`WeeklyStats`, `CollapseStages`, `OverallStatus`, plus the `Snapshot` /
`CommitView` types that move from `internal/watcher`. The new `View`,
`Groupings` (exported), `WeekStat` (exported), `HeaderStatus`, and
`BuildSnapshot` / `DeriveView` functions are added during step 1.

### Public API surface

`clarityrefs` stays the **canonical public Go API** — its `Event`,
`WriteEvent`, `WriteEvents`, `ReadEvents`, `ReadAllEvents`, `EventsRef`
remain externally importable. Third-party code that reads or writes the
clarity events ref keeps working unchanged.

`internal/core` imports `clarityrefs` for the `Event` type (re-exported
as an alias for in-core ergonomics). The dependency direction is
`core → clarityrefs`, never the reverse. `Snapshot` and `View` are
core-internal: they're derived types used by the renderer, not a public
storage format, so they live under `internal/`.

## Composition

The Lens shrinks to almost nothing — adapters do the polling, the lens
just runs `DeriveView` on each Snapshot:

```go
// internal/core/lens.go
type Lens struct {
    source Source
}

func NewLens(source Source) *Lens

func (l *Lens) Views(ctx context.Context) <-chan View {
    out := make(chan View)
    go func() {
        defer close(out)
        for snap := range l.source.Watch(ctx) {
            select {
            case out <- DeriveView(snap):
            case <-ctx.Done():
                return
            }
        }
    }()
    return out
}
```

Composition root (`cmd/git-clarity/main.go`):

```go
cfg := config.Load(repoPath)
source := buildSource(cfg, repoPath, "ref" /* default identifier */)

lens := core.NewLens(source)
if opts.useTUI {
    lens = core.NewCachedLens(lens, ".git/clarity/snapshot-cache.json.gz")
}

renderer := pickRenderer(cfg, opts)
renderer.Render(ctx, lens.Views(ctx))
```

`buildSource` is a factory keyed by identifier (`ref`, `github`,
future `file:...`) — same factory the `copy-events` subcommand uses to
look up its `--from` and `--to` adapters. Single resolution path
regardless of caller.

The `Lens` doesn't take writers, sinks, or any output destination other
than the views channel. If the user wants events forwarded to another
store, that's a separate `copy-events` invocation — not a side effect of
rendering.

## Stale-while-revalidate cache

`CachedLens` is a decorator around `Lens` used only for renderers that want
instant first paint (TUI, web). It:

1. Emits a cached `View` (with `Stale: true`) immediately on startup if
   `.git/clarity/snapshot-cache.json.gz` exists and parses.
2. Lets the underlying `Lens` produce fresh `View`s (`Stale: false`) as
   normal; each fresh view supersedes the stale one and gets written back
   to the cache.

**Plain mode** does NOT use `CachedLens` — scripts and agents need
fresh data. Cache writes live in the decorator rather than the bare
`Lens`, so a plain run neither reads nor warms the snapshot cache. (The
plan originally had `Lens` write it as a side effect; implementation
kept the core free of the cache entirely, which is the better boundary
— the price is that a plain run doesn't warm the next TUI run.)

**Renderers** check `View.Stale` and may show a small "refreshing…"
indicator (TUI: right of header; web: spinner; plain: ignored).

### Three loading states owned by the renderer

There are *three* visual states the renderer needs to handle:

1. **No view yet** — channel hasn't emitted anything. Renderer shows
   `Loading…` (bubble-tea's existing `Model.received` pattern keeps
   working).
2. **Stale view** — cached view delivered first by `CachedLens`. Render
   the data plus a "refreshing…" indicator (`View.Stale == true`).
3. **Fresh view** — real fetch completed. Render the data, no
   indicator (`View.Stale == false`).

Only states 2 and 3 are encoded in `View`. State 1 is the renderer's
own concern — the channel simply hasn't produced anything yet. Keeping
"loading" out of the View protocol means the lens doesn't have to emit
synthetic "I'm loading" sentinels.

## Caching strategy summary

Two complementary caches, both optional, neither in the core. Both are
atomic gzipped JSON files under `.git/clarity/`, written via a shared
helper — git refs would add machinery (content-addressing, push semantics,
fast-forward retry loops) that local-only single-writer caches don't need.

| Cache              | Location                              | Purpose                                                                                                                                                     |
| ------------------ | ------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **gh runs cache**  | `.git/clarity/github-runs.json.gz`    | Mirror of raw GitHub Actions data inside the GH `EventReader` adapter. Makes the mapping → events derivation fast (no API hit) and incremental polls cheap. |
| **snapshot cache** | `.git/clarity/snapshot-cache.json.gz` | Joined `Snapshot` written by `CachedLens` for stale-while-revalidate TUI startup.                                                                           |

Shared `internal/cache` helper:

```go
// internal/cache/file.go
type File struct{ path string }

func New(path string) *File
func (f *File) Read() ([]byte, bool, error)   // (data, exists, err)
func (f *File) Write(data []byte) error       // atomic: write temp + rename
func (f *File) Invalidate() error
```

Just atomic gzipped bytes — callers JSON-marshal whatever they want.
`CachedLens` puts a `Snapshot` through it; the gh-source adapter puts a
`map[runID]GHRun` through it. `rm` to clear either.

Both are _implementation details of adapters_, not ports — the core
doesn't know they exist.

### Cache location configurability

Default cache path is `.git/clarity/` (lives inside the repo's working
tree). For ephemeral container deployments (the Docker forwarder) the
working tree may be discarded between runs; losing the gh-cache defeats
incremental polling. Two overrides:

- `--cache-dir <path>` CLI flag — explicit per invocation
- `CLARITY_CACHE_DIR` env var — for containerised use, point at a mounted
  persistent volume

Resolution order: flag → env var → default. Both cache files
(`snapshot-cache.json.gz` and `github-runs.json.gz`) live inside the
resolved directory.

### Diagnostics log

The resolved directory also holds `ghsource.log` — not a cache, but it
shares the location because it shares the "local, disposable, per-repo"
lifetime.

Non-fatal failures in the gh polling loop (a `ListRuns` call that
rate-limits, one run whose `/jobs` fetch fails) must not kill the TUI:
the adapter keeps its stale cache and retries next tick. But they also
can't go to stderr — the TUI runs in an alt-screen, and a mid-frame
write corrupts the render. So `ghsource.Options.Logger` takes an
`io.Writer` (nil ⇒ `io.Discard`) that the composition root points at
this file in append mode; `CLIClient.WithLogger` does the same for
failures that occur below the `Source`. Each line is RFC3339-stamped.

Fatal errors keep their existing behaviour — `ghsource.New` and the
first workflow-listing call still fail the process loudly before the
TUI starts.

## Adapters

```
internal/
  gitlog/        # Shared utility — gogit commit walk (used by both Source adapters)
  cache/         # Shared atomic gzipped-file helper (used by CachedLens + ghsource)
  adapters/
    refsource/   # Source for refs/clarity/events  (today's watcher logic)
    ghsource/    # Source for GitHub Actions (gh CLI + github-runs.json.gz)
    refwriter/   # EventWriter for refs/clarity/events  (used by copy-events)
    filewriter/  # EventWriter for a JSONL file (future, used by copy-events)
    tui/         # Renderer — bubble-tea
    plain/       # Renderer — plain text
    web/         # Renderer — HTTP server (future)
```

GH adapter shells out to `gh` (zero new auth code; users already have it).
A `GHClient` interface lets tests substitute a fake without subprocess.

### What each adapter owns

| Concern                | refsource                         | ghsource                                |
| ---------------------- | --------------------------------- | --------------------------------------- |
| Initial setup          | `EnsureClarityFetchRefspec`       | (none — `gh` handles auth externally)   |
| Polling cadence        | ~5s, `ls-remote` (covers both refs in one call) | ~30s, `gh api` with `updated_at` filter |
| Fetch verb             | `git fetch` (branch + events ref) | `gh api`; writes raw runs to gh-cache   |
| Commit log read        | `internal/gitlog`                 | `internal/gitlog`                       |
| Events read            | `clarityrefs.ReadAllEvents`       | Derive from gh-cache + `clarity.github` mapping |
| Snapshot join          | `core.BuildSnapshot`              | `core.BuildSnapshot`                    |
| Change detection       | ref tip moved vs last tick        | any new/updated run since last tick     |
| Emit                   | Snapshot when changed             | Snapshot when changed                   |

Both implementations of `Source.Watch` are independent — neither needs
to coordinate with the lens or with each other. The lens never knows
which Source it's using.

## Copy events between event stores

A separate, single-purpose subcommand for moving events from one store to
another. Doesn't touch the core — no commits, no grouping, no views, no
renderer. Just `EventReader.Read()` → `EventWriter.Write()`.

```
git clarity copy-events --from <source> --to <destination>
```

- `--from <source>` — short identifier for an event store (see below)
- `--to <destination>` — short identifier for an event store
- `--watch` (optional) — keep running, polling the source and writing
  deltas. Intended for the always-on Docker forwarder use case below;
  for cron-driven sync the one-shot form is simpler.

### Store identifiers

| Identifier | Refers to                                                         | As source | As destination |
| ---------- | ----------------------------------------------------------------- | --------- | -------------- |
| `ref`      | `refs/clarity/events` (the clarity events ref)                    | ✓         | ✓              |
| `github`   | GitHub Actions API (uses `clarity.github` mapping from config)    | ✓         | ✗              |
| `file:…`   | A JSONL file at the given path (future)                           | ✓         | ✓              |

`github` is read-only — clarity doesn't push events back to GitHub
Actions. `--to github` errors out at parse time with a clear message
("github isn't a valid destination — events from github can only be
copied OUT, not IN"). Future stores will follow the same pattern: each
identifier declares which directions it supports.

For users who want a non-default ref path, a URI-style escape hatch
later (`--from ref:refs/some/other/path`) — additive, bare `ref` keeps
working as the common case.

### Examples

```sh
# One-shot backfill from GitHub Actions into the events ref:
git clarity copy-events --from github --to ref

# Continuous mirror (Docker container, daemon-style):
git clarity copy-events --from github --to ref --watch

# Export ref contents to a JSONL file (future):
git clarity copy-events --from ref --to file:./backup.jsonl

# Rejected — clarity doesn't write to GitHub Actions:
git clarity copy-events --from ref --to github
# → error: 'github' is read-only as a destination
```

This subsumes the current `scripts/generate-backfill.sh` flow: pick which
jobs are CI vs deploy in `.ezcd.json` once, then run the copy command. No
generated script, no two-layers-of-bash. First-time users get an
interactive helper via `git clarity init --github` (see sequencing) so
they don't have to hand-write the mapping.

### `--watch` steady-state cost

`--watch` is not "N pushes per N events". Per tick:

1. Source's `Watch` channel either yields a Snapshot (something
   actually changed) or stays silent.
2. If a Snapshot yields, the writer computes the new tree using
   content-addressed event filenames. If the resulting tree hash
   matches the parent's, `WriteEvents`' idempotency short-circuit (see
   [clarityrefs.go:updateEventsRef](../clarityrefs/clarityrefs.go)) skips
   the commit and push entirely.
3. A push only happens when actual new events landed (different content
   → different filenames → different tree).

So the steady-state cost when nothing's happening is just the source's
own poll (e.g. one `gh api ...?updated_at>=since`). Network traffic is
proportional to *change*, not to wall-clock time.

## Web UI / Docker / always-on forwarder (future)

The web renderer is just another `Renderer` adapter. It:

1. Starts an HTTP server bound to a configurable port
2. Maintains the latest `View` in memory
3. Pushes updates to connected clients via SSE or WebSocket
4. Returns from `Render` when ctx is canceled

Configured via env vars (git repo URL, SSH key, branch, source choice) it
runs as a long-lived Docker container. Two modes the same container can
serve:

- **Web UI mode**: `git clarity --web` — web renderer attached to a lens
  reading from the configured source. Non-terminal users see the lens
  in a browser. No event writing.
- **Headless forwarder mode**: `git clarity copy-events --from github
  --to ref --watch` — no renderer, just a long-lived pump from source
  to destination. Useful as a centralised always-on instance that
  mirrors GitHub Actions activity into the events ref (or later, into a
  DB / cloud).

The two modes are distinct subcommands rather than two roles of the same
process — a single container can run either, or you can run two
containers (one for UI, one for forwarding). Cleaner than overloading
"clarity" with both responsibilities.

The web renderer maintains its own per-client sync state (clients
connecting at different times need bootstrap + delta), but that's
contained inside the adapter — the rest of the system doesn't know.

## Config (`.ezcd.json`)

Shared with pushq (and any other future ezcd tools). The whole file is
**optional**: with no `.ezcd.json`, clarity falls back to current
defaults (`branch: main`, source = `refs/clarity/events`).

The config describes *project facts* (which branch, which tools are in
use, how GitHub Actions jobs map to clarity stages). Operational choices
like "where do you want to copy events to" are CLI arguments to
`copy-events`, not config — they're decided per invocation, not once
per repo.

### Schema

```json
{
  "branch": "main",
  "pushq":   { ... },
  "clarity": { ... }
}
```

- **`branch`** (top-level) — the trunk branch every ezcd tool operates
  on. Defaults to `main` when omitted. Clarity uses it for the commit
  log; pushq uses it for its queue ref. Single source of truth — there's
  no per-tool branch override.
- **`pushq`** (optional) — presence enables pushq integration. Currently
  just `test_command`:
  ```json
  "pushq": { "test_command": "go test ./..." }
  ```
  Clarity infers "pushq is in use" from this section's existence — no
  separate `enabled` flag.
- **`clarity`** (optional, see below) — only needed when the user wants
  a non-default source (i.e. GitHub Actions instead of the ref).

### `clarity` shape

```json
"clarity": {
  "github": {
    "ci":     { "workflow": "CI",     "jobs": ["Test", "Integration"] },
    "deploy": { "workflow": "Deploy", "jobs": ["deploy"] }
  }
}
```

- **`github`** (optional) — presence switches the source from the
  default ref to the GitHub Actions API. The mapping picks which
  workflows + jobs feed clarity's CI and deploy stages. Used by both
  the TUI/plain renderers and by `copy-events --from github`.
  - `jobs` collapses started/completed into one field for the 95% case
    where they're the same set. A struct shape
    (`{ "started": [...], "completed": [...] }`) is also accepted via
    `json.RawMessage` dispatch when the user needs distinct sets.

No `sinks` / `mirror_to` field. Writing events somewhere else is a
verb (`copy-events`), not a property of the project.

### Example shapes

**No config file at all** — clarity defaults work:

```
(no .ezcd.json)
```

→ branch=main, source=`refs/clarity/events`, no pushq.

**pushq + clarity, defaults for clarity**:

```json
{
  "branch": "main",
  "pushq": { "test_command": "go test ./..." }
}
```

→ pushq enabled. Clarity reads from `refs/clarity/events`.

**Clarity-only, no pushq, custom branch**:

```json
{ "branch": "trunk" }
```

→ branch=trunk, ref source.

**Clarity with GitHub Actions as source**:

```json
{
  "branch": "main",
  "clarity": {
    "github": {
      "ci":     { "workflow": "CI",     "jobs": ["Test"] },
      "deploy": { "workflow": "Deploy", "jobs": ["deploy"] }
    }
  }
}
```

→ `git clarity` reads from GitHub Actions. `git clarity copy-events --from
github --to <wherever>` uses the same mapping. Nothing is automatically
written anywhere; if the user wants to mirror events into the ref they
run `copy-events` themselves (one-shot, scheduled, or `--watch`).

**Both tools, full config**:

```json
{
  "branch": "main",
  "pushq":  { "test_command": "go test ./..." },
  "clarity": {
    "github": {
      "ci":     { "workflow": "CI",     "jobs": ["Test", "Integration"] },
      "deploy": { "workflow": "Deploy", "jobs": ["deploy"] }
    }
  }
}
```

### Inference rules summary

| Has `pushq` section? | Has `clarity.github` section? | Resulting behavior                                |
| -------------------- | ----------------------------- | ------------------------------------------------- |
| no                   | no                            | Ref source, no pushq awareness                    |
| yes                  | no                            | Ref source, pushq awareness on                    |
| no                   | yes                           | GH source                                         |
| yes                  | yes                           | GH source, pushq awareness on                     |

Mirror / backfill behaviors aren't in this table because they're not
config-driven — they're separate `copy-events` invocations.

## Limit / scrollback

> **Settled differently from the original plan.** The default stays at 100.
> `--limit 0` is the opt-in for walking everything.

The plan called for flipping the default to unbounded ("load everything" —
same UX as `git log`). It wasn't worth it. Clarity is a "what is in flight
right now" view; the cost analysis below shows unbounded is *affordable*,
but affordable is not the same as useful, and nobody scrolls ten thousand
commits back to check a deploy from two years ago. `--limit 0` covers the
case that does.

What the cap actually costs is the DORA aggregate at the window boundary,
and that is fixed independently of the default: the oldest week's divider is
dropped rather than shown undercounted, and the list closes with a note
naming the limit that ended it. Both apply at any `--limit`, which flipping
the default would not have — a user passing `--limit 50` had the same
problem.

`gogit` walks 10k commits in ~1s; events ref parse is already unbounded;
memory cost ~1.2MB per 10k commits in rendered string form. Acceptable.

DORA stats span everything in the loaded snapshot. There's no good reason
to bound DORA history smaller than what's displayed.

## Test architecture

Three layers, each able to be tested without touching the layer below.

### Layer 1 — Core (pure functions)

Lives in `internal/core/*_test.go`. Table-driven, microsecond-fast, no
goroutines, no temp dirs, no I/O.

- `BuildSnapshot` — joins commits + events. Edge cases: events for
  unknown SHAs, commits with no events, mixed.
- `DeriveView` — end-to-end on hand-built Snapshots. Existing
  `tui/group_test.go`, `weekly_test.go`, `elapsed_test.go` migrate here
  with import-path updates only.
- `Lens.Views` — uses a `fakeSource` (programmable channel of
  Snapshots), asserts `View` flows through derivation correctly. This
  is the only Layer-1 test that uses a goroutine.

### Layer 2 — Adapters (each tested in isolation)

Each adapter lives in `internal/adapters/<name>/`. Tests use the
narrowest set of dependencies the adapter actually has.

- **`refsource`** — uses `gittest.NewRemote` / `gittest.NewClone` (real
  git over a temp dir). Push events, assert the source emits a fresh
  Snapshot with the new event present. Inherits today's
  `internal/watcher/*_test.go` coverage.
- **`ghsource`** — uses a fake `GHClient` returning canned `Runs` /
  `Jobs` responses, real `gitlog` against a `gittest` repo, real cache
  file in a temp dir. Asserts the mapping config → events derivation,
  and that the cache stores raw API data correctly.
- **`refwriter`** — `gittest`, assert events land on the ref at the
  expected paths.
- **`tui`** — feed scripted `Views` through, snapshot-test the
  rendered strings. Existing `program_test.go` pattern.
- **`plain`** — same idea, simpler output. Existing
  `render_plain_test.go` pattern.

### Layer 3 — Composition / integration

- `cmd/git-clarity/main_test.go` — root-flag parsing, `copy-events`
  identifier validation, direction rejection (`--to github` errors),
  config loading.
- `test/integration/clarityrefs_test.go` — end-to-end via the real
  binary. Stays where it is.

### Fakes that need to exist

| Fake               | Used by                          | Purpose                                              |
| ------------------ | -------------------------------- | ---------------------------------------------------- |
| `core.fakeSource`  | `lens_test.go`                   | Programmable channel of Snapshots                    |
| `ghsource.fakeClient` | `ghsource_test.go`            | Canned GH API responses (Runs, Jobs)                 |
| `core.fakeRenderer` | composition tests in `cmd/`     | Captures `View`s to a slice                          |
| `core.fakeWriter`  | `copy-events` tests              | Captures `Write(events)` calls                       |

All four are simple structs with channels/slices. No mocking framework.

### Migration of existing tests

| Existing                                            | After                                            |
| --------------------------------------------------- | ------------------------------------------------ |
| `clarityrefs/*_test.go`                             | Stays — public API surface                       |
| `internal/tui/group_test.go`                        | `internal/core/groupings_test.go`                |
| `internal/tui/weekly_test.go`                       | `internal/core/weekly_test.go`                   |
| `internal/tui/elapsed_test.go`                      | `internal/core/elapsed_test.go`                  |
| `internal/tui/render_test.go`                       | `internal/adapters/tui/render_test.go`           |
| `internal/tui/render_plain_test.go`                 | `internal/adapters/plain/render_test.go`         |
| `internal/tui/program_test.go`                      | `internal/adapters/tui/program_test.go`          |
| `internal/watcher/watcher_test.go` / `snapshot_test.go` | `internal/adapters/refsource/refsource_test.go` |
| `cmd/git-clarity/main_test.go`                      | Stays                                            |
| `test/integration/clarityrefs_test.go`              | Stays — **highest-risk step** for accidental breakage |

The integration suite is the canary. Step 3 (refsource extraction) is
the one where it'll get exercised hardest; treat it as the highest-risk
step, not the lowest.

## Sequencing (work plan)

Each step is a separate commit. Each leaves the suite green and the binary
functional. Risk callouts on the steps most likely to surface latent
coupling.

1. **Move pure lens code to `internal/core`** — `GroupCommits`,
   `LeadTime`, `DeployedAtIndex`, `IsStaleStage`, `WeeklyStats`,
   `CollapseStages`, `OverallStatus`, plus `Snapshot` and `CommitView`
   from `internal/watcher`. Add `BuildSnapshot` and `DeriveView`. Semantics
   unchanged but the diff is sweeping — every `internal/tui`,
   `internal/watcher`, and `cmd/demo` file with these imports has to be
   touched. Tests still pass.
2. **Define `Source` and `Renderer` ports** in `internal/core/ports.go`.
   The bubble-tea renderer and the existing watcher temporarily
   implement them in-place; no logical change yet.
3. **Extract `refsource` adapter** from the existing watcher into
   `internal/adapters/refsource/`. Move `EnsureClarityFetchRefspec`
   into `refsource.New`. Lift the unified `ls-remote` loop verbatim;
   the adapter owns its own polling. ⚠️ **Highest-risk step** — this is
   where `test/integration/clarityrefs_test.go` gets exercised hardest.
   Run the integration suite explicitly as part of this commit's check.
4. **Move the renderers** into `internal/adapters/tui` and
   `internal/adapters/plain`. Both consume `<-chan View` via the
   `Renderer` interface. Add `Snapshot.RepoName` so renderers don't need
   the name via constructor.
5. **Add `CachedLens` decorator** for SWR startup. TUI mode wraps;
   plain mode doesn't. Add `View.Stale` and the small indicator in the
   TUI. Renderer keeps owning its "no view yet → Loading…" state.
6. **Config loader** for `.ezcd.json` + `--cache-dir` / `CLARITY_CACHE_DIR`.
   Optional in v1 (no config → current defaults). Threads `cfg.Branch`
   through everywhere that's currently hardcoded `"main"`.
7. **GH source adapter** (`internal/adapters/ghsource`) — shells out to
   `gh`, caches raw GH data into `<cache-dir>/github-runs.json.gz`,
   derives events from cache + mapping config. First user-visible
   feature: `git clarity` renders the TUI from GitHub Actions data, no
   writes anywhere.
8. **`git clarity init --github`** — interactive workflow + job
   discovery. Same gh-API exploration the bash script does today, but
   it writes the answers into `.ezcd.json` instead of generating
   another script. Replaces the discovery half of
   `scripts/generate-backfill.sh`.
9. **`copy-events` subcommand + `EventWriter` interface + `refwriter`
   adapter**. `git clarity copy-events --from github --to ref` replaces
   the execution half of `scripts/generate-backfill.sh`. Identifier
   table + direction validation. Add `--watch` for the daemon use case.
10. **Web renderer + Docker image** — future. Same `Renderer` port. A
    long-lived container can run either `git clarity --web` (UI mode)
    or `git clarity copy-events --watch` (forwarder mode).

## Migration notes (user-visible behaviour changes)

Worth a release note when this lands:

- ~~**`--limit` default flips from 100 to unbounded.**~~ Not done — the
  default is still 100. `--limit 0` opts into unbounded and pays the
  one-time startup cost (~1s per 10k commits). What did change: a truncated
  list now says so on its last line, and drops the boundary week's DORA
  divider instead of showing a partial count.
- **TUI shows a "refreshing…" indicator** while a stale cached view
  is being revalidated. First-paint feels instant on repeat
  invocations; the indicator disappears as soon as the fresh fetch
  completes.
- **`scripts/generate-backfill.sh` retires** (after step 9). Migration:
  `git clarity init --github` (one-time mapping setup) +
  `git clarity copy-events --from github --to ref`. The README example
  changes from `bash backfill.sh` to two `git clarity ...` invocations.
- **Cache files** live at `.git/clarity/` by default; configurable via
  `--cache-dir` / `CLARITY_CACHE_DIR` for containerised use.
- **`.ezcd.json` is now read** if present. No config = current defaults
  preserved — opt-in, not breaking.

## Out of scope (for now)

- Lazy / paginated scrollback (current proposal loads everything; revisit
  if profiling shows real cost on huge repos)
- Cross-version cache schema migration (just invalidate on version bump)
- Additional event sources (GitLab, Buildkite) — once ghsource lands, the
  pattern is established
- `Sink` ports for derived state (View / aggregates) — the proper "sink"
  abstraction for things like dashboards, Slack alerts, metric
  exporters. Deferred until there's a concrete consumer.
