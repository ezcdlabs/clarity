# Design documents

These files record the design decisions behind clarity. They are a **living spec**, not an archive: when an open question is resolved, an implementation reveals a detail the design didn't anticipate, or an approach changes, the relevant file here is updated before the work is considered done. Decisions that were reversed or retired stay recorded, next to the design that replaced them, because the reasoning is worth keeping.

- [Problem and concept](problem-and-concept.md) — the problem statement, who clarity is for, the core concept, and the original v1 scope
- [Data model](data-model.md) — the events ref, event filenames and content, why per-file events, status collapse, and the fetch refspec
- [The config file](config.md) — `.clarity` on the trunk: why a committed file and not a ref, and the legacy fallbacks
- [Event sources](event-sources.md) — the pluggable `exec` source and its trust model, and the retired GitHub Actions live source
- [Deploy targets](deploy-targets.md) — flows, declaration as expectation, and renames without history rewrites
- [Candidacy](candidacy.md) — affected/unaffected: keeping monorepo lead times honest
- [The TUI](tui.md) — trunk detection, startup, the header and deploy strip, badges, and rendering rules
- [CI reporting](reporting.md) — what `report` does, failure recovery, the action's version resolution, batch backfill and migration
- [Pre-push checking](check.md) — why `check` is a convention and not a hook, what counts as red, failing open
- [Weekly metrics](metrics.md) — box plots and medians, the log-space axis, whole weeks only
- [Mobile](mobile.md) — the native apps' product and UX decisions
- [Mobile engineering notes](mobile-engineering.md) — the proto boundary, the mark, the bridge, notifications, auth, and iOS without a Mac
- [Android app flow](android/README.md) — the design handoff for the Android app's screens

Historical:

- [Architecture refactor](architecture-refactor.md) — the reasoning that produced the hexagonal core/adapter split. Kept for the trade-offs it records; not maintained as a description of the current system.
