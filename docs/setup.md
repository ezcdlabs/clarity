# Setup and configuration

## First run and setup: `git clarity init`

A repo nobody has set up has no `.clarity` on its trunk (see [The config file](design/config.md#the-config-file-clarity)), and on such a repo the TUI opens on a **welcome pane** rather than a dashboard where every commit sits meaninglessly "before CI Passed". The pane names the detected trunk, explains what is missing, and offers two on-screen options — **Set up this repo** and **Exit** — navigated with the arrow keys and confirmed with enter. (On-screen selectables, never bare hotkeys: a visible choice confirmed with enter beats an invisible, instant letter binding.) Choosing setup writes a default `.clarity` into the working tree, then holds a done pane on screen — `git add .clarity && git commit && git push` — until acknowledged, because committing the file is the one step clarity cannot do for you and a transient message would scroll it away. Acknowledge it and the dashboard carries on; the pane also points at `git clarity onboard` for handing the rest of the wiring to an agent.

`git clarity init` is the same act as a command: it writes `.clarity` with defaults for you to commit and push to the trunk, and touches nothing (saying so) when the file is already on the trunk or already in the working tree — a re-run must never clobber choices someone has made since. On a repo whose config still lives in the legacy `refs/clarity/config`, init seeds the file from the ref's exact content, so migration carries choices over instead of resetting them. There is nothing to ask, so it is a one-shot, not a wizard: the trunk is detected, the lead time mode has a default, and the events source defaults to the repo's own events ref. It exists alongside the TUI's welcome pane for the caller that cannot press enter in a TUI — a script, a CI bootstrap, or the agent following the onboarding prompt. Init itself pushes nothing: the config lives on the trunk so that review and branch protection cover it, and landing it is an ordinary commit by whoever runs this.

## Reading and changing config: `git clarity config`

```sh
git clarity config                      # print the config in effect (the trunk's .clarity)
git clarity config leadTime             # read one key ("pipeline (default)" when unset)
git clarity config leadTime all         # rewrite .clarity for you to commit
```

Reads answer from the trunk — the config in effect — never from the working tree, so `get` cannot report an edit that hasn't landed as though it had. A set rewrites the one key in the working-tree `.clarity` and leaves the commit to you: the config lives on the trunk precisely so that changing it goes through the same review as any other trunk change, and a tool that pushed there itself would bypass exactly that. The seed for the rewrite is, in order, the working-tree file (an uncommitted edit is a change mid-flight, not something to discard), the trunk's file, or the legacy ref — so a set on an unmigrated repo doubles as the migration.

A set re-parses the **whole** resulting file before writing it, so a typo'd value fails with the list of valid ones and no file is touched. Writes are stricter than reads on purpose: the read path ignores keys it doesn't know (an old binary must survive a newer config), but this command refuses to *write* a key this build cannot validate, because writing forward-compatibly is how typos become permanent shared state. The settable keys are `leadTime` and `source`; `[deploy "…"]` and `[source]` sections are structural and stay a hand edit until a subcommand earns its keep. The audit trail is the trunk's own history: `git log -- .clarity`.

## Approving an exec events source: `git clarity trust`

A repo whose `.clarity` configures `source = exec` (see [Pluggable event sources](design/event-sources.md#pluggable-event-sources-claritysource--exec)) names a command for clarity to run — and shared configuration must not be able to run code on your machine just because you opened a dashboard. `git clarity trust` shows the exact argument list and asks for a typed confirmation; the approval is recorded in this clone's local `.git/config` as a hash of that argv, so any change to the command asks again and a fresh clone asks its own human again. Until it is approved, clarity reads the events ref instead and marks every frame `source degraded`, in the TUI and plain output both. Approval is deliberately a human act: the command never prompts outside an interactive terminal, and agents should treat running it as a decision for their human.
