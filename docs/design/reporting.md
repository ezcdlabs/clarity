# CI Reporting: `git clarity report`

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

## What it does

1. Validates that `<stage>` is `ci` or `deploy`, and `<status>` is `started`/`passed`/`failed` — rejects anything else (`skipped` is retired, and its error points at `report unaffected`). A `<target>` is accepted only for `deploy`, and only if declared in the clarity config when it declares any
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

## Recovering a failed report

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

## Authentication in CI

Uses whatever git credentials are already available to the CI runner. In GitHub Actions the `GITHUB_TOKEN` suffices once the job grants it `contents: write` — no additional secrets needed.

## Which version the action installs

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

## Concurrency

Two pipeline jobs reporting simultaneously cannot corrupt each other's data because they write different files. The only contention is the fast-forward push race on `refs/clarity/events`, handled by the same retry loop pattern pushq uses for its state branch.

## The target argument

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

When the clarity config declares flows, a target none of them claims is refused at
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
lead time — see [Candidacy](candidacy.md#candidacy-affected--unaffected).

## Reporting candidacy

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

## Explicit overrides for migration

Two optional flags let a caller override the SHA and timestamp clarity would otherwise infer:

- `--sha <sha>` — write the event under this commit SHA instead of resolving from `GITHUB_SHA` / `CI_COMMIT_SHA` / `HEAD`. Highest precedence — beats env vars.
- `--at <rfc3339>` — record this timestamp in the event instead of `time.Now()`. Format is RFC3339 (e.g. `2024-04-08T15:48:54Z`).

The primitive use case is **backfilling history** when adopting clarity on an existing repo. A migration script (e.g. one that queries the GitHub Actions API for past workflow runs) shells out to `git clarity report` once per historical run, passing `--sha` and `--at` to recreate events for commits clarity wasn't installed on at the time.

Backfilled events use the same JSON schema and the same ref as live ones — the TUI cannot distinguish them.

## Batch mode for backfill

For large backfills (hundreds of historical events) the per-event push round-trip dominates wall-clock time. `--batch` flips `report` into a stdin-driven mode that does **one** fetch + **one** push for the whole stream:

```
git clarity report --batch < events.jsonl
```

Each line is a JSON object with all four fields:

```json
{"sha":"abc1234...","at":"2024-04-08T15:48:54Z","stage":"ci","status":"passed"}
```

Blank lines are skipped; the first malformed line aborts with a `line N: ...` error. A batch lands on `refs/clarity/events` as a single commit with message `report: batch of N events`. Backfill events in batch mode do not get CI metadata auto-attached — the local env doesn't describe the historical pipeline that produced them. Live reporting should continue to use the single-event form so each event lands as its own audit-able commit.

## Migrating from GitHub Actions

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
