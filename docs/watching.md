# Watching: `git clarity`

[![demo: running git clarity](../assets/happy-path.gif)](https://cdn.jsdelivr.net/gh/ezcdlabs/clarity@main/assets/happy-path.mp4)

In any git repository, run:

```sh
git clarity
```

This opens an alt-screen TUI showing the most recent commits on the trunk (detected from the repository — see [The TUI](design/tui.md#the-tui-git-clarity)) with their pipeline status. Press `q` to quit. On a repo that has never been set up it opens on a welcome pane instead (see [First run and setup](setup.md#first-run-and-setup-git-clarity-init)); clarity also adds the events fetch refspec to your repo's git config the first time it runs.

When `stdout` is not a terminal (e.g. piped to another command, redirected to a file, or invoked by an agent), `git clarity` automatically switches to a static plain-text rendering of the same view — header line plus HEAD / CI Passed / Deployed sections, no animation, no ANSI escapes. Force it in an interactive terminal with `--plain`. Three more flags shape the output in either mode:

- `--show-shas` — include a short commit SHA per row. Off by default in both TUI and plain modes; agents acting on specific commits should turn this on.
- `--limit N` — cap how many commits are walked. Defaults to 100; pass `0` for unlimited. When the cap is what ended the list, the last line says so and names the limit, so the bottom of a truncated scroll can't be mistaken for the start of the repository.
- `--deploy=<flow>` — select one deploy flow in a repo with several. In plain mode it narrows the output to that flow's section; in the TUI it opens with that flow selected. Only meaningful alongside [deploy targets](design/deploy-targets.md#deploy-targets).

The plain output is intentionally grep-friendly: `grep ✗` finds failed commits, `grep deploying` finds in-flight deploys, `grep "(live)"` finds the currently-live batch.

The Deployed section also surfaces per-ISO-week DORA throughput inline: a `<year>-W<NN>  N deploys  Xh Ym avg` summary on each week boundary. **Only the current week shares the `Deployed` header row**; every other week, including last week, gets its own divider below.

That position reads as the current state, so whatever sits in it is taken for now. Merging the *newest* week there instead meant a repo whose last deploy was a week ago showed last week's totals as its headline — "36 deploys" where a glance reads this week's velocity. Two repos side by side made it plain: one had deployed this week and one hadn't, and nothing in the headline distinguished them.

The current week is reported whether or not it has deploys — `2026-W39  0 deploys` — because that is what makes two repos comparable at a glance, and it leaves no headline slot for an older week to occupy. A week with no deploys has no average, so the label omits it rather than printing a placeholder: `0s avg` reads as an extraordinarily fast week rather than an empty one.

An empty current week is followed by a blank line, so the section opens the way `HEAD` and `CI Passed` do when they are empty. Without it the next week's divider butts directly against the header and the eye binds its totals to it, which is the misreading this whole rule exists to prevent. "Deploys" counts distinct deploy batches (not per-commit events) and "avg" averages the per-commit lead times shown on the right of each row — see [Lead time](lead-time.md#lead-time) for what those measure and how to change it. ISO weeks are computed in UTC so the same data renders into the same buckets regardless of the user's locale.

The oldest week in a truncated window gets **no** divider. `--limit` almost always cuts mid-week, so that week is missing however many deploys fell below the cap — and unlike a missing row, an undercounted aggregate is wrong in a way the reader cannot see. Dropping it is the honest option: a divider that isn't there prompts a `--limit 0`, whereas "2 deploys" for a week that had nine just gets believed. Weeks above the boundary are kept; they can only be short by a commit authored below the cut but deployed inside the window, which is the long-lead case `--limit 0` also answers.
