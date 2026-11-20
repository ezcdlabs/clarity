# clarity documentation

clarity (`git clarity`) is a commit-centric pipeline status TUI for trunk-based development. If you're new, start with the [README](../README.md) for install, then [Onboarding](onboarding.md) to wire up a repository.

## Using clarity

- [Onboarding with an agent](onboarding.md) — the recommended way to adopt clarity: one prompt, and `git clarity onboard`
- [Watching: `git clarity`](watching.md) — the live TUI, the plain/agent-friendly mode, and the `--show-shas`, `--limit` and `--deploy` flags
- [Setup and configuration](setup.md) — `git clarity init`, reading and changing config with `git clarity config`, and approving exec sources with `git clarity trust`
- [Lead time](lead-time.md) — what the per-commit number measures, and choosing between the `pipeline`, `reported` and `all` modes
- [Checking before you push](check.md) — `git clarity check && git push`
- [Reporting from CI](reporting.md) — `git clarity report <stage> <status>` from any CI, and the `ezcdlabs/clarity` GitHub Action
- [Weekly metrics](design/metrics.md) — `git clarity metrics`: weekly lead-time box plots and deploy counts
- [The phone apps](phone-apps.md) — installing on Android (self-updating APK) and iOS (AltStore)
- [Troubleshooting](troubleshooting.md) — known failure modes and their fixes

## Design

The [design documents](design/README.md) record the decisions behind clarity and are maintained as a living spec:

- [Problem and concept](design/problem-and-concept.md) — why commits, not runs; who this is for
- [Data model](design/data-model.md) — the events ref, event files, status collapse, and the fetch refspec
- [The config file](design/config.md) — `.clarity` on the trunk, and why it isn't a ref
- [Event sources](design/event-sources.md) — the `exec` source and trust model; the retired GitHub Actions live source
- [Deploy targets](design/deploy-targets.md) — flows for monorepos that ship several things
- [Candidacy](design/candidacy.md) — affected/unaffected, and honest lead times in a monorepo
- [The TUI](design/tui.md) — trunk detection, the header and deploy strip, rendering rules
- [CI reporting](design/reporting.md) — what `report` does, recovery, backfill and migration
- [Pre-push checking](design/check.md) — why `check` is a convention, not a hook
- [Weekly metrics](design/metrics.md) — box plots, axes, and what crosses to the phone
- [Mobile distribution](design/mobile-distribution.md) — releases as the store: the signed APK, the self-updater, AltStore
- [Mobile](design/mobile.md) and [mobile engineering notes](design/mobile-engineering.md) — native apps over the same core
- [Architecture refactor](design/architecture-refactor.md) — historical: the reasoning behind the hexagonal split

## Internals

For contributors — see also [CONTRIBUTING.md](../CONTRIBUTING.md) and [AGENTS.md](../AGENTS.md):

- [Repository structure](internals/repository-structure.md) — the package layout and the public `clarityrefs` API
- [What CI covers, and where](internals/ci.md)
- [Releasing](internals/releasing.md) — conventional commits cut releases automatically
- [Implementation notes](internals/implementation-notes.md) — go-git vs git, clone shapes, the push loop, surviving gc

## Roadmap

- [Future work](roadmap.md) — a parking lot awaiting triage into issues
