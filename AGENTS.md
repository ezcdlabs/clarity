# AGENTS.md

clarity (`git clarity`) is a commit-centric pipeline-status TUI for trunk-based development, written in Go. Pipeline events live on a dedicated git ref (`refs/clarity/events`) in the user's own repository; there is no server.

> **Here to onboard a different repo to clarity?** Don't read this file — follow [docs/onboarding.md](docs/onboarding.md): install the binary, run `git clarity onboard`, and follow the briefing it prints.

## Build and test

```sh
go build ./...
go vet ./...
go test ./...
```

The SSH-backed integration suite is behind the `ssh` build tag and needs a container runtime; CI runs it. The mobile apps build in CI (`gomobile bind`, Gradle, Xcode on macOS runners) — you generally don't build them locally.

## Test-driven development

Write tests before implementation. The red/green/refactor cycle is the default approach for all new behaviour.

- New functions that classify or categorise (like `isFastForwardRejected`) must have a table-driven unit test covering all known cases before the function is written. When a new case is discovered at runtime, add the failing test first, then fix the code.
- New public API behaviour must have an acceptance test (or unit test) that fails before the implementation is added.
- Do not write implementation speculatively and backfill tests — always go red first.

## The design docs are a living spec

[docs/design/](docs/design/README.md) documents decisions that have been made, not just intent. When a design decision is made in a conversation — especially one that diverges from, refines, or extends what is written there — update the relevant design doc before moving on.

Update a design doc when: an open question gets resolved; an implementation reveals a detail the design didn't anticipate; an approach changes; a new constraint or behaviour is established. Do **not** update it for pure implementation details (code comments), ephemeral task tracking, or anything already accurately described.

Before changing behaviour, check the matching design doc — most surprising-looking code in this repo is a recorded decision with reasoning behind it.

## Commits cut releases

Pushing to `main` is how a release is cut — there is no manual step. Versioning follows Conventional Commits: `fix:` → patch, `feat:` → minor, `!`/`BREAKING CHANGE` → major; `docs:`, `chore:`, `test:`, `refactor:`, `ci:` cut no release. See [docs/internals/releasing.md](docs/internals/releasing.md).

## Where to read

- [docs/README.md](docs/README.md) — index of all documentation
- [docs/design/](docs/design/README.md) — why things are the way they are
- [docs/internals/](docs/internals/repository-structure.md) — repo structure, CI coverage, releasing, implementation notes
