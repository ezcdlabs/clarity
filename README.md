# clarity (`git clarity`)

![demo](assets/happy-path.gif)

Commit-centric pipeline status TUI for trunk-based development. Answers "is main green?" at a glance — the most recent commits on main with their per-stage CI status in a live updating view. No server, no database, no proprietary format. Just git refs.

---

## Install

### macOS (Homebrew)

```sh
brew install ezcdlabs/tap/git-clarity
```

### Windows (Scoop)

```sh
scoop bucket add ezcdlabs https://github.com/ezcdlabs/scoop-bucket
scoop install git-clarity
```

### macOS / Linux (manual)

Download the latest release from [github.com/ezcdlabs/clarity/releases](https://github.com/ezcdlabs/clarity/releases), extract, and place `git-clarity` somewhere on your `$PATH`:

```sh
# example — adjust version and platform
curl -sSL https://github.com/ezcdlabs/clarity/releases/latest/download/git-clarity_linux_amd64.tar.gz \
  | tar -xz git-clarity && sudo mv git-clarity /usr/local/bin/
```

Git will then expose it as `git clarity`.

---

## Usage

### Watching: `git clarity`

In any git repository, run:

```sh
git clarity
```

This opens an alt-screen TUI showing the most recent commits on `main` with their pipeline status. Press `q` to quit. No setup beyond install — clarity adds the events fetch refspec to your repo's git config the first time it runs.

When `stdout` is not a terminal (e.g. piped to another command, redirected to a file, or invoked by an agent), `git clarity` automatically switches to a static plain-text rendering of the same view — header line plus HEAD / CI Passed / Deployed sections, no animation, no ANSI escapes. Force it in an interactive terminal with `--plain`. Three more flags shape the output in either mode:

- `--show-shas` — include a short commit SHA per row. Off by default in both TUI and plain modes; agents acting on specific commits should turn this on.
- `--limit N` — cap how many commits are walked. Defaults to 100; pass `0` for unlimited. When the cap is what ended the list, the last line says so and names the limit, so the bottom of a truncated scroll can't be mistaken for the start of the repository.
- `--deploy=<flow>` — select one deploy flow in a repo with several. In plain mode it narrows the output to that flow's section; in the TUI it opens with that flow selected. Only meaningful alongside [deploy targets](#deploy-targets).

The plain output is intentionally grep-friendly: `grep ✗` finds failed commits, `grep deploying` finds in-flight deploys, `grep "(live)"` finds the currently-live batch.

The Deployed section also surfaces per-ISO-week DORA throughput inline: a `W<year>-<NN>  N deploys  Xh Ym avg` summary on each week boundary. **Only the current week shares the `Deployed` header row**; every other week, including last week, gets its own divider below.

That position reads as the current state, so whatever sits in it is taken for now. Merging the *newest* week there instead meant a repo whose last deploy was a week ago showed last week's totals as its headline — "36 deploys" where a glance reads this week's velocity. Two repos side by side made it plain: one had deployed this week and one hadn't, and nothing in the headline distinguished them.

The current week is reported whether or not it has deploys — `W2026-39  0 deploys` — because that is what makes two repos comparable at a glance, and it leaves no headline slot for an older week to occupy. A week with no deploys has no average, so the label omits it rather than printing a placeholder: `0s avg` reads as an extraordinarily fast week rather than an empty one.

An empty current week is followed by a blank line, so the section opens the way `HEAD` and `CI Passed` do when they are empty. Without it the next week's divider butts directly against the header and the eye binds its totals to it, which is the misreading this whole rule exists to prevent. "Deploys" counts distinct deploy batches (not per-commit events) and "avg" averages the per-commit lead times shown on the right of each row — see [Lead time](#lead-time) for what those measure and how to change it. ISO weeks are computed in UTC so the same data renders into the same buckets regardless of the user's locale.

The oldest week in a truncated window gets **no** divider. `--limit` almost always cuts mid-week, so that week is missing however many deploys fell below the cap — and unlike a missing row, an undercounted aggregate is wrong in a way the reader cannot see. Dropping it is the honest option: a divider that isn't there prompts a `--limit 0`, whereas "2 deploys" for a week that had nine just gets believed. Weeks above the boundary are kept; they can only be short by a commit authored below the cut but deployed inside the window, which is the long-lead case `--limit 0` also answers.

### Lead time

Each row's right-hand column is that commit's lead time, and the weekly `avg` is the mean of the ones visible in the Deployed section. What "lead time" starts from is configurable, because the honest answer depends on what you intend the number to change:

```json
{ "clarity": { "leadTime": "pipeline" } }
```

| Mode | Which commits count | Measured from |
| --- | --- | --- |
| `all` (default) | every commit that reached production | its authoring time |
| `reported` | commits carrying at least one pipeline event | its authoring time |
| `pipeline` | commits carrying at least one pipeline event | its earliest event |

**Why the choice exists.** CI usually runs on the pushed head, not on every commit in a push. A developer who makes five small commits and pushes them together produces one commit with events and four without — but all five reached production, so under `all` they contribute five samples, each carrying however long the work sat unpushed. The average ends up describing commit-and-push habits as much as the pipeline. Someone who commits on Friday and pushes on Monday moves the team's number more than any pipeline change would.

`all` is the default so that upgrading never silently moves anyone's numbers.

`reported` removes the multiplier — one sample per pushed batch instead of one per commit — but not the magnitude. The surviving commit is still timed from when it was authored, so the Friday-to-Monday gap is still in the average, just once instead of five times. Choose it to keep the DORA definition intact and only stop the double-counting.

`pipeline` is the one to choose if the number is meant to drive pipeline work. Time spent unpushed, in review, or otherwise before CI took the code stops counting. Three consequences, all deliberate:

- It is **not** DORA lead time for changes, which includes commit-to-deploy precisely because unpushed work is real delay in the value stream. Don't report it as DORA.
- Anything before the first event is invisible, **including runner queueing** — which is genuine pipeline performance you stop seeing. `ci started` is reported from inside the job, so a saturated runner pool looks free.
- The number depends on reporting discipline. A team reporting only `ci passed` and `deploy passed` measures from the *end* of CI and gets a much smaller number than one that also reports `ci started`.

**Commits without a lead time still appear in the log.** Under `reported` and `pipeline` they render without a right-hand column and contribute nothing to the average. The intent is to stop them skewing the number, not to hide that they shipped.

**Non-positive intervals are dropped, not clamped.** A rebased or amended commit can carry an author date later than the deploy that shipped it; a commit can inherit a deploy from a newer commit that landed earlier; and under `pipeline`, a team reporting only terminal events has its start land on the deploy itself. All three produce a zero or negative interval, none of them represent fast delivery, and a run of them would quietly report a perfect pipeline. Such commits contribute nothing.

### Reporting from CI: `git clarity report <stage> <status>`

Inside your pipelines:

```yaml
- run: git clarity report ci started
- run: ./build.sh
- run: git clarity report ci passed
- run: ./deploy.sh && git clarity report deploy passed || git clarity report deploy failed
```

In GitHub Actions the existing `GITHUB_TOKEN` is sufficient — no additional secrets needed. Stages are exactly two: `ci` and `deploy`. Statuses are `started`, `passed`, `failed`, or `skipped`.

A monorepo that deploys several things adds an optional target to its deploy
reports — `git clarity report deploy passed ios` — and the TUI gains a selectable
flow per target. CI never takes one. See [Deploy targets](#deploy-targets).

### Reporting from GitHub Actions: `ezcdlabs/clarity`

For GitHub Actions specifically there is a setup action that handles installation and (optionally) the report call in one step. Pin the action ref to a release and you get the matching binary automatically — no separate `version:` input needed in the common case.

The job needs `contents: write` on the `GITHUB_TOKEN` because clarity reports by pushing an event to `refs/clarity/events`. The default token permissions vary by org and repo; declaring it explicitly makes the workflow portable.

Install only, then report manually:

```yaml
permissions:
  contents: write

steps:
  - uses: actions/checkout@v4
  - uses: ezcdlabs/clarity@v0.1.1
  - run: ./build.sh && git clarity report ci passed
```

Install and report in one step:

```yaml
permissions:
  contents: write

steps:
  - uses: actions/checkout@v4
  - uses: ezcdlabs/clarity@v0.1.1
    with:
      stage: ci
      status: started

  - run: ./build.sh

  - if: always()
    uses: ezcdlabs/clarity@v0.1.1
    with:
      stage: ci
      status: ${{ job.status == 'success' && 'passed' || 'failed' }}
```

Inputs:

- `version` — explicit version override (e.g. `v0.1.1`, `latest`). Defaults to the action's own ref when that's a semver tag, otherwise to the latest GitHub release.
- `stage` — pipeline stage to report (`ci` or `deploy`). When combined with `status`, the action runs `git clarity report <stage> <status>` after install.
- `status` — status to report (`started` / `passed` / `failed` / `skipped`).
- `target` — which deployable a `deploy` event is about, for a repo shipping several from one trunk (e.g. `ios`). Rejected on `ci`. Omit it for the untargeted deploy.

`actions/checkout` must run first so the action has a repository to push events from.

### Troubleshooting: `report` fails with a rename or bad object error

A report step that fails with one of these lost a race with `git gc`:

```
error: build tree: rename .git/objects/pack/tmp_obj_1741553381
  .git/objects/9f/9edc86...: no such file or directory
```

```
push events ref: fatal: bad tree object 5a8315cd2bc0...
error: remote unpack failed: eof before pack header was fully read
 ! [remote rejected] refs/clarity/events -> refs/clarity/events (unpacker error)
```

Both are fixed — **upgrade the pinned action ref to v0.1.6 or later**. Nothing needs configuring; older versions are affected regardless of settings.

CI is where this shows up because a fresh `actions/checkout` has every object in a packfile and no `.git/objects/<xx>` directories at all, so nearly every event write has to create one. See [Surviving a concurrent gc](#surviving-a-concurrent-gc) for the mechanism. Versions before v0.1.3 are the most exposed: the racing gc there is one clarity spawns itself, so the failure needs no other git activity in the workflow at all.

---

## Design Document

The rest of this document records the design decisions behind clarity. Treat it as a living spec — when an open question is resolved or a new constraint is established, this document is updated before the work is considered done.

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

- **Lead time for changes** — time from commit to deploy, derived per commit from its authoring time (or its first pipeline event) and the `deploy passed` event that shipped it. Which commits count and where the clock starts is configurable — see [Lead time](#lead-time), including why only the default mode is DORA's definition
- **Deployment frequency** — count of `deploy passed` events per unit time

The remaining two — **mean time to recovery** and **change failure rate** — require additional event semantics (e.g. tagging deploys that failed in production, marking commits as hotfixes) and are tracked as Future Work. The data model is designed to accommodate them when the time comes.

Even with just the two metrics that fall out of the v1 design, teams get visibility into their delivery cadence without setting up dedicated DORA tooling.

---

## Core Concept

The git history is already the canonical timeline of what happened. Clarity adds a thin layer on top: pipeline events keyed by commit SHA, stored as files on a dedicated ref.

```
git log:               clarity events (refs/clarity/events):

abc123  add billing    events/abc123/...ci-passed.json
                       events/abc123/...deploy-started.json
def456  fix auth       events/def456/...ci-passed.json
                       events/def456/...deploy-passed.json
ghi789  update deps    events/ghi789/...ci-failed.json
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
- run: git clarity report ci started
- run: ./build.sh
- run: git clarity report ci passed
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
scope/
  abc123/
    1744119900-e5f7a1.json
```

`scope/` is a sibling tree holding [candidacy](#candidacy-affected--unaffected)
records. It is separate from `events/` because candidacy is not a pipeline event,
and because every existing binary walks only `events/` — so a repo adopting
candidacy stays readable by older clarity versions by construction rather than by
luck.

### Event filename

```
<unix-timestamp>-<short-uuid>.json
```

Timestamp first for natural sort order. Short UUID suffix for uniqueness when multiple events arrive in the same second. No CI-specific identifiers in the filename — keeps the design portable across all CI systems.

### Event content

```json
{
  "stage": "deploy",
  "status": "passed",
  "ts": 1744120134,
  "target": "ios",
  "ci": {
    "system": "github-actions",
    "run_id": "12345",
    "run_url": "https://github.com/org/repo/actions/runs/12345",
    "actor": "alice"
  }
}
```

The **core schema** is `stage`, `status`, and `ts`. These three fields are required and stable.

`target` is optional and valid only on `deploy` events — it names which deployable
thing this event is about. Absent means the repo's untargeted deploy. See
[Deploy targets](#deploy-targets). It lives in the JSON rather than in the file
path so that adding it doesn't change `ReadEvents`' shape for existing readers;
filenames are content-addressed, so two events differing only by target already
hash to different files.

The optional **`ci` block** is opaque metadata captured from the environment when available. Clarity opportunistically reads common env vars (`GITHUB_*`, `GITLAB_*`, `CI_*`) and populates the block. Renderers may surface this metadata where useful but never depend on its presence.

Statuses per stage: `started`, `passed`, `failed`, `skipped`.

Stages are fixed at two: `ci` and `deploy`. This is deliberate — trunk-based development cares about exactly three things, latest HEAD, latest green CI, and latest production deploy, and that maps onto two state transitions. A "test" or "lint" or "integration" stage is not a separate lifecycle position; it's part of CI that either passes or doesn't. Adding more stages would dilute the "is HEAD green?" question this tool exists to answer. The `git clarity report` command rejects any other stage name with an error rather than letting custom names drift in.

### Why per-file events on a custom ref

A single shared blob (e.g. git notes) requires read-modify-write semantics for every append, with concurrent CI jobs racing on the push. Per-file events solve this:

- **Race-free appends** — different filenames, different events, no content conflicts. Only the fast-forward push race remains, which is trivially retried.
- **No data loss** — event filenames are content-addressed (`<unix-ts>-<sha256(json)[:8]>.json`), so two truly identical reports collapse into one file and two reports that differ (different timestamps, retries with different `GITHUB_RUN_ATTEMPT`, etc.) keep distinct files.
- **Audit trail by design** — every distinct report is preserved as its own file. Retries and parallel jobs carry different CI metadata so they hash to different filenames; duplicate writes of the literal same event are idempotent at the file and tree level (re-running a backfill produces no new commit).
- **Consistent with pushq** — same architectural pattern as `refs/push-queue/state`, making the EzcdLabs codebase coherent.

The trade-off is that events aren't directly inspectable with `git notes show`. This is addressed in Future Work via an optional summarisation layer that writes derived digests to `refs/notes/clarity` for native git tooling.

### Status collapse at render time

For each commit, the renderer walks all event files under `events/<sha>/`, sorts by timestamp, and computes the latest status per stage. This is what makes clarity feel different from a "run history" view — retries don't clutter the timeline; the user sees the current state of each stage per commit.

### Deploy targets

A monorepo with one integrated CI can still have several things to deploy — a web
backend, an iOS app, an Android app. Clarity models that as an optional `target`
on deploy events, which the TUI presents as a strip of selectable deploy flows.
The intent is to make a monorepo feel like several polyrepos without becoming a
multi-repo dashboard: one flow on screen at a time, one commit list underneath
them all.

**CI never takes a target.** `git clarity report ci passed ios` is an error, not a
filter. A target that can pass CI on its own is not integrated with the rest of
the repo, and the second word in "continuous integration" is the one doing the
work. CI answers one question for the whole commit; only deploy fans out. The
practical consequence is that "is main green?" has exactly one answer no matter
which flow is selected — which is what keeps this from degenerating into a
per-subsystem status board.

**An untargeted deploy is a flow of its own, not a shared one.** `deploy passed`
and `deploy passed ios` produce two flows, not one flow plus an overlay. A team
adding mobile doesn't have to touch the web pipeline in the same commit.

**Flows are declared in `.ezcd.json`,** which maps each flow to the target(s) whose
events it contains:

```json
{
  "clarity": {
    "deploys": [
      { "name": "web",     "targets": ["", "web"] },
      { "name": "iOS",     "targets": ["ios"] },
      { "name": "Android", "targets": ["android"] }
    ]
  }
}
```

`""` is the untargeted deploy — the literal value in the data, not a keyword. When
a flow's name is its only target the entry shortens to a bare string, so a repo
that has never renamed anything writes `"deploys": ["web", "ios", "android"]`.
The key is `deploys` rather than `tabs` because it describes the repo's
deployable units, not a UI affordance: plain mode consumes the same config to
produce stacked sections, and the file is useful documentation of what the
monorepo actually ships.

**Selecting a flow: `--deploy`.** A root flag, not a plain-mode one — plain mode
narrows to that flow's section, and the TUI opens with it selected. The latter is
how a monorepo gets the polyrepo feel that motivated targets in the first place:
one terminal pane per subsystem, each pinned to its own flow, with the strip
still showing the others' health so cross-flow visibility isn't traded away.

The flag says `deploy` rather than `target` because it selects a flow, and a flow
is not a target — `{"name": "web", "targets": ["", "web"]}` makes "the web
target" ambiguous where "the web deploy" is not. Matching is forgiving: flow name
first, case-insensitively, then a target name, so `--deploy=iOS` and
`--deploy=ios` both land on the iOS flow. That requires one invariant at config
load — a flow's name may not collide with another flow's target — checked
alongside the rule that two flows may not claim the same target.

An unknown value is an error naming the known flows, in both modes. It must
never fall back to rendering everything, or to some other flow: a script or
agent that quietly reported on the wrong subsystem is worse than one that
failed, and in the TUI an unrequested flow looks exactly like a deliberate
selection. The TUI therefore quits with the same error rather than opening on
something else.

The check waits for a view that can answer it. The cached lens paints a stale
frame first, from a snapshot that may predate the flow being asked for, so
resolving there would reject a flag the fresh data is about to satisfy.

In plain mode each flow's section carries its name on the header line, so the
grep-friendliness the plain renderer advertises (`grep ✗`, `grep deploying`)
survives having several flows stacked — a hit can still be attributed to a flow.

Declaration is what makes the config an **expectation**, and that is its real
value — it gives a gap in either direction a name:

- A declared flow with no events is a failed expectation: "we say we deploy
  Android; nothing has ever reported one." That is a finding, and without
  declaration it is invisible.
- An event whose target matches no declared flow is an unexpected actual — a typo,
  or a target someone added without updating config. When config is present
  `git clarity report` rejects an undeclared target at write time, so the typo
  fails the pipeline that made it. Stray events already on the ref still surface
  as an extra flow marked unexpected, because a silently dropped deploy is the
  worst outcome available.

A repo with no config accepts and discovers everything and hides nothing, so the
single-target case still needs no setup beyond install. The configuration cost
lands only on repos that have opted into multiple deploy targets.

**Many-to-one is for identity over time, not aggregation.** `{"name": "web",
"targets": ["", "web"]}` is a web backend mid-rename: both forms land in one
continuous flow, with no history rewrite, no force-push to the events ref, and no
rename record in the data. The same mechanism covers `ios` → `mobile-ios` later.
It is *not* for folding concurrently-live systems together — merging `ios` and
`android` into one "mobile" flow makes "deployed" mean "one of them deployed" and
averages two unrelated pipelines into one lead time.

Renaming is therefore adding a target to an existing flow. A rename command that
rewrote old events was considered and rejected: it requires a force-push to
`refs/clarity/events`, and a force-push racing an in-flight reporter is the only
unrecoverable write race in the system — every other loser simply replays its
commit. It would also break backfill idempotency (content-addressed filenames
would recreate the old target on a re-run) and destroy the audit property the
per-file design exists to provide.

**Flow names must be unique, and the invented one yields.** A flow is addressed
by name — in the strip, in plain output, and by anything grepping it — so two
flows sharing a label are indistinguishable to all three. The untargeted
deploy's label is the only one clarity invents, so it is the one that gives way
when a pipeline reports a target literally named `deploy`, or a config declares
a flow by that name. It falls back to `(untargeted)`. A target name is data and
must always render as itself.

**Flows never age out.** A target that last deployed months ago is the single most
valuable cell on the screen for a team trying to improve delivery; a rule that
hid it would be inverted against the point of the tool. Declared flows always
render, however stale.

Lead time and deployment frequency are computed per flow, measured to that flow's
deploys. An untargeted deploy contributes nothing to `ios`. This is strictly more
truthful than the single-flow view was for a monorepo, where a two-minute web
deploy and a multi-day store review were averaged into one meaningless number.

### Candidacy: affected / unaffected

Flows share one commit list, so every trunk commit sits above every flow's last
deploy. Left alone, an Android deploy weeks later scoops up hundreds of
web-only commits and each contributes a lead time measured from its authoring
time — turning Android's number into "the average age of every commit in the
monorepo" rather than "how long Android changes take to ship". That poisons the
DORA metric the tool exists to provide.

The fix rests on separating two questions that had been collapsed into one:

1. **Is commit X in the Android app?** No, definitively, whatever files it
   touched. Integrated CI means a commit is indivisible, so the behind-count
   stays unfiltered — and rightly: the binary is built from a tree containing all
   of those commits, and a shared-library change breaking Android is exactly why
   the CI is integrated. The batch-size risk is real.
2. **How long did it take to deliver Android change Y?** Only meaningful for
   commits that contained Android change.

Candidacy answers (2) only. It changes lead time and DORA attribution, and
touches neither the behind-count nor the deployed/not-deployed grouping.

```yaml
- run: |
    for t in web ios android; do
      if affected "$t"; then
        git clarity report affected "$t"
      else
        git clarity report unaffected "$t"
      fi
    done
```

**Why not a status.** `started` / `passed` / `failed` are outcomes of an attempt.
Candidacy is not an attempt — nothing was tried. It is a property of the commit,
true from the moment the commit exists, which is why it can be reported before
`ci started` has even run. `affected` is the word nx, turbo and bazel already
use, so whoever writes that `affected` helper is using it too.

**The record.** Candidacy lives in a sibling `scope/` tree on the same ref, at
`scope/<sha>/<unix-ts>-<hash>.json`:

```json
{ "target": "ios", "affected": false, "ts": 1744120134000000000 }
```

`ts` is Unix **nanoseconds**, unlike an event's seconds. Candidacy supersedes
by timestamp — a later record corrects an earlier one — so the resolution has
to be finer than the rate records can be written at, or two reports in the same
second tie.

All three fields are required on read. `affected` is never omitted when written
and a record missing it is rejected rather than defaulted, because false and
absent are different answers — and an event JSON is structurally valid
candidacy JSON, so a record that strayed into the wrong tree would otherwise
read back as a confident "unaffected" for the untargeted deploy. An empty
`target` is refused on write for the same reason: empty means the untargeted
deploy, so a forgotten field would record a real claim about a real flow.

Records come back in timestamp order, which is what makes "the latest record
wins" resolvable — git returns tree entries in filename order, which is
lexicographic rather than chronological.

**A tie resolves to affected.** Two records for one target at the same instant
have no honest winner: the ref preserves both and neither is newer. Resolving
to affected errs towards counting a commit, which can only inflate a lead time,
whereas the other way could hide delivery — and a number that flatters is the
worse failure for a metric meant to drive work. Without an explicit rule the
winner would be whichever content hash sorted first, so a correction could be
discarded while the command that wrote it reported success. Correcting a record
therefore needs a later timestamp, which is automatic unless `--at` pins it.

**Three states, not two.** Reporting both directions carries more information
than reporting only exclusions:

| What the ref says | What it means |
| --- | --- |
| nothing | nobody has told us; every commit is a candidate (the default, and today's behaviour) |
| `unaffected` on 336 of 340 | we know; this flow's lead time comes from the other four |
| `affected` on all 340 | CI is explicitly saying everything affects this target — a different answer from silence |

Because the default is "candidate", nothing moves for a repo that never reports
candidacy. Same principle that makes `all` the default lead-time mode.

**Rendering needs no new concept.** A non-candidate commit appears in that flow's
log with no right-hand column and contributes nothing to the average — exactly
how `reported` and `pipeline` modes already treat a commit with no events. The
intent is to stop it skewing the number, not to hide that it shipped.

**Candidacy records are not pipeline events.** The `reported` and `pipeline` lead
time modes count "commits carrying at least one pipeline event"; a commit whose
only record is `unaffected android` must not thereby acquire a lead time. The
separate `scope/` tree makes that easy to get right and easy to get wrong by
accident, so it carries a test.

**Why clarity does not infer this from paths.** A config like
`{"name": "android", "paths": ["apps/android/**"]}` is tempting — declarative, no
CI changes, and it would work retroactively on history where events only fix
things going forward. But which commits affect a target is a dependency-graph
question, not a glob question; that is what bazel, nx and turbo exist for. A
change under `libs/shared/` affects Android and matches no Android glob. Clarity
records what the build system says and does not infer build semantics.

### The header and the deploy strip

The header answers two questions with different scopes: CI is repo-wide, deploys
are per-flow. The layout says so by naming the group — `deploy:` labels the strip,
so the flows read as sub-items of deploy rather than as peers of `ci`.

A repo with one flow renders exactly what it renders today, byte for byte:

```
your-app · ci: ✓ · deploy: ✓
```

A repo with several gains the strip, and the header row becomes chrome:

```
your-app · ci: ✓ · deploy:  web ✓   ios ✗   android ·
```

**The selected flow is cut out of the chrome, not raised above it.** The header
row is painted one step off the terminal background; the selected tab is painted
in the terminal background itself. It is therefore the only thing on that row
sharing the body's colour, which reads as a tab continuous with the content it
controls — the browser/editor idiom — and is a stronger signal than raising it
would be.

The surface is **derived from the real terminal background**, not picked from the
palette. `lipgloss.BackgroundColor` gets the actual colour over OSC 11 and
`lipgloss.Lighten(bg, 0.10)` gives the chrome, so the bar picks up each theme's own
tint: Solarized's comes out blue, Gruvbox's warm, a light theme's is darkened
instead. A colour chosen from the ANSI text palette cannot do this, which is why
every fixed grey looked pasted on.

Three consequences of that, all of which need handling rather than hoping:

- **The query can fail** — no TTY, a terminal that ignores OSC 11, piped output.
  There is no colour to derive an elevation from in that case, so no bar is drawn
  and the selected flow is marked by weight and an underline instead. A fill
  picked from the ANSI text palette was considered and rejected: a colour that is
  not made of the background can only ever look pasted on to it.
- **Elevation needs truecolor.** On a 256-colour terminal the derived shade
  quantises to the nearest cube entry and can land back on a flat grey. Not
  detected yet — a terminal that answers the query but renders few colours gets
  the quantised bar.
- **The blank row below the header must stay body-coloured.** That is what the
  cutout connects to, and it is the whole reason the tab metaphor works. It is
  written deliberately today — `Model.View` emits `"\n\n"` between the header and
  the viewport — and `headerHeight` merely sizes the viewport to match. From here
  that row is load-bearing, so changing either without the other breaks the tab.

**The bar runs the full width** and appears only when there is more than one flow.
One flow renders flat, with no bar and nothing cut out, because a lone raised tab
looks like a control and isn't one — the same reason `ci` must not look like a tab.
`ci:` and `deploy:` stay lowercase, matching the header as it ships today.

**Status sits after each name**, matching `ci: ✓`. A dot immediately after
`deploy:` would read as the deploy group's own status, which is not a thing that
exists once flows are named.

**Colour is header-only.** A flow's badge is the same `✓` / `✗` / `·` the header
badges have always used, and the header has always been where clarity spends
colour — it is the summary, and it earns the colour that the per-row icons
deliberately forgo. Coloured `●` dots were explored and dropped, and the
reason is worth keeping: two filled dots differ only by hue, so to a red/green
colourblind reader — roughly 8% of men — the strip becomes a row of identical
grey circles with no way to tell which flow is broken. That is precisely the
failure the palette rule exists to prevent, and the strip's whole job is
answering "is anything red?" at a glance.

A hollow-passed / filled-failed pair was also considered, since filled-means-
attention survives greyscale and keeps the LED look. It was dropped for a
smaller reason: a healthy repo would then show a row of hollow rings, which
reads as "nothing has happened" rather than "all good", and the healthy state is
the common one. Per-commit rows are untouched either way: still
`✓ / ✗ / spinner / ·`, still carrying meaning by shape, still reserving red for
genuinely broken.

Flows are selected with `1`–`9` or `tab` / `shift-tab`. The arrow keys stay with
the body, which already binds them to viewport scroll; for three to six flows
direct jump beats cycling anyway. Selection is tracked by flow name rather than
index, so a config change that reorders the strip doesn't move the selection out
from under the user, and a flow that disappears falls back to the first rather
than leaving the body blank.

Clicking a tab is deliberately not supported yet. Bubble Tea's mouse capture
applies to the whole program, and turning it on breaks the terminal's own
text selection for the entire session — which matters in a tool people copy
SHAs and commit subjects out of. Worth revisiting only if click-to-select can
be had without that cost.

On overflow the strip degrades in stages, in the order things stop earning their
columns: the quit hint goes first — the only thing on the row carrying no
information — then names shorten to an ellipsis, then names go entirely, then the
group label, and finally the separators. A flow is never hidden and never
scrolled out of reach, since the stuck deploy is the one most worth seeing and
would be the one off-screen; at the extreme the strip becomes a row of bare
status glyphs, which is still the higher-priority half of what a tab carries.
(Herdr, whose tab treatment this otherwise follows, scrolls instead; that is
right for a multiplexer with unbounded tabs and wrong here.)

**A tab carries a name and a status, and nothing else.** Batch size and staleness —
how many commits are waiting, how long since a flow last deployed — were designed
as a second line under each tab and deliberately dropped. They cost a header row
for every repo to serve the multi-target minority, and they were the only reason
the strip needed to be more than one line.

The information is not lost, it is just read in the body rather than the header: a
flow's rows carry their own lead times, so a month-old batch reads as a month-old
batch the moment that flow is selected. What the strip gives up is seeing that
without selecting it. That is the trade, taken knowingly in favour of a header
that costs nothing.

### Why the badges follow the newest commit

The `ci: ✓ · deploy: ✓` badges on the header line summarise the whole branch in two characters, so which event they speak for matters. Each badge takes **the status of that stage on the newest commit that has resolved it** — not the latest event for the stage overall.

Two rules follow from that, and they pull in different directions:

- **Only `passed` and `failed` resolve a stage.** `started` and `skipped` are ignored, so a badge holds its colour through a retry instead of flickering to neutral every time a build kicks off, and a commit whose deploy was skipped doesn't blank out the badge the commit below it earned. A stage no commit has ever resolved renders neutral.
- **Recency is measured in commits, not timestamps.** Two pushes close together put two CI runs in flight at once, and the older commit's run can finish *last*: push a bad commit, revert it 60 seconds later, and the revert reports green seconds before the bad commit reports red. Ranking events by timestamp would then paint the header red for a commit that is no longer HEAD and was already reverted — while `deploy` stayed green, because the bad commit never emitted a deploy event to overtake with (its deploy job `needs: build`, so it was skipped). That asymmetry between the two badges is the signature of the bug. The newest commit with an answer is the one the header speaks for; a commit that has gone quiet on a stage defers to the one below it.

Per-commit rows are unaffected — they were always scoped to their own commit.

**The rule is applied per flow.** Each strip cell resolves this over that flow's
deploy events alone, so one flow's failure cannot paint another's badge. `ci` is
the exception and resolves over the whole commit, because it does not fan out.

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
your-app · ci: ✓ · deploy: ✓                                   press q to quit

HEAD
  · grace   wip notes                                              30s

CI Passed
  ✓ dave    update dependencies                                  4m 12s

Deployed
  deploying…
  ✓ alice   refactor user model                                  6m 30s
  deployed 5m ago
  ✓ frank   tweak homepage                                      24m 10s
```

Above is the single-flow case, which is what a repo with one deploy target
renders. A repo with several turns that header row into chrome carrying a strip of
selectable flows — see [The header and the deploy strip](#the-header-and-the-deploy-strip).

**Nothing is allowed past the right edge.** A commit subject longer than the
terminal used to push the lead time off-screen, and the viewport then scrolled
sideways to reach a column that is supposed to be pinned to the edge. The
subject is what yields: a clipped subject can still be read by widening the
terminal, whereas a lead time that isn't rendered can't be recovered at all.
Batch subheaders clip for the same reason.

**The week's stats shed whole facts rather than vanishing.** The `Deployed`
divider carries the week's deploy count and average lead time on its right.
Those used to disappear the moment the *decorative* trailing rule stopped
fitting, so they were lost at widths where they would still have rendered.

They now drop a fact at a time from the left — the week number, then the count,
leaving the average, which is the number worth reading:

```
 54 ─────Deployed ──── W1970-02  1 deploy  2m 00s avg ────
 46 ─────Deployed ────── 1 deploy  2m 00s avg ────
 38 ─────Deployed ──────── 2m 00s avg ────
 26 ─────Deployed ────────────
```

Whole facts or none: a fragment like `…s avg` spends columns saying nothing, so
below the width where the average fits intact the label goes and the rule takes
the space back.

**Clipping is measured in display columns over grapheme clusters.** Commit
subjects are arbitrary text from `git log %s` — CJK at two columns per rune,
emoji, ZWJ sequences, combining marks, and escape sequences, since nothing
sanitises them. Slicing by rune index against a column budget overflows it for
wide characters, can run past the end of the string, and can sever an escape so
its colour bleeds across the rest of the row.

Updates live as the underlying refs change. Polls the remote every 5 seconds (configurable) using git's lightweight `info/refs` endpoint to check whether the events ref or branch tip has moved, and only does a full fetch when SHAs differ.

### GitHub Actions as a live source

Teams who haven't yet added the `ezcdlabs/clarity` action to their workflows can point the TUI straight at the GitHub Actions API instead of the events ref. Add a `clarity.github` section to `.ezcd.json` mapping each stage to a workflow and the jobs that signal its start and completion:

```json
{
  "clarity": {
    "github": {
      "ci": { "workflow": "CI", "jobs": ["build", "test"] },
      "deploy": {
        "workflow": "Deploy",
        "jobs": { "started": ["approve"], "completed": ["release"] }
      }
    }
  }
}
```

`jobs` accepts either shape: a single array (the same set gates both start and completion) or `{started, completed}` for distinct sets. Either stage may be omitted. `git clarity init --github` writes this file interactively — it discovers your workflows and jobs via `gh` and asks which ones map to CI and Deploy.

When the section is present, `git clarity` derives its view from GitHub Actions runs (via the `gh` CLI, using its existing auth) rather than from `refs/clarity/events`.

### Local state: the cache directory

Clarity keeps its local state in `<repo>/.git/clarity`, overridable with `--cache-dir <path>` or `$CLARITY_CACHE_DIR` (flag wins over env wins over default). Point it at a mounted volume when running in an ephemeral container, otherwise incremental polling starts cold on every restart.

The directory holds:

| File                       | Contents                                                              |
| -------------------------- | --------------------------------------------------------------------- |
| `snapshot-cache.json.gz`   | Last rendered snapshot, replayed at startup so the TUI paints instantly while a fresh fetch is in flight |
| `github-runs.json.gz`      | Mirror of raw GitHub Actions run data, so polls stay incremental       |
| `ghsource.log`             | Non-fatal diagnostics from the GitHub poller                           |

All three are safe to `rm` — they're rebuilt on the next run.

`ghsource.log` is where to look when the GitHub source renders fewer events than expected. Transient failures (a rate-limited API call, one run whose `/jobs` fetch failed) are deliberately non-fatal: the poller keeps the stale cache and retries on the next tick rather than killing the TUI. Those failures can't go to stderr — mid-frame writes corrupt the TUI's alt-screen — so they land in this file, one timestamped line each. Fatal startup errors (bad config, `gh` not authenticated) still fail loudly on stderr before the TUI starts.

### Authentication

Uses whatever git auth the user already has configured (SSH agent, git credential helper). No tokens or env vars required when run inside a repo the user can already pull from.

---

## CI Reporting: `git clarity report`

Used inside pipelines, not by end users:

```
git clarity report [--sha <sha>] [--at <rfc3339>] <stage> <status> [<target>]
git clarity report [--sha <sha>] [--at <rfc3339>] affected|unaffected <target>
```

Examples:

```yaml
- run: git clarity report ci started
- run: ./build.sh
- run: git clarity report ci passed
- run: ./deploy.sh && git clarity report deploy passed || git clarity report deploy failed
```

### What it does

1. Validates that `<stage>` is `ci` or `deploy`, and `<status>` is `started`/`passed`/`failed`/`skipped` — rejects anything else. A `<target>` is accepted only for `deploy`, and only if declared in `.ezcd.json` when that file declares any
2. Resolves HEAD SHA (or reads `GITHUB_SHA` / equivalent when set)
3. Builds the event JSON (core fields + auto-detected `ci` metadata block)
4. Generates a unique filename: `<unix-ts>-<short-uuid>.json`
5. Optimistic push loop:
   a. Fetch `refs/clarity/events`
   b. Add the event file under `events/<sha>/<filename>`
   c. Commit with message `report: <sha> <stage> <status>`
   d. Push the ref with `--no-verify` so user pre-push hooks (which typically gate real code pushes) don't block clarity's bookkeeping push
   e. If rejected (not fast-forward): fetch, replay commit, retry
6. Exit

### Recovering a failed report

A dropped report does not correct itself. Nothing retries it later, so the stage stays unrecorded and the TUI shows that commit as in-flight indefinitely — a deploy that finished hours ago still spinning. The failures that cause this are usually transient and environmental, which means the recovery is genuinely just writing the same event again.

So every report echoes the fully-resolved form of itself before writing, and repeats it if the write fails:

```
$ git clarity report deploy passed
running: git clarity report --sha 9f9edc86... --at 2026-08-04T12:30:05Z deploy passed
error: failed to report deploy passed: push events ref: exit status 1
...

The event was not recorded — 9f9edc86 will keep showing as in-flight in
`git clarity` until it is. Re-run this once the problem is fixed:

  git clarity report --sha 9f9edc86... --at 2026-08-04T12:30:05Z deploy passed
```

Both, not one or the other: the echo is in the log before anything can go wrong, which is what a step that gets killed or times out needs, since it never reaches its own failure path. The repeat on failure puts the command where someone reading a red log is already looking.

The echoed command reproduces the event rather than approximating it. `--sha` and `--at` pin the two values that would otherwise resolve differently later — re-running a bare `git clarity report deploy passed` next week records today's deploy at next week's timestamp, which silently corrupts DORA lead time. Because the payload stores its timestamp as Unix seconds, a second-precision `--at` round-trips exactly, and event filenames are content-addressed, so re-running from the same environment collapses into a no-op instead of a duplicate.

One caveat: the auto-detected `ci` metadata block is part of that content. Re-running from a laptop, or from a different CI run, produces a second event file for the same logical event. The stage, status and timestamp are what the TUI renders, so the view is correct either way — but the events ref will hold both.

### Authentication in CI

Uses whatever git credentials are already available to the CI runner. In GitHub Actions the existing `GITHUB_TOKEN` is sufficient — no additional secrets needed.

### Which version the action installs

`uses: ezcdlabs/clarity@v1.2.3` installs git-clarity v1.2.3. A pin that is quietly ignored is worse than one that fails: builds drift with no diff to show for it, and pinning is normally a supply-chain control.

Honouring it is harder than it should be. `GITHUB_ACTION_REF` is the documented way to learn the ref an action was invoked at, but **the runner does not set it inside a composite action's steps** — neither the environment variable nor the `github.action_ref` context is populated, both come back empty. Measured, not assumed:

```
PROBE ctx github.action_ref = ''
PROBE env GITHUB_ACTION_REF = '<unset>'
PROBE path GITHUB_ACTION_PATH = '/home/runner/work/_actions/ezcdlabs/clarity/781bc347…'
```

The ref survives only in `GITHUB_ACTION_PATH`, where the runner unpacks the action, so its last segment is the ref. Resolution order is therefore: an explicit `version:` input, then `GITHUB_ACTION_REF` (still consulted first, so this keeps working if the runner ever starts setting it), then the ref parsed out of the unpack path, then the latest release.

A ref that is not a plain version tag — a branch, a commit sha, or a local `uses: ./` checkout, whose path is the workspace rather than an unpack directory — means "track the newest release", because no release exists under that name to install.

This regressed silently and shipped that way: for a period, every pinned `uses: ezcdlabs/clarity@vX` installed whatever had shipped most recently. Nothing caught it because the pinned-tag branch was the one path no test executed — the action self-test invokes the action as `uses: ./`, which has no ref in its path, and the local harness hardcoded an empty ref. Both now clear the ref explicitly *and* there are two tests that cannot pass without it: a hermetic resolution table in `internal/actioninstall` that runs on every push, and a workflow job that installs from nothing but a ref-carrying path and asserts the binary reports that exact version. The version it pins is deliberately an old one, so "we installed the latest release" cannot pass as success.

### Concurrency

Two pipeline jobs reporting simultaneously cannot corrupt each other's data because they write different files. The only contention is the fast-forward push race on `refs/clarity/events`, handled by the same retry loop pattern pushq uses for its state branch.

### The target argument

An optional third positional argument names the deploy target:

```yaml
- run: ./deploy-web.sh && git clarity report deploy passed web
- run: ./deploy-ios.sh && git clarity report deploy passed ios
```

Positional rather than a flag because it is a fixed-arity part of what is being
reported, and because `report deploy passed ios` is what a pipeline step should
read like. The batch JSONL form carries it as a `"target"` field, and the
`ezcdlabs/clarity` action takes a `target:` input alongside its existing
`stage` / `status` pair.

**What may be written.** A target is an identifier: letters, digits, and
`. _ / + : @ -`, not starting with a dash, up to 64 characters. Whitespace,
control characters and ANSI escapes are refused. Strictness here is cheap and
the alternative is not — the events ref is append-only and content-addressed,
so a target written once is a flow label forever, with no edit and no delete. A
stray `" ios "` becomes a second flow the declared `ios` can never claim, an
embedded newline splits the deploy strip across two rows, and an escape
sequence is emitted straight to the reader's terminal.

The accepted set is deliberately the shell-word-safe set, because the echoed
recovery command is a command line and its whole purpose is being re-runnable
verbatim.

An explicitly empty target — `report deploy passed ""`, which is what an unset
`$TARGET` expands to — is an error rather than a silent untargeted deploy.
Omitting the argument is how the untargeted deploy is reported.

When `.ezcd.json` declares flows, a target none of them claims is refused at
write time, so a typo fails the pipeline that made it instead of appearing days
later as a flow nobody deploys to. A repo declaring nothing keeps the open
vocabulary, since discovery is the zero-setup path.

Supplying one for `ci` is rejected, with the reason rather than just the rule:

```
error: ci takes no target — "ios"

  A target that can pass CI on its own isn't integrated with the rest of
  the repo. CI answers one question for the whole commit; only deploy
  fans out.
```

This is an arity check on the third argument. Stages and statuses are already
enforced against closed sets in `internal/report`; what targets cannot yet be
checked against is a *set*, since a repo declaring no flows keeps an open
vocabulary by design.

Because the commit list is shared across flows, a commit that never triggered the
iOS pipeline still sits above the last iOS deploy and reads as not-yet-shipped.
That is true and should stay visible. What it must not do is pollute that flow's
lead time — see [Candidacy](#candidacy-affected--unaffected).

### Reporting candidacy

```
git clarity report affected <target>
git clarity report unaffected <target>
```

A second grammar under the same command, dispatched on the first token — which is
unambiguous because `affected` is not a stage. Git itself does this throughout.
Forcing candidacy into the three-slot stage/status/target shape would need a
filler word and buy nothing, because candidacy has no status: it is a property,
not an outcome.

The record lands in the `scope/` tree rather than `events/`. Everything else is
shared with stage reporting: the same `--sha` and `--at` overrides, the same
target format rules, the same content-addressed filenames, the same optimistic
push loop, and an echoed fully-explicit command that reproduces this grammar
just as it reproduces stage reporting's. One invocation writes to one tree, so
there is no cross-tree push to race.

One gap is open, deliberately: the grammar names a target, and the untargeted
deploy has no name to give, so candidacy is expressible only for named targets.
A repo whose single flow is the untargeted one has nothing to exclude a commit
*from*, so the gap only becomes real for a repo that both names some targets and
leaves one unnamed — at which point naming it is the smaller change.

### Explicit overrides for migration

Two optional flags let a caller override the SHA and timestamp clarity would otherwise infer:

- `--sha <sha>` — write the event under this commit SHA instead of resolving from `GITHUB_SHA` / `CI_COMMIT_SHA` / `HEAD`. Highest precedence — beats env vars.
- `--at <rfc3339>` — record this timestamp in the event instead of `time.Now()`. Format is RFC3339 (e.g. `2024-04-08T15:48:54Z`).

The primitive use case is **backfilling history** when adopting clarity on an existing repo. A migration script (e.g. one that queries the GitHub Actions API for past workflow runs) shells out to `git clarity report` once per historical run, passing `--sha` and `--at` to recreate events for commits clarity wasn't installed on at the time.

Backfilled events use the same JSON schema and the same ref as live ones — the TUI cannot distinguish them.

### Batch mode for backfill

For large backfills (hundreds of historical events) the per-event push round-trip dominates wall-clock time. `--batch` flips `report` into a stdin-driven mode that does **one** fetch + **one** push for the whole stream:

```
git clarity report --batch < events.jsonl
```

Each line is a JSON object with all four fields:

```json
{"sha":"abc1234...","at":"2024-04-08T15:48:54Z","stage":"ci","status":"passed"}
```

Blank lines are skipped; the first malformed line aborts with a `line N: ...` error. A batch lands on `refs/clarity/events` as a single commit with message `report: batch of N events`. Backfill events in batch mode do not get CI metadata auto-attached — the local env doesn't describe the historical pipeline that produced them. Live reporting should continue to use the single-event form so each event lands as its own audit-able commit.

### Migrating from GitHub Actions

`scripts/generate-backfill.sh` is an interactive generator that reads your repo's workflows via `gh`, asks which jobs mark the start and end of CI (and optionally Deploy) — modelled as the `needs:` list of a hypothetical clarity step — and emits a tailored backfill script. The generated script aggregates timestamps and conclusions across the chosen job set per run, then pipes the whole event stream through `git clarity report --batch`.

**Recommended order: integrate first, backfill after.** Add the `ezcdlabs/clarity` action to your workflows and merge that change before running the backfill. The live action then handles the currently-in-flight run, and the backfill handles strictly-historical commits — any run still in progress when the backfill executes gets skipped (the generator gates on a paired terminal status), and that's correct because the live action will emit its own paired events when the run completes.

Run it from the root of your repo (requires `gh` authenticated, `jq`, and `git-clarity` v0.1.2+):

```bash
curl -sLO https://raw.githubusercontent.com/ezcdlabs/clarity/main/scripts/generate-backfill.sh
bash generate-backfill.sh > backfill.sh   # answer the prompts
bash backfill.sh --dry-run | less          # review the JSONL stream
bash backfill.sh                           # execute (single push)
```

Re-runs are safe: event filenames are content-addressed, so a second `bash backfill.sh` for the same input is a no-op (no duplicate files, no new commit on the events ref).

The generator is the only GitHub-specific piece of the migration; everything downstream of the JSONL stream is platform-neutral. Adapting it for another CI provider is a matter of rewriting the discovery and event-emission steps to target that provider's API — the `git clarity report --batch` consumer stays the same.

---

## Weekly Metrics: `git clarity metrics`

`git clarity` answers *"is main green right now?"* — a live view of the most recent commits. It cannot answer *"are we getting better?"*, because trend is invisible in a list of commits. That is a different question, for a different reader, on a different cadence, and it gets its own command rather than a mode of the TUI.

```
git clarity metrics                      # interactive, opens on the first flow
git clarity metrics --deploy ios         # open on one deploy flow
git clarity metrics --weeks 26           # a longer window
git clarity metrics --limit 0            # read the whole history, not the default window
git clarity metrics --plain              # print once and exit
```

`tab` / `shift+tab` switch flow, `1`–`9` jump to one, `↑`/`↓` scroll a long history, `g` returns to the newest week, `q` quits.

The top bar carries what the reader needs to know they are looking at: the repo, the span of weeks on screen, and which flow is selected. A repo with no targets still gets one — the strip is what varies with targets, the bar is not.

It deliberately does **not** carry ci/deploy status, and the deploy strip here shows no status glyphs. Those answer *"is main green right now?"* — the live view's whole question — with a value frozen at launch that goes stale while this view is read. A badge that looks live and is not is worse than no badge, and leaving glyphs on the tabs while removing the badges would apply the reasoning half-way. The strip is a selector here, not a status display.

The footer lists only keys that change what is on screen; every terminal program quits, and saying so spends the line on the one hint carrying no information.

### Lead time mode

The weekly view obeys `clarity.leadTime` exactly as the commit list does — it builds its lens the same way, so `pipeline` measures from the first pipeline event here too. The mode changes the distribution and therefore the axis, not only the average, which is a wider surface than the deploy strip has.

That is guarded by a test crossing from a config file to the derived numbers, on both lens paths, because this is the shape of the bug that produced the renderer rule: core computed every mode correctly and had tests proving it, both renderers rebuilt their own groupings from the raw snapshot, and the setting did nothing while every unit test passed.

`--weeks` bounds what is drawn; `--limit` bounds what is read. They are different windows, and the second can cut the first short — when it does, a footnote says so, because an aggregate that is a few deploys short is wrong in a way no reader can see.

One row per ISO week, newest first: a box plot of that week's lead times on the left, a bar of that week's deploy count on the right.

```
             lead time  ·  median, p25–p75                                    deploys
  W2026-39       ├─────▓▓▓▓█▓▓▓───────────────┤                █████████████████  18
  W2026-38     ├───▓▓▓▓█▓▓────────────┤                   ██████████████████████  24
  W2026-37          ├─────▓▓▓▓▓▓▓█▓▓▓──────────────────┤                ████████   9
  W2026-36        ├─────▓▓▓▓▓▓█▓▓────────────────┤                    ██████████  11
  W2026-35      ├────▓▓▓▓█▓▓▓───────────────┤                    ███████████████  16
  W2026-34     ├───▓▓▓█▓▓───────────┤                        ███████████████████  21
            0─────────2h─────────4h─────────6h────────8h
```

The two halves are one signal. A bad week pushes the box right *and* pulls the bar back, so degradation reads as ink migrating rightward across the whole row — which is the entire reason the bars are mirrored rather than growing from a shared left edge.

### Interactive, but not live

Two different things, easily conflated: **live** means a watcher polling the remote; **interactive** means an event loop responding to keys. This view is the second without the first.

It reads one snapshot and never polls. There is no tick, no re-fetch, and no staleness to signal — so it can afford to read far more history than the live view ever could.

It is still a Bubble Tea program, because the deploy strip is a control. Rendering the tab bar while requiring `--deploy` to change flows would be showing a control that does not work. Switching flows costs nothing: every flow is already derived on the `View`, so selection is local UI state with no round trip to the Lens (see `FlowView`).

Not a mode of the main TUI, though. A *view* selector there would have had to coexist with the deploy-target tabs as a second orthogonal dimension, and the tick budget would have capped the history.

Piped or redirected output falls back to a single static render, the same way `--plain` does for the live view — a tab bar is meaningless when there is no keyboard. That path uses a fixed width rather than probing the terminal, so piped output is byte-identical run to run and there is nothing to measure when the destination is a file.

Because there is no tick to keep cheap, the one-shot read takes a far larger commit window than the live view's default. The cost is paid once at startup.

### Box plots, and medians

Lead times are strongly right-skewed — one commit that sat over a weekend drags a mean badly — so the headline is the median, and the box plot gives it for free. Note this is a change in published numbers: `WeekStat.AvgLead` is a mean today.

Spread matters as much as speed. A team with a 2h median and a three-day p90 has a problem the median alone hides, which is why the whiskers stay and an interquartile band is not enough.

`WeekStat` therefore needs quantiles it does not currently carry — `Min/P25/P50/P75/Max` on the View, not raw samples, so the renderer stays dumb per the rule in "Acceptance tests".

**Below five deploys a week, plot the points instead.** A box plot over two deploys is theatre: quartiles of two numbers mean nothing. The strip form degrades honestly — two deploys look like two deploys.

That needs the individual values, not the summary: below the floor the quantiles are interpolations between two or three real numbers, so drawing them would put a smear on screen where there were only two deploys. `LeadQuantiles.Samples` carries them, and only below the floor, so it cannot grow with a busy week.

### One axis per flow, chosen from that flow's data

Per-row autoscaling would make the rows incomparable, which is the whole point of the view. A fixed ceiling is no better: it wastes the width for a fast team and squashes a slow one.

The axis is per flow, for the same reason `Weekly` is: a store review and a web deploy need different scales, and one axis across both squashes the faster of them into a column. Within a flow it is shared across every visible week, and derived from that flow's pooled lead times:

1. Take Q1 and Q3 of the **log** of every lead time in the window.
2. Fence = `exp(Q3 + 1.5 × IQR)` — Tukey's rule, which is the box plot's own definition of an outlier, so it answers exactly the question being asked.
3. Axis maximum = the next readable boundary at or above `min(largest value, fence)`.

It is computed from the raw lead times inside `core`, not from the per-week quantiles. Pooling quantiles instead would give a two-deploy week the same weight as a forty-deploy one and bias the fence. The result reaches the renderer as `FlowView.LeadAxis`, so nothing downstream re-derives it.

**The log step is load-bearing.** A raw Tukey fence clips tidy data, because a long tail is normal for lead times and the raw fence reads ordinary skew as a crowd of outliers. In log space the fence is multiplicative, which is how lead times actually vary: *five times the typical commit* is an outlier, *two hours longer than the typical commit* is not.

A percentile is the wrong instrument for the same reason it looks right. p95 is a quantile, so there is always data above it: it clips whether or not anything deserves clipping.

The result satisfies both constraints at once — tidy data is never clipped, and a single 74-hour outlier is clipped without compressing every other week into two columns.

Rounding to a readable boundary does two jobs: the ticks read as durations rather than arbitrary fractions of the data, and the axis stops moving every time a deploy lands, so weeks stay comparable between runs and not only within one render.

**The `→` marker and the `+` on the final tick must agree.** A value landing exactly on the axis is shown, not clipped. Drawing the arrow on `>=` while setting the `+` on `>` makes the view contradict itself about whether it is showing everything.

### Whole weeks only

The window is a number of whole weeks, not a commit count. A week the window cuts through is **dropped, not drawn partial** — its deploy count and its distribution would both understate, and there is no honest way to render that in a bar.

This is the `WeekUnknown` principle from the deploy strip, taken further: rather than inventing a way to say "we do not know", do not show the row. It removes a state from the code as well as from the screen.

A commit-count limit would also make how far back you can see depend on how busy the repository was, which is not a useful property for a trend view.

### Colour and weight

- **Box plot** — blue (ANSI 12), the existing accent. The interquartile box is `▓`, the median a solid `█`.
- **Deploy bars** — **ANSI 8**, the same grey as the `lead time` label, as full blocks.
- **Counts and week labels** — the terminal's default foreground.

Grey for the bars is not an absence of choice. Green already means *passed* and amber *in-flight*, so either would assert that a deploy count is good or bad; grey carries no verdict, which is right for a measurement. It is also already in the palette, so there is no 256-colour dependency and no fallback to write, and it works on light and dark backgrounds without detection.

The concern that grey would collide with the chrome — the axis and labels are ANSI 8 too — does not survive contact: a solid 22-column block and thin glyph text read as completely different things at identical colour. Shaded variants (`▓`, `▒`) were tried and are too weak, especially on light backgrounds, where short bars nearly vanish and the length comparison is lost.

### Narrow terminals squeeze the plot, they do not replace it

The box plot stays a box plot down to about 10 columns. Precision degrades, but which weeks are better or worse remains readable, which is what the view is for. Two things shed before the plot does:

- **Axis ticks**, which collide first — intermediate labels drop, leaving the ends.
- **The empty-week note**, `· no deploys`, which becomes `·`.

An earlier design fell back to text (`3h 18m (2h 08m–5h 55m)`) below a width threshold. It reads well and is more precise, but it is not scannable down a column, and scanning is the point.

---

## Mobile: native apps over the same core

The TUI and the phone apps answer the same question and share everything that
decides the answer. What differs is only the drawing.

### Why mobile needs its own source

The CLI's rule is that **go-git is never used to read; git is** — git is the only
thing that knows how a working copy was cloned, and a blobless partial clone, a
shared object store and a shallow graft each break a naive reader in a different
way (see "Clone shapes and the object store").

A phone has no git binary and no working copy, so that rule cannot apply and does
not need to: clarity did the cloning itself, so it already knows the shape.
`mobile/internal/gitsource` is a go-git `core.Source` that fetches the events ref
in full and the branch at a depth, and reads back what it fetched. It is the one
place in the project go-git reads, and it is allowed to because nothing else
produced the store it reads from.

The shallow boundary is the subtlety. `Repository.Log()` walks straight past a
graft and fails on the first missing parent, but `Storer.Shallow()` records where
the boundary is — so the walk in `mobile/internal/gitsource/walk.go` stops there
deliberately and reports the history as truncated, which is the same thing the
TUI's commit limit does.

### Light mode, for the reason the terminal already had it

The TUI works on a light terminal without a line of code about light terminals,
because it never picks a colour: it asks for ANSI 1/2/3/8/12 and the emulator
answers. The phone had the same structure — nine roles, named — but only one
answer, so it stayed dark under a light system and left the status bar
unreadable.

So there are two answers now, behind a composition local. The light palette is
not the dark one lightened: a pastel green legible on near-black vanishes on
near-white, so each role is answered again at a weight that carries against a
light ground, which is what a terminal theme does when it maps the same codes
for a light profile.

**Two tones, and which one is the brand's matters.** The chrome — the top bar,
the flow strip, the ground the content is laid on — is the mark's navy exactly,
so the app opens into the colour the splash handed it. The commits sit on a
second, darker navy, because a reading surface should be the extreme of the
pair: every role gains contrast against it, and no role loses any.

The step between the two is deliberately small, which is why the content has
turned top corners. Close tones plus a shape read as a sheet laid on a ground;
close tones alone read as two panels that failed to match. The pattern is
Google Messages', and it is also one the TUI already has — it paints the header
row one step off the background and cuts the selected flow out of it, so a tab
is continuous with the body it controls. On the phone the selected tab is cut
to the content tone and the sheet begins immediately below it, so that idiom now
works for the reason it was invented.

Both navies are the same hue and saturation at different lightnesses, *derived*
rather than picked beside each other — exactly what the TUI does with the
terminal's real background, where a grey chosen next to a blue can only ever
look pasted on to it. The mark's yellow and blue replace the ad-hoc ones, which
were within a few points of them anyway.

**Light takes only the navy, as ink.** The mark's yellow on white is barely a
colour and its blue is a highlight rather than something legible, so neither
carries. The navy does: as an off-black it holds the brand's hue at 17:1 against
the page, which is the one place a light theme can carry the identity without
giving up contrast.

This is the one deliberate departure from the terminal, and the asymmetry is the
point. The TUI never sets a background — it inherits the terminal's, which is
why it has no brand colour and cannot have one. The phone owns its whole
surface, so it can carry what the terminal structurally cannot.

Three things had to agree for the system bars, and only all three together fix
it: the window background follows the system (a `values-night` override rather
than a hardcoded dark), the activity's theme parent follows it too, and
`enableEdgeToEdge` picks the bar icon colours from the system's own dark-mode
setting — which was right all along, and was drawing dark icons over a window
painted dark.

Nothing may reach for a literal colour. Anything that does is a thing that
looks wrong under one of the two themes, and the only way to find out is to be
holding the phone.

### The phone follows the terminal's two colour tables

Not one table. The header is the summary and earns the colour the per-row icons
deliberately forgo, which is why `ci: ✓` is green and a tick in a commit row is
grey. Red is spent only on a failure that is still breaking something: once a
newer commit has gone green the older failure is history, and goes grey with
everything else. A started stage shows as unresolved in the header rather than
as progress — the header is a verdict, and "something is happening" is not one.

A screen of green ticks would spend the one colour that means "look at this" on
the state needing no attention at all, and the first mobile build did exactly
that.

A commit row carries what the terminal's does — status icon, author, subject,
lead time — over two lines rather than one. This is the one place the phone
departs from the row and it is the screen's fault, not a preference: at this
width a single row clips the subject, which is the thing the list is being read
for. So identity and lead time share the top line and the subject gets the full
width below, indented to line up under the author. Section labels, batch
subheaders and week dividers all indent to that same column, as the terminal's
dividers do.

No sha — the terminal does not print one, and nothing on a phone can be copied
out of a list row anyway. No commit age either: the only timer on a row is the
lead time, and a second one beside it invites the reader to work out which is
which. `Commit.age` was removed rather than left unrendered, because a
preformatted string nothing draws is a string free to drift.

A deploy subheader keeps its time inline — "live on production · deployed 4m 43s
ago" is a sentence, and pushing the time to the right edge turned it into a
column and broke it.

### Weeks are divided where the terminal divides them

The Deployed section carries this week's throughput on its own rule — "W2026-40
 3 deploys  2h 14m avg" — and every other week is named above its first batch.
Both strings come from `core.WeekDividerLabel`, the same function the TUI and
the plain renderer use, so the format cannot drift between them.

Which batch carries a divider is a decision rather than a position: batches are
ordered by commit and not by deploy time, so a redeploy of an older commit puts
a week out of sequence, and a client tracking only "did the week change" would
name the same week twice. The summary is empty when the window was truncated
through this week's bucket — the count is then unknown rather than zero, and
"0 deploys" printed above the very deploys it denies is worse than nothing.

### Two pages, not a stack

The repository is the app. The list of repositories is the page beside it, the
way Slack puts its channel list beside the conversation: swipeable from anywhere
on the screen in either direction, with a button for anyone who does not go
looking for a gesture.

Deliberately not a drawer, and not a navigation stack. A drawer opens from an
edge and sits over what it covers; a stack makes the list a place you travel to
and the repository a place you come back from. Here they are peers, and the
selection is model state rather than navigation state — so sliding to the list
and back puts you where you were, and the first launch, a tap, and the fallback
after removing a repository all land you in a repository by the same route.

### What ticks, and what refetches

The TUI is live; the phone has to be too, without being a battery.

Durations cross the boundary preformatted, which is right for the instant the
view was built and wrong a second later. So the model advances a clock once a
second and every timer on screen is drawn against it — one clock, so the whole
screen stays consistent with itself. A live lead time carries the instant it
started counting, which is what lets a client keep it honest without asking Go
to walk the commit graph again every second.

The formatting itself stays in Go, exposed as `Elapsed`. The alternative was
reimplementing the rule in Kotlin and in Swift: two more places for "3m 29s" to
drift into "3:29".

The open repository refetches every thirty seconds. Both the clock and the
refresh stop when the window stops being visible — a timer nobody can see only
spends battery, and a fetch nobody asked for spends their data too.

A *background* refresh that fails says nothing when there is already a view on
screen. The alternative is an error bar reappearing every thirty seconds for as
long as you are on a train, over a dashboard that reads perfectly well. The
failure is reported when you asked for it, or when there is nothing behind it to
read.

### Where the proto boundary sits

Between the core and the UI, not between the app and the network.

`proto/clarity/v1/view.proto` carries a presentation model: groups already
formed, statuses already resolved, lead times already computed and preformatted.
The rule is **decisions in Go, geometry native**. Anything that decides what is
true crosses the boundary settled; anything that decides how wide, what colour or
which glyph is the platform's business.

That split is not tidiness. Most of the terminal renderer's logic exists because
a terminal quantises to character cells — reserving a column for a clamp arrow,
guaranteeing a one-cell minimum bar, shedding tick labels that would collide.
None of it has an analogue at pixel resolution, and none of it should cross.

What crosses is the terminal's own structure, not a flattening of it. Three
lifecycle bands — HEAD, CI Passed, Deployed — always all three and always in
that order, because they are the frame the view is read against: an empty
Deployed says "nothing has shipped", which is an answer, and a client handed
only the non-empty bands could not tell that from a band that went missing.
Inside them, each deploy is its own batch with the subheader the TUI writes
above it, and exactly one batch is marked live — what is running in production
now, as against what merely happened.

Getting that wrong is what the first mobile build shipped: five deploys became
five sections all labelled "Deployed", so the phone repeated the word down the
screen while the terminal says it once.

A commit carries its CI status and no deploy status, which is not an omission.
The terminal renderer has never drawn one: *"the deploy status is implied by the
section the row sits in"*. Two marks per row say the same thing twice, so the
field is absent rather than ignored.

Protobuf also sidesteps gomobile's type system. Only strings, bytes, ints, bools,
errors and struct pointers cross an FFI boundary it generates, so a rich view
would otherwise need a hand-written wrapper per platform — three places for the
same shape to drift. Encoded bytes need none, and one schema generates all three
sides.

### Structure

```
mobile/
├── core/                 # the FFI surface gomobile binds. No logic.
├── internal/
│   ├── gitsource/        # go-git source: fetch, then read what was fetched
│   ├── present/          # core.View -> clarity.v1.View
│   ├── keys/             # the device's ed25519 identity
│   └── registry/         # which repositories this device tracks
└── apps/
    ├── android/          # Gradle project, Kotlin + Compose
    └── ios/              # Xcode project, Swift + SwiftUI
```

`mobile/core` is deliberately empty of decisions — every method translates an
FFI-friendly shape into a call on `mobile/internal/*`, which is where behaviour
lives and where it can be tested without a device. Generated protobuf is
committed, in Go and in each app's own language, the same way `proto/gen/go` is.

So are the app icons, from the ezcd mark in `mobile/internal/icon` — written out
at every size both platforms want by `go run ./mobile/tools/gen-icons`.

### The mark is geometry, not a file

Three identical checks offset along one axis on a navy disc: white at the back,
then yellow, then blue in front. The two at the back are clipped to the disc —
the white one runs off its lower left and is cut there — while the blue one is
allowed past the edge at the top right. That asymmetry is the composition rather
than an accident of drawing: the mark reads as layers, and layers need something
to be in front of.

The constants came off the original artwork by fitting the *edges* of each arm,
not by finding its corners. Antialiasing erodes a sharp corner by a pixel or
two, so corners read as consistently inside where the geometry really is, while
a long straight edge fits exactly. Both arms turn out to sit at precisely 45°,
the ends are cut square, the elbow is a mitre, and the checks are 33px apart on
a 180px mark.

The one thing that could not have been got by eye is the **moat**: every check
carries a navy clearance that cuts the one behind it short. Two parallel arms
are already far enough apart to show navy between them, but where a back check's
long arm passes behind a front check's short arm they would meet and merge into
a single shape. Modelling the moat took the reconstruction from 3.4% of pixels
wrong to 1.6% — the difference between three checks that happen to be near each
other and a stack that reads as layers.

Holding it as geometry rather than as a PNG is what lets one definition serve an
Android adaptive foreground, two legacy densities, an iOS app icon and a splash
screen, and makes a change to it a readable diff.

### Each format wants the mark differently

Three grounds, because the formats disagree about what may be behind the mark.
Transparent outside the disc is the mark as the brand uses it. Navy to the edges
is for anywhere transparency is refused — an iOS app icon is rejected outright
if any pixel is see-through — and the navy reaching the corners is the same navy
the disc is made of. Ink only, with every navy pixel including the moats left
transparent, is what an adaptive icon's foreground is: Android paints the
background layer and moves it independently, so a foreground carrying its own
disc would be a second navy that cannot follow the first.

The adaptive foreground is the one place the blue check is clipped like the
others, and the one place sizing is a judgement rather than a rule. Android
guarantees only the inner 66dp of a 108dp canvas, but it shows 72dp — and the
strict safe zone is for a mark with a subject floating in space, which this is
not. Sized to it, the mark sat adrift in a ring of empty navy and was barely
legible at 48dp, which is the size that decides whether an icon works. So the
disc is mapped onto the mask instead: the mask plays the part the disc plays in
the mark, a tighter launcher crops navy the way a tighter mask crops any
circular logo, and the blue overflow goes — because a check cannot be seen
escaping the very thing that is cutting it.

A splash screen is the mark with the navy pulled out to the screen. Android 12
took splash screens over and ignores a window background set for the purpose, so
there are two: a layer-list for the launch theme on older versions, and the v31
attributes above it. Only the launch style is duplicated into `values-v31`, not
the app's theme — night mode outranks platform version when Android picks a
resource folder, so a theme copied there would lose its dark variant on every
modern device.

The bound archive is not committed. `scripts/bind-android.sh` produces a 33 MB
`.aar` of per-ABI native code; checking it in would make every bind a binary
diff.

### A bridge interface, so the UI is testable without a device

Each app talks to an interface — `ClarityBridge` on Android — not to the
generated bindings. Behind it sits either the real bound core or a fake, and the
screens cannot tell which.

This is what makes the interesting behaviour testable on a laptop. The state
machine in front of the bridge decides things that only show up on a bad day: a
rejected URL keeps you on the add screen so a pasted clone URL is not lost; a
failed refresh keeps the view it already had, because on a phone yesterday's
pipeline beats an empty screen with an error on it; a first open of a
never-fetched repository reports why the *fetch* failed, not the "reference not
found" that reading an empty store always produces.

The fake makes every one of those a unit test rather than something you find out
about on a train.

What the fake deliberately does *not* do is re-decide anything the core already
decided. The registry validates URLs and writes the message explaining a
rejection; the app surfaces that message verbatim. A second opinion in Kotlin
could only disagree with it.

### Authentication: a key per device

The app generates an ed25519 keypair on first use and shows the
`authorized_keys` line to paste into whichever host the user uses. There is no
provider integration, no OAuth app and no server, because **clarity is not a
GitHub tool** — a key the user installs themselves works on GitHub, GitLab and a
box in a cupboard equally. The private half never leaves the device, and an
https remote gets no credential at all.

The host key is checked against a `known_hosts` the app keeps itself. go-git's
default is to read `~/.ssh/known_hosts`, which on a phone does not exist, so
every fetch fails before it reaches the network: *unable to find any valid
known_hosts file*. The obvious shortcut is `ssh.InsecureIgnoreHostKey`, and it
is the wrong one — a tool whose job is telling you the truth about your pipeline
should not accept whatever answers on port 22.

So the app does what an ssh client does on first contact: trust the key it is
shown, write it down, and refuse anything different afterwards. That does not
protect the very first fetch, which nothing short of a key the user types in
ever could. It does mean a host whose key changes under you is reported rather
than silently accepted, which is the case that actually happens.

### Nothing that crosses the boundary may be empty

gomobile cannot carry a zero-length byte slice. Its `fromSlice` turns one into a
null `jbyteArray` on Android and a nil `NSData` on iOS, and a protobuf message
with no fields set encodes to exactly zero bytes — so a perfectly legal payload
arrives as null and the decoder throws. The first thing a new install asks for
is the repository list, which is empty, so this is the common path rather than
an edge of it.

Both `View` and `RepoList` therefore carry `generated_unix_seconds`, always set.
It is not only ballast: a mobile client holds the last good view across a failed
refresh, so how old the thing on screen is happens to be exactly what the user
needs to know at that moment.

### iOS without a Mac in the loop

Development happens on Linux, where there is no `swift`, no `xcodebuild` and no
iOS SDK, so iOS source is written blind. The correctness gate is a macOS CI
runner, added with the first iOS commit rather than after it — otherwise the
first person to open the project on a Mac inherits every mistake at once.

Two consequences follow from that, and both are deliberate:

**The `.xcodeproj` is generated, not committed.** `project.yml` is the source of
truth and XcodeGen produces the project from it. A `pbxproj` is a UUID soup that
cannot be reviewed and cannot be opened at all on the machine it would be edited
from.

**The Swift protobuf is generated in the build, not committed.** This is the one
place the "commit the generated output" rule does not hold, and the reason is
the same one: `protoc-gen-swift` is itself a Swift binary, so it cannot run
where the output would be committed from. Committing output nobody present can
regenerate is worse than regenerating it, so `scripts/gen-ios-proto.sh` runs on
the macOS runner before every iOS build — which also means the app is always
compiled against the current schema rather than a stale copy of it.

---

## Repository Structure

```
ezcdlabs/clarity/
├── cmd/git-clarity/      # main binary (named git-clarity for git extension discovery)
├── clarityrefs/          # public package: read/write clarity's events
├── internal/
│   ├── adapters/tui/     # terminal UI rendering
│   ├── report/           # the `report` subcommand logic
│   ├── refs/             # fetch refspec configuration
│   └── ci/               # opportunistic CI env var detection
├── proto/                # the presentation model the mobile apps render
├── mobile/               # go-git source, FFI surface, and the native apps
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
    Target string             // deploy only; "" is the untargeted deploy
    CI     map[string]string  // optional, may be empty
}

func ReadEvents(repoPath, sha string) ([]Event, error)
func ReadAllEvents(repoPath string) (map[string][]Event, error)
func WriteEvent(repoPath, remote, sha string, event Event) error

// Candidacy lives in the sibling scope/ tree, not in events/.
type Scope struct {
    Target   string
    Affected bool
    Time     time.Time
}

func ReadAllScope(repoPath string) (map[string][]Scope, error)
func WriteScope(repoPath, remote, sha string, scope Scope) error
```

Reads operate on the local events ref only — callers (typically the watcher) fetch first, via `FetchEventsRef`, which brings the complete object graph (see "Clone shapes and the object store"). `WriteEvent` fetches the remote ref before writing and pushes after, retrying on fast-forward rejection so concurrent reporters never lose events. Repository handles are passed as paths rather than `*git.Repository` so callers don't have to depend on a specific go-git version.

This mirrors the pattern pushq uses with its `pushqrefs` package — the ref format is the public contract, exposed via a Go package.

---

## What CI Covers, and Where

Six stages in one workflow, ordered so each answers a question the one before it cannot.

- **Test / Integration (SSH)** — `go vet`, `go test`, and the SSH-backed suite. Linux only.
- **Action** — the composite action on Linux, macOS and Windows, on every push and pull request.
- **Android** — `gomobile bind`, then the app's unit tests and a debug build.
- **iOS** — generate the Swift protobuf and the Xcode project, bind, and run the unit tests in a simulator. macOS only.
- **Release** — tag and publish, gated on all of the above.
- **Smoke** — install the version just published, on all three platforms.

The mobile jobs gate the release even though nothing they build is released. A
tag is a statement about a commit, and a commit where the bind no longer
compiles is not one to make that statement about. The iOS job gates it for a
second reason: it is the only thing in the project that compiles the Swift at
all, so an ungated version of it would be a check nobody has to look at — which
is exactly how `aux.go` survived four months. It is also the only thing that
proves clarity's dependency graph still crosses an FFI boundary: the bound
archive is not committed, so a dependency that reaches for `os/exec`, cgo, or a
syscall Android does not have fails here and nowhere else.

### Why the action is tested before the release and smoked after

They are different questions, and one workflow cannot answer both.

**Before**: does the repository check out, and does the installer work? Both exist in the commit under test, so this belongs pre-merge — and it is gated on nothing, because the job starts by checking out the whole repository and *any* file in it can break that. It installs an old release deliberately: the binary is incidental, the installer is the subject, and the version this commit will become does not exist yet.

**After**: is the thing we just published actually installable? Structurally unanswerable earlier — before the release job runs there is no such artefact, so a pre-merge job can only ever verify some older binary. Nothing checked that a freshly cut release was installable at all until this existed. It is skipped when no release was cut, since a docs- or test-only push has nothing new to smoke.

### Why the action job carries no path filter

It used to live in its own workflow, gated on `action.yml`, `scripts/action-install.sh` and the workflow file — on the assumption that only a change to the action can break the action's test.

That assumption is false, and it cost four months. `internal/core/aux.go` matched none of those paths, so the workflow did not run; AUX is a reserved device name on Windows, and the repository was uncheckoutable there from 2026-05-17 until it was found on 2026-09-28. The last green run was 2026-05-11, six days before the file landed. The filter also hid the fix: the rename touched only `internal/core/`, so the workflow did not run on that commit either and had to be dispatched by hand.

The general rule it broke: **a filter may only be narrower than what the job actually depends on.** This job depends on every file in the repository, because it checks all of them out.
---

## Releasing

Releases are cut automatically from `main`, but only off a green build.
Versioning follows [Conventional Commits](https://www.conventionalcommits.org).
The `release` job in `.github/workflows/ci.yml` `needs:` the test and
integration jobs, so a failing build never produces a tag. When a
release-worthy commit lands and CI passes, the job derives the next semantic
version, pushes the tag, and runs goreleaser to publish the GitHub release and
update the Homebrew/Scoop taps.

The bump is the highest implied by any commit since the last tag:

- `fix:` → patch
- `feat:` → minor
- a `!` marker (`feat!:`) or a `BREAKING CHANGE:` footer → major
- anything else (`docs:`, `refactor:`, `chore:`, `ci:`, `test:`, non-conventional) → no release

A push to `main` with no release-worthy commits produces no tag and no release.
Because the bump is computed from *every* commit since the last tag, a release
deferred by a red build (or a string of non-release commits) still accounts for
everything accumulated in between once a green, release-worthy commit lands.
The classification lives in `internal/release` (unit-tested) and is driven in
CI by the non-shipped `cmd/next-version` helper, which prints the next tag (or
nothing) from the git history.

The whole flow runs on the default `GITHUB_TOKEN` plus the existing
`TAP_GITHUB_TOKEN` secret — no personal access token is required. Tagging and
publishing happen in the **same** job rather than via a separate tag-triggered
workflow, because a tag pushed with `GITHUB_TOKEN` does not trigger another
workflow run. This is the only release path: pushing to `main` is how a release
is cut, with no manual tagging step.

---

## Implementation Notes (Go)

### Acceptance tests

`internal/acceptance` wires the real composition — `.ezcd.json` on disk, `config.Load`, the Lens, the renderers — and asserts on rendered output rather than on derived values. It exists because unit tests per layer are not sufficient to show a feature works.

`clarity.leadTime` proved that. Core computed all three modes correctly and had tests to prove it; both renderers then rebuilt their own groupings from the raw snapshot and threw the configured `View` away. Every unit test passed and the setting did nothing at all. The bug lived precisely in the seam that no single-layer test looked at.

The rule that falls out: **a renderer consumes the `View` it is handed and never re-derives from `Snapshot`.** The derivation layer is the single source of grouping, lead time and DORA truth — that's what `View`'s doc comment always claimed, and what the renderers quietly violated. Anything configurable needs a test that crosses from the config file to the thing a user reads, because that is the only place the claim can be checked.

### go-git vs shelling out

Use `go-git` for: walking the commit log, and building the objects a write commits.

Shell out to `git` for anything that talks to a remote — `fetch` and `push` — because git handles credentials, transports and refspecs. And for reading the events ref, because git is the only thing that knows how this particular working copy was cloned; see "Clone shapes and the object store" below. This matches pushq's pattern.

### Clone shapes and the object store

Clarity is handed a working copy made by whatever checkout action the user's CI happens to run. It must not care how that clone was shaped — but it did, because it resolved objects through go-git, which reads `.git/objects` directly.

Two layouts that CI produces routinely break that assumption, and both present identically: a bare `object not found` against a ref that is present and correct, locally *and* on the remote.

- **Partial clone** (`--filter=blob:none`). Sets `remote.origin.promisor`, and a later fetch *inherits the filter* — so fetching `refs/clarity/events` brings the commit and its trees but leaves the event JSON blobs on the server, to be retrieved lazily on access. git does that transparently; go-git has no promisor support.
- **Shared object store** (`.git/objects/info/alternates`). How CI caches avoid re-downloading a repo on every run: the checkout keeps a mirror between jobs and points the workspace at it, so the workspace holds almost no objects of its own. go-git *has* alternates support, but its filesystem abstraction refuses to cross the repository boundary, so an absolute path to a mirror elsewhere on disk never resolves.

Two rules fall out, and between them they make clarity independent of the clone:

1. **Reads of the events ref go through git, not go-git** — `git ls-tree` plus a single `git cat-file --batch`. git is the only thing that knows how this working copy was cloned, so it handles both layouts, and any future variation, by construction. It is also where a lazily-held blob gets fetched, which a direct object-store read cannot do at all.
2. **Clarity's own fetch asks for a complete object graph** (`git fetch --no-filter`), whatever filter the working copy was cloned with. `--no-filter` arrived with partial clone in git 2.19; older git rejects the flag and the fetch retries without it, which is safe because such a git cannot have made a partial clone to begin with.

Rule 1 is what makes it *work*; rule 2 is what makes it work *well*. Once reads go through git, a lazily-held blob would be fetched on access anyway — but one object at a time, mid-read, against a ref that can hold thousands of files. `--no-filter` collapses that into the single round trip that was already happening, and it is the difference between working and failing outright where lazy fetching is refused (`GIT_NO_LAZY_FETCH=1`, which CI setups set to keep builds off the network).

The commit-log walk goes through git for the same reason, which closes a third layout: a **shallow clone**. `--depth=1` is the actions/checkout default, so it is the shape most CI repositories have — and it records a graft boundary in `.git/shallow` whose oldest commit claims parents that were never downloaded. git honours the graft and stops; go-git does not read `.git/shallow`, followed the parent pointer into a missing object, and failed the entire walk. That surfaced as `git clarity --plain` dying with "context deadline exceeded".

What is left for go-git is writing: building the blob, tree and commit objects a report commits. That needs none of this — creating loose objects in the local store always works whatever the clone's shape, and the `git push` that follows reads them back with git's own rules. **go-git is never used to read; git is.** That one line is the whole invariant, and it is what makes the clone's shape stop mattering.

Trading a fix for one checkout against a break on another is the specific failure this is all guarding against: `actions/checkout` is what nearly every consumer uses, and every one of these paths keeps working exactly as before under it.

### Distinguishing "no ref yet" from "cannot reach the ref"

These look identical at the point of failure and mean opposite things, so they are reported differently:

- **The remote has no events ref yet.** The ordinary state of every repo before its first report. Not an error — the fetch returns `nil` and the write builds from an empty tree.
- **The remote could not be reached.** A `*clarityrefs.FetchError`, naming the ref, the remote and its URL, with git's own output below it. This is where a credential or configuration problem surfaces, and it must never be phrased as something missing.

The same distinction has to be made locally, and it is easier to get wrong there. `git rev-parse --verify --quiet` exits 1 and says nothing for **both** "no such ref" and "the ref exists but names an object this repository cannot produce" — `--quiet` is precisely what suppresses the difference. So existence is probed with `git show-ref --verify`, which exits 1 silently when the ref is absent and fails loudly when it is unreadable, and only then is it resolved.

Collapsing those two is worse than the bug this all started with: an unreadable history renders as a repo that has never reported, which is an empty dashboard with no diagnostic at all. It is reachable by exactly the route that motivated the change — a shared object cache evicted between jobs leaves the ref in the workspace and takes the objects away.

**Credentials never appear in an error.** `git remote get-url` returns the remote verbatim, and embedding a token in it is routine in CI — GitHub's `x-access-token`, GitLab's `CI_JOB_TOKEN`, hand-rolled runners. It also applies `url.<base>.insteadOf` rewriting, so a repository whose own remote looks clean can still hand back a URL carrying a token injected by global config. A failed report must not write a live credential into a build log.

Two things get scrubbed, and missing either one leaks:

- **Clarity's own text**, where the remote URL is named. Textually rather than via `net/url`: `url.Parse` rejects plenty of URLs git accepts — an invalid percent-escape, a non-numeric port, a stray space — and a parse-based redactor has to decide what to do when it fails. Returning the original is a leak, and falling back to scp-style handling silently does nothing, because the `//` in a scheme guarantees a `/` before the `@`.
- **git's own output**, which clarity embeds beneath its message. git is only half a safety net: it redacts the *password* but prints the username in the clear, and `https://<token>@host/...` carries the token as the username. Since clarity runs git with `GIT_TERMINAL_PROMPT=0`, the message a misconfigured runner actually produces is `could not read Password for 'http://<token>@host'`.

The username is therefore treated as secret alongside the password, and the scrub lives in `internal/gitenv` because every package that shells out to git embeds git's output in its errors, and each one is a route to the same log.

**Git is asked for untranslated messages** — `LC_ALL=C` and an empty `LANGUAGE`, set once in `internal/gitenv`. Clarity classifies several outcomes by matching git's own text: "couldn't find remote ref" to recognise a repo that has never reported, "unknown option" to fall back on old git, plus the push-rejection and damaged-object matchers. Every one of those strings is gettext-translated, so under a non-English locale they stop matching and the behaviour they guard silently inverts — most sharply on the first report in a repo, where failing to recognise "no events ref yet" turns an ordinary first run into a hard failure.

A fetch failure is fatal to a write rather than ignored. Continuing would rebuild the ref from an empty tree, so a push that somehow landed would discard every event already recorded — and a push rejected as non-fast-forward would send the retry loop back to a fetch that fails identically, forever.

The read side (the watcher) still ignores a failed events fetch: a TUI that cannot reach the remote should keep rendering the last known state rather than blank the view.

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

A lost race has two shapes, and both must be recognised as retryable. When the
client can already tell it is behind from the ref advertisement it declines the
push itself: `! [rejected] ... (fetch first)`. When two jobs push at the same
instant neither client knows it is behind, so the loser's compare-and-swap is
refused by the *server*, which reports it as a lock failure:

```
 ! [remote rejected] refs/clarity/events -> refs/clarity/events
   (cannot lock ref 'refs/clarity/events': is at a9f4824... but expected 1668810...)
```

Note the `remote` inside the brackets — matching `[rejected]` never sees that
line. The second shape is matched on `cannot lock ref` rather than on
`[remote rejected]`, because a remote fsck rejection is worded the same way and
needs a different recovery (dropping the local ref, not just replaying the
commit).

### Surviving a concurrent gc

`git gc` and clarity's writes contend for the same object store, and the loser is always clarity. go-git stores a loose object by streaming it to `.git/objects/pack/tmp_obj_*` and renaming it into `.git/objects/<xx>/<rest>`; go-billy creates the destination's parent directory immediately before the rename. `git gc` rmdirs empty `.git/objects/<xx>` directories — at its default two-week prune expiry, not just under `--prune=now` — so a gc landing between those two syscalls makes the rename fail with `no such file or directory`. The read side has its own version of this: a gc that repacks mid-read deletes a packfile go-git has already resolved from its `.idx`, surfacing as `packfile not found`.

CI is where this bites, because a fresh checkout has every object packed and no `.git/objects/<xx>` directories at all, so nearly every event write has to create one.

There is a second, narrower way a gc breaks a write. The objects a write creates are unreachable until the events ref moves to point at them; a gc pruning aggressively enough to collect objects that young deletes them in that window, and the push then fails locally while packing what it can no longer read. This one needs `--prune=now` or a configured `gc.pruneExpire` — auto-gc's two-week default never collects an object that young — so it only affects repos whose workflows run their own gc.

Three layers of defence, because none is sufficient alone:

- **Prevent the gc we cause.** Every git command clarity runs is prefixed with `-c gc.auto=0 -c maintenance.auto=false`. Without this, the `git fetch` at the top of the push loop spawns a detached `gc --auto` that then races the tree build a few statements later — clarity's own fetch is the most likely trigger of clarity's own failure.
- **Retry the write.** Another job, another tool in the same workflow, or a repo with `gc.auto` configured can still start one. Each loose-object write retries a bounded number of times on a rename that failed because a path component vanished. Git objects are content-addressed, so a retried write is idempotent, and the retry re-creates the pruned directory. Retries are matched on the typed `*os.LinkError` rather than on message text, so an unrelated missing file still fails fast.
- **Rebuild after a prune.** Retrying the write cannot help once the objects are already gone, so a push that fails because the local store can no longer read what it is packing drops the local ref and rebuilds from scratch, a bounded number of times. The tree is derived from the events the caller supplied, so every pruned object is simply written again. Git's wording here varies by object type — `unable to read <id>` for a blob, `bad tree object <id>` for a tree, `bad object <id>` for a commit — and mentions gc in none of them, so the match keys on the shape they share: a diagnostic naming a raw object id.

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
- **Full DORA metrics support** — extending the event model to capture incidents and hotfixes so that MTTR and change failure rate can be derived alongside the lead time and deploy frequency metrics that already fall out of the v1 design. [Weekly Metrics](#weekly-metrics-git-clarity-metrics) is the surface those would be shown on
- **pushq integration** — when `refs/push-queue/*` exists in the repo, render the queue as a "pending" section above the most recent commit on main. Implemented by importing `github.com/ezcdlabs/pushq/pushqrefs` and adding a section to the TUI render
- **CI-specific integrations** — clickable run links, actor avatars, job-level breakdowns, derived from the `ci` metadata block on events
- **Relay mode** — a long-running process with git access that forwards normalised events to a hosted dashboard, enabling a SaaS tier without giving the SaaS git access
- **Multi-repo dashboard** — show several repos at once, primarily a feature for the relay/SaaS tier
- **Notification hooks** — Slack/Discord/webhook integrations for deploy events
- **Historical retention / `git clarity gc`** — events can grow unbounded; a pruning command for old events
- **Branch awareness** — currently focused on the main branch; optional support for PR branches with their own pipeline status
- **Auto-detect current branch** — the TUI currently hardcodes `main`; defaulting to the current checkout (or accepting `--branch`) is straightforward once the need arises
- **Per-target GitHub Actions source** — the `clarity.github` config maps one workflow/job set per stage, so the GitHub source is single-flow. Deploy targets currently require the events ref. Extending the config to a per-target deploy mapping (and `init --github` to ask for it) is the notable follow-on cost of the target model
- **Candidacy in batch mode** — `report --batch` carries stage events only, so a monorepo backfilling its history gets flows but not candidacy, and every historical commit counts towards every flow's lead time. A `scope/` equivalent for the JSONL stream would close it
- **Flow aliases** — renaming a target is handled by adding both names to one flow's `targets`, which covers everything seen so far. A separate alias mechanism would only be needed if a flow's identity had to change without a config edit; not built until asked for
- **Target name validation without config** — stages and statuses are enforced against closed sets in `internal/report`. Target names can't be: with no `.ezcd.json` declaring flows, any string is accepted, so a typo creates a ghost flow. Declaring flows closes the set (see [Deploy targets](#deploy-targets)); a repo that hasn't declared any keeps the open vocabulary by design
- **Watcher fetch error surfacing** — fetch failures in the polling loop are currently silent; the TUI shows the last successful snapshot with no indication that it has gone stale. A subtle "(stale)" marker on the header would close that loop
- **JSON output mode** — `git clarity --json` for scripting and piping into other tools
- **Configuration file** — a `.clarity.json` or `.git/config` section for per-repo settings (poll interval, branch, etc) once there are options worth configuring
- **Demo gif machinery** — port pushq's `cmd/demo/` + `scripts/record-demo.sh` pattern: a tiny replay binary that drives the TUI's render functions with scripted snapshots, then asciinema → agg → gif. Because the demo invokes the real renderer the recording can never drift from actual behaviour, which makes it cheap to keep up to date for the README and release announcements
- **GitHub Action and `pull_request_target`** — the `ezcdlabs/clarity` action writes events via `git push`, which requires `contents: write` on the `GITHUB_TOKEN`. GitHub forces fork-PR tokens to read-only on the standard `pull_request` trigger, so forks can't write events even when the workflow declares `contents: write`. If a workflow ever switches to `pull_request_target` (which gives forks the host repo's full token), forks would be able to write arbitrary events. Worth a documented warning in the action README and possibly an in-action guardrail (refuse to run when `GITHUB_EVENT_NAME == pull_request_target`) once it becomes a real risk
