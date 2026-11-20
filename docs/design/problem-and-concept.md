# Problem and concept

## Problem Statement

In trunk-based development, the question that matters is: **is HEAD on main green?** If it isn't, everything else stops until it is. The unit of work is the commit, the unit of integration is the commit, and the unit of release is the commit. Runs, jobs, and workflows are implementation details of how a commit gets validated and shipped.

CI/CD UIs have this backwards. GitHub Actions, GitLab Pipelines, and others present **runs** as the primary object — a list of pipeline executions, ordered by start time, with commits as a label on each row. Answering "is HEAD green?" requires scanning that list, ignoring retries of older commits, filtering out unrelated workflows, and finally finding the run that corresponds to the commit you actually care about.

This is the wrong model. The commit is the right object. A pipeline run is just a thing that happened to a commit, and what users need to see is the commit with its current status — not a log of every event that ever touched it.

**Field note (November 2026): green is the common case.** On a working trunk team, main is green nearly all of the time, so "is main green?" is not a question anyone keeps asking — in practice the dashboard's job is to be unmissable on the rare red, and to show enough of what is happening that its quiet is believed. The detail on screen is what makes the absence of an alarm credible. A second observation from the same period: before clarity, most teams have no view of the trunk at all — only `git log` and the CI's job history, where one knows the commits and the other knows the runs. The README's introduction is written from both framings.

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

- **Lead time for changes** — time from commit to deploy, derived per commit from its authoring time (or its first pipeline event) and the `deploy passed` event that shipped it. Which commits count and where the clock starts is configurable — see [Lead time](../lead-time.md#lead-time), including why only the default mode is DORA's definition
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
brew install ezcdlabs/tap/git-clarity
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

*Historical — this records the original scope decision. Clarity has since grown `check`, `metrics`, deploy targets, candidacy and the mobile apps, each documented in its own design doc.*

The first release does exactly two things:

1. **`git clarity`** — opens a live updating TUI showing your commit history with pipeline status per commit.
2. **`git clarity report <stage> <status>`** — invoked from CI, writes a single event file to `refs/clarity/events` for HEAD and pushes the ref.

Everything else (web UI, metrics endpoint, pushq integration, multi-repo, relay mode, SaaS, summarisation into git notes, CI-specific integrations) is explicitly out of scope for v1 and listed under Future Work.
