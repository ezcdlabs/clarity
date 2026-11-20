# Data Model

## The events ref (`refs/clarity/events`)

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

`scope/` is a sibling tree holding [candidacy](candidacy.md#candidacy-affected--unaffected)
records. It is separate from `events/` because candidacy is not a pipeline event,
and because every existing binary walks only `events/` — so a repo adopting
candidacy stays readable by older clarity versions by construction rather than by
luck.

## Event filename

```
<unix-timestamp>-<short-uuid>.json
```

Timestamp first for natural sort order. Short UUID suffix for uniqueness when multiple events arrive in the same second. No CI-specific identifiers in the filename — keeps the design portable across all CI systems.

## Event content

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
[Deploy targets](deploy-targets.md#deploy-targets). It lives in the JSON rather than in the file
path so that adding it doesn't change `ReadEvents`' shape for existing readers;
filenames are content-addressed, so two events differing only by target already
hash to different files.

The optional **`ci` block** is opaque metadata captured from the environment when available. Clarity opportunistically reads common env vars (`GITHUB_*`, `GITLAB_*`, `CI_*`) and populates the block. Renderers may surface this metadata where useful but never depend on its presence.

Statuses per stage: `started`, `passed`, `failed` — plus `noop`, valid only on
`deploy`.

**`noop`: delivery completed, nothing shipped.** A deploy event answers two
questions at once — *is this commit's tree in production?* and *did a
deployment occur?* — and `noop` is the case where they diverge: the pipeline
ran its deploy stage, compared the desired state against what is deployed, and
found them already equal. The canonical producer is a revert pair: two commits
that each modify a target's inputs (so both are genuinely *affected* — see
below for why candidacy cannot describe this) but whose cumulative diff is
empty. Without `noop` those commits either sit in CI Passed forever, dragging
the lead time up until an unrelated commit ships them, or get reported as a
deploy that never happened, inflating the deploy count. The derivations split
along the two questions:

- **State — yes.** A `noop` moves the deploy line, anchors a `Deployed` batch
  (subheader `deploy no-op`), and stamps `deployedAt`, freezing lead times at
  the moment convergence was verified. Those lead times enter the weekly
  distribution: the pipeline took as long to converge as it would have to
  deploy, and the measurement is real.
- **Activity — no.** A `noop` batch contributes zero to the weekly deploy
  count. The "live on production" marker, though, is a *state* claim rather
  than an activity claim, so a `noop` batch on top holds it: the pipeline just
  verified production equals that batch's tree, which is the freshest possible
  statement of the present state. (Leaving the marker on the passed batch
  below was considered and rejected — it put the noop's commits above the live
  line, reading as "not yet live" when they are live, trivially.) Which batch
  is live is core's decision (`Groupings.LiveBatchIndex`), shared by all three
  renderers.
- **Badges — green.** A badge answers "is production current?", so the badge
  projection (`CurrentStageStatus`) folds `noop` into `passed`; which kind of
  completion it was is per-batch information.

`noop` is meant to come from the pipeline's own comparison — a content hash
of the built artifact against the deployed one — in the step that would
otherwise deploy, consistent with "clarity records what the build system
says". It is rejected on `ci`: convergence is a statement about production,
and CI ships nothing to converge.

**`skipped` is retired.** It arrived with the conventional CI status set (it is
what GitHub Actions calls a job whose condition said not to run), but an event
that ships nothing says nothing about a commit's lifecycle, and no derivation
ever gave it positive meaning — its only effect in practice was to anchor an
unlabeled batch whose commit read as waiting to ship forever. Every real use
turned out to mean one of two things the tool now spells properly: "this
commit does not affect this target", which is
[candidacy](candidacy.md#candidacy-affected--unaffected), a property of the commit rather
than the outcome of an attempt (`git clarity report unaffected <target>`); or
"the deploy ran and had nothing to ship", which is `noop` above — a verdict,
where `skipped` never was one. `report` rejects `skipped` with an error
pointing at both. Reads still tolerate it — old refs carry it — and grouping
treats a skipped deploy event as no deploy event at all.

Stages are fixed at two: `ci` and `deploy`. This is deliberate — trunk-based development cares about exactly three things, latest HEAD, latest green CI, and latest production deploy, and that maps onto two state transitions. A "test" or "lint" or "integration" stage is not a separate lifecycle position; it's part of CI that either passes or doesn't. Adding more stages would dilute the "is HEAD green?" question this tool exists to answer. The `git clarity report` command rejects any other stage name with an error rather than letting custom names drift in.

## Why per-file events on a custom ref

A single shared blob (e.g. git notes) requires read-modify-write semantics for every append, with concurrent CI jobs racing on the push. Per-file events solve this:

- **Race-free appends** — different filenames, different events, no content conflicts. Only the fast-forward push race remains, which is trivially retried.
- **No data loss** — event filenames are content-addressed (`<unix-ts>-<sha256(json)[:8]>.json`), so two truly identical reports collapse into one file and two reports that differ (different timestamps, retries with different `GITHUB_RUN_ATTEMPT`, etc.) keep distinct files.
- **Audit trail by design** — every distinct report is preserved as its own file. Retries and parallel jobs carry different CI metadata so they hash to different filenames; duplicate writes of the literal same event are idempotent at the file and tree level (re-running a backfill produces no new commit).
- **Consistent with pushq** — same architectural pattern as `refs/push-queue/state`, making the EzcdLabs codebase coherent.

The trade-off is that events aren't directly inspectable with `git notes show`. This is addressed in Future Work via an optional summarisation layer that writes derived digests to `refs/notes/clarity` for native git tooling.

## Status collapse at render time

For each commit, the renderer walks all event files under `events/<sha>/`, sorts by timestamp, and computes the latest status per stage. This is what makes clarity feel different from a "run history" view — retries don't clutter the timeline; the user sees the current state of each stage per commit.

## Fetch refspec

The events ref isn't fetched by default. The first time `git clarity` runs in a repo, it ensures the fetch refspec is configured:

```
[remote "origin"]
    fetch = +refs/clarity/events:refs/clarity/events
```

This is a one-time, automatic step. The config needs no special transport at
all: `.clarity` rides the trunk, so any clone that can see the trunk — a
fresh CI checkout included, which is exactly where `report` enforces
declared targets — already has the config of record. The one clone that
cannot is a CI job checked out from a **tag**, which fetches no branch; a
reader that finds no trunk ref at all fetches just the tip (depth 1,
best-effort) before concluding the repo has no config, because that job may
be exactly the one reporting deploys and enforcement must not silently
switch off there. A clone that *has* a trunk ref is never fetched this way —
its ref is the answer, and a depth-1 fetch would shallow-mark a developer's
full clone. Only the legacy `refs/clarity/config` fallback is still fetched
explicitly (best-effort, so offline answers from the last fetch), for as
long as unmigrated repos need it.
