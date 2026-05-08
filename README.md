# Clarity (`git clarity`) — Design Document

## Problem Statement

In trunk-based development, the question that matters is: **is HEAD on main green?** If it isn't, everything else stops until it is. The unit of work is the commit, the unit of integration is the commit, and the unit of release is the commit. Runs, jobs, and workflows are implementation details of how a commit gets validated and shipped.

CI/CD UIs have this backwards. GitHub Actions, GitLab Pipelines, and others present **runs** as the primary object — a list of pipeline executions, ordered by start time, with commits as a label on each row. Answering "is HEAD green?" requires scanning that list, ignoring retries of older commits, filtering out unrelated workflows, and finally finding the run that corresponds to the commit you actually care about.

This is the wrong model. The commit is the right object. A pipeline run is just a thing that happened to a commit, and what users need to see is the commit with its current status — not a log of every event that ever touched it.

`clarity` is built around the correct model. It is a single binary, distributed as a git extension (`git clarity`), that:

- runs as a TUI in any repo with no setup beyond install
- reads pipeline events stored on a dedicated git ref in the repository itself
- has a CI-side mode that writes those events from inside pipelines

There is no database, no separate backend, and no proprietary format — the data lives in the repo.

---

## Who This Is For

### Teams practising trunk-based development

Clarity is main-branch focused by design. The TUI's primary view is "what is happening on `main` right now" — the most recent commits, in order, with their pipeline status. This aligns naturally with TBD where the trunk is the only long-lived branch and every commit on it is a candidate for production.

It also pairs naturally with pushq (see Future Work) because both tools share the assumption that the main branch is the centre of the workflow.

### Teams who want a quick answer to "is main green?"

The native CI UIs (GitHub Actions, GitLab Pipelines, etc.) are organised around runs, not commits. To answer "is the latest commit on main green or red?" you have to find the most recent run _for that commit_, ignore retries of older commits, and visually parse a list designed for a different question.

Clarity answers that question directly. The top row of the TUI is the most recent commit on main, with its current status visible at a glance. No clicking through to find the right run.

### Teams who want to improve their delivery process via DORA metrics

The events data clarity captures naturally supports two of the four DORA metrics out of the box:

- **Lead time for changes** — time from commit to deploy, derivable from the timestamps of the first event and the `deploy passed` event for each commit
- **Deployment frequency** — count of `deploy passed` events per unit time

The remaining two — **mean time to recovery** and **change failure rate** — require additional event semantics (e.g. tagging deploys that failed in production, marking commits as hotfixes) and are tracked as Future Work. The data model is designed to accommodate them when the time comes.

Even with just the two metrics that fall out of the v1 design, teams get visibility into their delivery cadence without setting up dedicated DORA tooling.

---

## Core Concept

The git history is already the canonical timeline of what happened. Clarity adds a thin layer on top: pipeline events keyed by commit SHA, stored as files on a dedicated ref.

```
git log:               clarity events (refs/clarity/events):

abc123  add billing    events/abc123/...build-passed.json
                       events/abc123/...deploy-started.json
def456  fix auth       events/def456/...build-passed.json
                       events/def456/...deploy-passed.json
ghi789  update deps    events/ghi789/...build-failed.json
```

Rendering is just a `git log` walk with events loaded from the ref and joined onto each commit. The output is the answer to "what's happening with my code right now."

---

## Distribution: A Git Extension Like pushq

Clarity ships the same way pushq does — a single binary installed via Homebrew, invocable as `git clarity`. Git automatically discovers any executable named `git-clarity` on `PATH` and lets you run it as a subcommand.

```
brew install ezcdlabs/tap/clarity
git clarity              # opens the TUI in the current repo
```

The same binary is also used inside CI to report events:

```yaml
- run: git clarity report build started
- run: ./build.sh
- run: git clarity report build passed
- run: ./deploy.sh
- run: git clarity report deploy passed
```

Two modes, one binary, one install path.

---

## v1 Scope

The first release does exactly two things:

1. **`git clarity`** — opens a live updating TUI showing your commit history with pipeline status per commit.
2. **`git clarity report <stage> <status>`** — invoked from CI, writes a single event file to `refs/clarity/events` for HEAD and pushes the ref.

Everything else (web UI, metrics endpoint, pushq integration, multi-repo, relay mode, SaaS, summarisation into git notes, CI-specific integrations) is explicitly out of scope for v1 and listed under Future Work.

---

## Data Model

### The events ref (`refs/clarity/events`)

A dedicated branch — never merged to main, used purely as a coordination point. Its working tree contains one file per pipeline event, organised by commit SHA:

