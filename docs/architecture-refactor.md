# Architecture refactor — hexagonal core, pluggable sources/renderers

Working plan, not yet implemented. Treat as a target.

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
                       ┌───────────────────────────────┐
   inbound ports       │                               │     outbound ports
   ─────────────       │      core (the lens)          │     ──────────────
                       │                               │
   CommitReader  ─────►│  Snapshot → DeriveView →      ├─────►  Renderer
                       │             View              │
   EventReader   ─────►│                               │
                       │  (GroupCommits, WeeklyStats,  │
                       │   LeadTime, OverallStatus...) │
                       │                               │
                       └───────────────────────────────┘
```

The core is pure: no `os`, no `net`, no `bubbletea`, no `gogit`. It takes
typed inputs from the readers, computes the derived view, and hands typed
outputs to the renderer.

`EventWriter` is an **auxiliary** outbound type used only by
`git clarity copy-events` (see below) — never imported by `internal/core`.
Writing events somewhere is a separate verb from rendering a view; the
two don't share a code path.

## Ports

### Inbound

```go
type CommitReader interface {
    Log(ctx context.Context, branch string, limit int) ([]Commit, error)
    WatchHead(ctx context.Context, branch string) <-chan struct{}
}

type EventReader interface {
    Read(ctx context.Context) (Events, error)         // synchronous, one-shot
    Watch(ctx context.Context) <-chan Events          // streaming, emits on change
}
```

`CommitReader.WatchHead` is signal-only — callers re-`Log` on each tick.
`EventReader.Watch` re-emits the full event map on each change; tiny
payload, simpler than diffing on the wire.

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

Defined alongside the core ports for type-symmetry with `EventReader`, but
not depended on by `internal/core`. Used only by the `copy-events`
subcommand and by adapters that want to mirror their input somewhere as
a side effect. Lives in a separate file (`internal/core/aux.go` or
similar) so the core's import graph stays clean.

## Core types

```go
type Commit struct {
    SHA, Author, Subject string
    Time                 time.Time
}

type Event struct {
    Stage, Status string
    Time          time.Time
    CI            map[string]string
}

type Events map[string][]Event // keyed by commit SHA

type Snapshot struct {
    Commits []CommitView // joined commits + events, newest-first
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

func DeriveView(snap Snapshot) View // pure, used by Lens + demo binary
```

Currently in `internal/tui` and migrating to `internal/core` unchanged:
`GroupCommits`, `LeadTime`, `DeployedAtIndex`, `IsStaleStage`, `WeeklyStats`,
`CollapseStages`, `OverallStatus`.

## Composition

```go
// internal/core/lens.go
type Lens struct {
    commits CommitReader
    events  EventReader
    branch  string
    limit   int
}

func (l *Lens) Views(ctx context.Context) <-chan View
```

Composition root (`cmd/git-clarity/main.go`):

```go
cfg := config.Load(repoPath)

commits := gitlog.New(repoPath)
events  := pickEventReader(cfg, repoPath)  // ref-source OR gh-source

lens := core.NewLens(commits, events, cfg.Branch, 0 /* unbounded */)
if opts.useTUI {
    lens = core.NewCachedLens(lens, ".git/clarity/snapshot-cache.json.gz")
}

renderer := pickRenderer(cfg, opts)
renderer.Render(ctx, lens.Views(ctx))
```

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
fresh data. The bare `Lens` still writes the cache as a side effect, so a
plain run keeps the cache warm for the next TUI run.

**Renderers** check `View.Stale` and may show a small "refreshing…"
indicator (TUI: right of header; web: spinner; plain: ignored).

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

## Adapters

```
internal/
  cache/         # Shared atomic gzipped-file helper (used by CachedLens + ghsource)
  adapters/
    gitlog/      # CommitReader via gogit
    refsource/   # EventReader for refs/clarity/events  (existing watcher logic)
    ghsource/    # EventReader for GitHub Actions (gh CLI + github-runs.json.gz)
    refwriter/   # EventWriter for refs/clarity/events  (used by copy-events)
    filewriter/  # EventWriter for a JSONL file (future, used by copy-events)
    tui/         # Renderer — bubble-tea
    plain/       # Renderer — plain text
    web/         # Renderer — HTTP server (future)
```

GH adapter shells out to `gh` (zero new auth code; users already have it).
A `GHClient` interface lets tests substitute a fake without subprocess.

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
generated script, no two-layers-of-bash.

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

Default `--limit` becomes effectively unbounded ("load everything" — same
UX as `git log`). The bubble-tea viewport handles arbitrary content length;
it only renders the visible portion. `--limit N` stays available as a
faster-startup escape hatch for very large repos.

`gogit` walks 10k commits in ~1s; events ref parse is already unbounded;
memory cost ~1.2MB per 10k commits in rendered string form. Acceptable.

DORA stats span everything in the loaded snapshot. There's no good reason
to bound DORA history smaller than what's displayed.

## Sequencing (work plan)

Each step is a separate commit. Each leaves the suite green and the binary
functional.

1. **Move pure lens code to `internal/core`** — `GroupCommits`, `LeadTime`,
   `DeployedAtIndex`, `IsStaleStage`, `WeeklyStats`, `CollapseStages`,
   `OverallStatus`, plus `Snapshot` and `CommitView` from `internal/watcher`.
   No behavior change. All tests still pass.
2. **Define ports + payload types** in `internal/core/ports.go`. The
   bubble-tea renderer and the existing watcher temporarily implement them
   in-place; no logical change.
3. **Build `Lens` struct + `Views(ctx)` channel** combining
   `CommitReader` + `EventReader`. Refactor the existing watcher into a
   ref-source `EventReader` adapter and a gogit `CommitReader` adapter.
4. **Move the renderers** into `internal/adapters/tui` and
   `internal/adapters/plain`. Both consume `<-chan View` via the
   `Renderer` interface.
5. **Add `CachedLens` decorator** for SWR startup. TUI mode wraps; plain
   mode doesn't. Add `View.Stale` and the small indicator in the TUI.
6. **Config loader** for `.ezcd.json`. Optional in v1 (no config → current
   defaults).
7. **GH source adapter** (`internal/adapters/ghsource`) — shells out to
   `gh`, caches raw GH data into `.git/clarity/github-runs.json.gz`,
   derives events from cache + mapping config. First user-visible feature:
   `git clarity` renders the TUI from GitHub Actions data, no writes.
8. **`copy-events` subcommand + `EventWriter` interface + `refwriter`
   adapter**. `git clarity copy-events --from github --to ref` replaces
   `scripts/generate-backfill.sh`. Identifier table + direction
   validation. Add `--watch` for the daemon use case.
9. **Web renderer + Docker image** — future. Same `Renderer` port. A
   long-lived container can run either `git clarity --web` (UI mode) or
   `git clarity copy-events --watch` (forwarder mode).

## Out of scope (for now)

- Lazy / paginated scrollback (current proposal loads everything; revisit
  if profiling shows real cost on huge repos)
- Cross-version cache schema migration (just invalidate on version bump)
- Additional event sources (GitLab, Buildkite) — once ghsource lands, the
  pattern is established
- `Sink` ports for derived state (View / aggregates) — the proper "sink"
  abstraction for things like dashboards, Slack alerts, metric
  exporters. Deferred until there's a concrete consumer.
