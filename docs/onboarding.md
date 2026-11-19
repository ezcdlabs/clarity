# Onboarding with an agent

The fastest way to adopt clarity is to hand the job to a coding agent. In the repository you want to instrument, paste this into Claude Code (or any agent that can run shell commands):

```
onboard this repo to clarity (github.com/ezcdlabs/clarity)
```

Setting up the *tooling* is one command, but instrumenting a pipeline and backfilling its history is real work — and it is work an AI agent demonstrably does well when handed the repo and told how clarity's events work.

## The procedure

This is the canonical path, for agents and humans alike. One path, no alternatives.

1. **Install the binary.** Use one of the install methods in the [README](../README.md#install) (Homebrew, Scoop, or the release tarball onto `$PATH`). Verify with:

   ```sh
   git clarity --version
   ```

2. **Run the briefing and follow it.**

   ```sh
   git clarity onboard
   ```

   This prints a self-contained prompt carrying facts the binary gathered on the spot (the remote, the detected trunk, whether the repo is already set up, whether GitHub Actions workflows exist) plus the full event grammar, the batch backfill format, and a verification step. Follow it rather than improvising from these docs. A human can pipe it straight to an agent: `git clarity onboard | wl-copy` (or `pbcopy`) and paste.

3. **Backfill only real history.** The briefing instructs the agent to backfill only runs the CI provider actually recorded — with their real SHAs and timestamps — never invented history.

## Why the briefing lives in the binary

The prompt lives in the binary rather than in these docs on purpose: it is versioned with the binary, so it always describes the event format this exact build reads instead of drifting independently; it works offline; and it opens with repo-specific facts a hosted document would make the agent rediscover.

## See also

- [Setup and configuration](setup.md) — what `git clarity init` writes, and the welcome pane for interactive first runs
- [Reporting from CI](reporting.md) — the `report` command and the GitHub Action the briefing will wire in
- [CI reporting design](design/reporting.md) — backfill batch mode, `--sha`/`--at` overrides, and migrating history from GitHub Actions