```
events/
  abc123/
    1744120000-a3f2c1.json
    1744120134-b8e4d2.json
  def456/
    1744120140-c1d5e3.json
    1744120175-d2e6f4.json
```

### Event filename

```
<unix-timestamp>-<short-uuid>.json
```

Timestamp first for natural sort order. Short UUID suffix for uniqueness when multiple events arrive in the same second. No CI-specific identifiers in the filename — keeps the design portable across all CI systems.

### Event content

```json
{
  "stage": "build",
  "status": "passed",
  "ts": 1744120134,
  "ci": {
    "system": "github-actions",
    "run_id": "12345",
    "run_url": "https://github.com/org/repo/actions/runs/12345",
    "actor": "alice"
  }
}
```

The **core schema** is `stage`, `status`, and `ts`. These three fields are required and stable.

The optional **`ci` block** is opaque metadata captured from the environment when available. Clarity opportunistically reads common env vars (`GITHUB_*`, `GITLAB_*`, `CI_*`) and populates the block. Renderers may surface this metadata where useful but never depend on its presence.

Statuses per stage: `started`, `passed`, `failed`, `skipped`.

Stages are user-defined. Common values: `build`, `test`, `deploy`. Clarity does not enforce a fixed pipeline shape — it renders whatever stages are reported.

### Why per-file events on a custom ref

A single shared blob (e.g. git notes) requires read-modify-write semantics for every append, with concurrent CI jobs racing on the push. Per-file events solve this:

- **Race-free appends** — different filenames, different events, no content conflicts. Only the fast-forward push race remains, which is trivially retried.
- **No data loss** — events have unique filenames so they cannot accidentally overwrite each other.
- **Audit trail by design** — every retry and parallel job's report is preserved as its own file.
- **Consistent with pushq** — same architectural pattern as `refs/push-queue/state`, making the EzcdLabs codebase coherent.

The trade-off is that events aren't directly inspectable with `git notes show`. This is addressed in Future Work via an optional summarisation layer that writes derived digests to `refs/notes/clarity` for native git tooling.

### Status collapse at render time

For each commit, the renderer walks all event files under `events/<sha>/`, sorts by timestamp, and computes the latest status per stage. This is what makes clarity feel different from a "run history" view — retries don't clutter the timeline; the user sees the current state of each stage per commit.

### Fetch refspec

The events ref isn't fetched by default. The first time `git clarity` runs in a repo, it ensures the fetch refspec is configured:

```
[remote "origin"]
    fetch = +refs/clarity/events:refs/clarity/events
```

This is a one-time, automatic step.

---

## The TUI: `git clarity`

A live updating terminal view of the most recent commits on the current branch (default: `main`), with pipeline stages and statuses rendered per commit.

```
┌─ clarity ────────────────────────────────────────────┐
│  ✓ alice   refactor user model    build · deploy     │
│  ⧗ dave    update dependencies    build...           │
│  ✗ eve     new search index       build failed       │
│  ✓ frank   tweak homepage         build · deploy     │
│  · grace   wip notes              (no events)        │
└──────────────────────────────────────────────────────┘
```

Updates live as the underlying refs change. Polls the remote every 5 seconds (configurable) using git's lightweight `info/refs` endpoint to check whether the events ref or branch tip has moved, and only does a full fetch when SHAs differ.

### Authentication

Uses whatever git auth the user already has configured (SSH agent, git credential helper). No tokens or env vars required when run inside a repo the user can already pull from.

---

## CI Reporting: `git clarity report`

Used inside pipelines, not by end users:

```
git clarity report <stage> <status>
```

Examples:

```yaml
- run: git clarity report build started
- run: ./build.sh
- run: git clarity report build passed
- run: ./deploy.sh && git clarity report deploy passed || git clarity report deploy failed
```

### What it does

1. Resolves HEAD SHA (or reads `GITHUB_SHA` / equivalent when set)
2. Builds the event JSON (core fields + auto-detected `ci` block)
3. Generates a unique filename: `<unix-ts>-<short-uuid>.json`
4. Optimistic push loop:
   a. Fetch `refs/clarity/events`
   b. Add the event file under `events/<sha>/<filename>`
   c. Commit with message `report: <sha> <stage> <status>`
   d. Push the ref
   e. If rejected (not fast-forward): fetch, replay commit, retry
5. Exit

### Authentication in CI

Uses whatever git credentials are already available to the CI runner. In GitHub Actions the existing `GITHUB_TOKEN` is sufficient — no additional secrets needed.

### Concurrency

