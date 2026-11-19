# clarity (`git clarity`)

[![demo](assets/happy-path.gif)](https://cdn.jsdelivr.net/gh/ezcdlabs/clarity@main/assets/happy-path.mp4)

**Live pipeline status for trunk-based development, in your terminal.**

- **The commit is the object.** CI UIs show runs; clarity shows your trunk's commits with their current resolved status — retries and parallel jobs collapse into one answer per stage.
- **No server.** Pipeline events live on a dedicated git ref (`refs/clarity/events`) in your own repository, written from CI with one command. Auth is whatever git auth you already have.
- **Works with any CI.** `git clarity report ci passed` is the whole integration; GitHub Actions gets a one-step setup action on top.
- **A pre-push convention.** `git clarity check && git push` stops you landing on a red trunk.
- **DORA numbers for free.** Per-commit lead time inline, and `git clarity metrics` draws weekly lead-time and deploy-frequency trends.

---

## Install

### With an agent (recommended)

In the repository you want to instrument, paste this into Claude Code or any coding agent:

```
onboard this repo to clarity (github.com/ezcdlabs/clarity)
```

The agent installs the binary, runs `git clarity onboard`, and follows the briefing that command prints — wiring your pipeline up and backfilling real history. See [Onboarding](docs/onboarding.md).

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

```sh
git clarity                      # watch the trunk, live (first run opens a welcome pane)
git clarity check && git push    # don't land on a red trunk
git clarity metrics              # weekly lead time and deploy frequency
```

Inside your pipelines:

```yaml
- run: git clarity report ci started
- run: ./build.sh
- run: git clarity report ci passed
- run: ./deploy.sh && git clarity report deploy passed || git clarity report deploy failed
```

In GitHub Actions the existing `GITHUB_TOKEN` is sufficient, and the [`ezcdlabs/clarity` action](docs/reporting.md#reporting-from-github-actions-ezcdlabsclarity) installs and reports in one step.

## Documentation

The [docs index](docs/README.md) has everything. Quick links:

- [Onboarding with an agent](docs/onboarding.md) — the recommended way to adopt clarity
- [Watching: `git clarity`](docs/watching.md) — the TUI, plain mode, and flags
- [Setup and configuration](docs/setup.md) — `init`, `config`, and `trust`
- [Reporting from CI](docs/reporting.md) — `git clarity report` and the GitHub Action
- [Checking before you push](docs/check.md) — `git clarity check`
- [Lead time](docs/lead-time.md) — what the numbers measure, and the three modes
- [Weekly metrics](docs/design/metrics.md) — `git clarity metrics`
- [Troubleshooting](docs/troubleshooting.md)
- [Design documents](docs/design/README.md) — the decisions behind clarity, kept as a living spec

## What this is not

- Not a CI runner — it consumes events emitted by your existing CI
- Not a build cache or artifact store
- Not a code review tool
- Not a replacement for OpenTelemetry — it's a different model (commit-centric vs trace-centric)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Coding agents: read [AGENTS.md](AGENTS.md) first.

[MIT License](LICENSE)