Two pipeline jobs reporting simultaneously cannot corrupt each other's data because they write different files. The only contention is the fast-forward push race on `refs/clarity/events`, handled by the same retry loop pattern pushq uses for its state branch.

---

## Repository Structure

```
ezcdlabs/clarity/
├── cmd/git-clarity/      # main binary (named git-clarity for git extension discovery)
├── clarityrefs/          # public package: read/write clarity's events
├── internal/
│   ├── tui/              # terminal UI rendering
│   ├── report/           # the `report` subcommand logic
│   ├── events/           # ref read/write helpers, optimistic push loop
│   └── ci/               # opportunistic CI env var detection
└── go.mod
```

The `clarityrefs` package is the public Go API for anyone (third-party tooling, future EzcdLabs projects) who wants to read clarity's events:

```go
package clarityrefs

const EventsRef = "refs/clarity/events"

type Event struct {
    Stage  string
    Status string
    Time   time.Time
    CI     map[string]string  // optional, may be empty
}

func ReadEvents(repoPath, sha string) ([]Event, error)
func ReadAllEvents(repoPath string) (map[string][]Event, error)
func WriteEvent(repoPath, remote, sha string, event Event) error
```

Reads operate on the local events ref only — callers (typically the watcher) fetch first. `WriteEvent` fetches the remote ref before writing and pushes after, retrying on fast-forward rejection so concurrent reporters never lose events. Repository handles are passed as paths rather than `*git.Repository` so callers don't have to depend on a specific go-git version.

This mirrors the pattern pushq uses with its `pushqrefs` package — the ref format is the public contract, exposed via a Go package.

---

## Implementation Notes (Go)

### go-git vs shelling out

Use `go-git` for: fetching, reading the events ref, walking the commit log, reading event files from the tree.

Shell out to `git push` for writing — robust handling of credentials and refspecs. This matches pushq's pattern.

### TUI library

Bubble Tea (`charmbracelet/bubbletea`) is the standard choice in Go for TUIs of this kind. The render loop receives state updates from the watcher goroutine and re-renders.

### The watcher

A single goroutine that polls the remote on the configured interval and emits `[]Commit` snapshots whenever the events ref or branch tip moves. The TUI subscribes via a channel.

### Optimistic push loop

The same pattern pushq uses, applied to `refs/clarity/events`:

```go
for {
    err := fetchEventsRef()
    addEventFile()
    commit()
    err = pushEventsRef()
    if err == nil {
        break
    }
    if !isFastForwardRejected(err) {
        return err
    }
    // lost the race, retry
}
```

---

## What This Is Not

- Not a CI runner — it consumes events emitted by your existing CI
- Not a build cache or artifact store
- Not a code review tool
- Not a replacement for OpenTelemetry — it's a different model (commit-centric vs trace-centric)

---

## Future Work (Out of Scope for v1)

- **Notes summarisation** — generate a derived digest into `refs/notes/clarity` so the latest status per stage is inspectable via `git notes show <sha>` and visible in `git log --show-notes`. The custom ref remains the source of truth; notes are a read-side convenience that can be regenerated at any time.
- **Web UI mode** — a `--serve` flag that runs an HTTP server with an embedded SPA, for shared team dashboards
- **Prometheus metrics endpoint** — `--metrics` exposing `/metrics` with DORA-style aggregates derived from the events, for Grafana integration
- **Full DORA metrics support** — extending the event model to capture incidents and hotfixes so that MTTR and change failure rate can be derived alongside the lead time and deploy frequency metrics that already fall out of the v1 design
- **pushq integration** — when `refs/push-queue/*` exists in the repo, render the queue as a "pending" section above the most recent commit on main. Implemented by importing `github.com/ezcdlabs/pushq/pushqrefs` and adding a section to the TUI render
- **CI-specific integrations** — clickable run links, actor avatars, job-level breakdowns, derived from the `ci` metadata block on events
- **Relay mode** — a long-running process with git access that forwards normalised events to a hosted dashboard, enabling a SaaS tier without giving the SaaS git access
- **Multi-repo dashboard** — show several repos at once, primarily a feature for the relay/SaaS tier
- **Notification hooks** — Slack/Discord/webhook integrations for deploy events
- **Historical retention / `git clarity gc`** — events can grow unbounded; a pruning command for old events
- **Branch awareness** — currently focused on the main branch; optional support for PR branches with their own pipeline status
- **JSON output mode** — `git clarity --json` for scripting and piping into other tools
- **Configuration file** — a `.clarity.json` or `.git/config` section for per-repo settings (poll interval, branch, etc) once there are options worth configuring
