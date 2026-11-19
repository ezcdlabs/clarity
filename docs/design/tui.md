# The TUI: `git clarity`

A live updating terminal view of the most recent commits on the trunk, with pipeline stages and statuses rendered per commit.

**Which branch is the trunk** is resolved the same way for every command (`loadConfig` in `cmd/git-clarity`): the repository itself is asked — origin/HEAD (the remote's default branch, recorded at clone time, which gets `master`, `trunk` or `develop` right where any name probe cannot), then `main`, then `master`, each candidate only accepted if it actually resolves to a commit, so a stale origin/HEAD falls through (`gitlog.DetectTrunk`). A master-trunk repo therefore works with zero config, and there is no branch setting to drift out of date — only a legacy `.ezcd.json` pin still overrides detection, for as long as that fallback lives. The checked-out branch is deliberately **not** in the chain: silently watching whatever happens to be checked out would point the dashboard — and the `check` push gate — at a feature branch, a wrong answer that looks right. When nothing conventional resolves, the `main` default stands and the source pre-flight fails, hinting at the likely trunk and the git-native fix: `git remote set-head origin --auto`, which repairs origin/HEAD for every tool that reads it. Detection applies to `check` too, so the gate and the dashboard always agree on what "the trunk" means.

**The TUI starts before anything else runs.** Config reading, the first fetch and the branch pre-flight all happen behind a live frame, each step naming itself in place of the old bare "Loading" — `reading clarity config (.clarity on the trunk)`, `fetching master from origin`, `reading events` — so a stall shows its stage instead of lying that progress is being made. A repo with no config lands on the welcome pane (see [First run and setup](../setup.md#first-run-and-setup-git-clarity-init)); a startup failure renders in-frame with its hint attached, rather than on a stderr the alt-screen would eat. Nothing is allowed to sit between "typed the command" and "seeing something": that gap is where a hang is indistinguishable from a tool that simply doesn't work.

```
your-app · ci: ✓ · deploy: ✓

HEAD
  · grace   wip notes                                              30s

CI Passed
  ✓ dave    update dependencies                                  4m 12s

Deployed
  deploying…
  ✓ alice   refactor user model                                  6m 30s
  deployed 5m ago
  ✓ frank   tweak homepage                                      24m 10s
```

Above is the single-flow case, which is what a repo with one deploy target
renders. A repo with several turns that header row into chrome carrying a strip of
selectable flows — see [The header and the deploy strip](#the-header-and-the-deploy-strip).

**The top-right of the header is the refreshing indicator's slot, and it is
empty the rest of the time.** It used to say "press q to quit" permanently —
the most prominent position on screen spent on something every TUI user
already knows. Now `⠋ refreshing…` appears there only while the view is a
stale replay from the snapshot cache with a fresh fetch in flight, and
disappears when the fetch lands; when the deploy strip needs the room, the
word is shed before any flow name is touched and the animated spinner alone
remains (see the strip's overflow order below).

**Nothing is allowed past the right edge.** A commit subject longer than the
terminal used to push the lead time off-screen, and the viewport then scrolled
sideways to reach a column that is supposed to be pinned to the edge. The
subject is what yields: a clipped subject can still be read by widening the
terminal, whereas a lead time that isn't rendered can't be recovered at all.
Batch subheaders clip for the same reason.

**The week's stats shed whole facts rather than vanishing.** The `Deployed`
divider carries the week's deploy count and average lead time on its right.
Those used to disappear the moment the *decorative* trailing rule stopped
fitting, so they were lost at widths where they would still have rendered.

They now drop a fact at a time from the left — the week number, then the count,
leaving the average, which is the number worth reading:

```
 54 ─────Deployed ──── 1970-W02  1 deploy  2m 00s avg ────
 46 ─────Deployed ────── 1 deploy  2m 00s avg ────
 38 ─────Deployed ──────── 2m 00s avg ────
 26 ─────Deployed ────────────
```

Whole facts or none: a fragment like `…s avg` spends columns saying nothing, so
below the width where the average fits intact the label goes and the rule takes
the space back.

**Clipping is measured in display columns over grapheme clusters.** Commit
subjects are arbitrary text from `git log %s` — CJK at two columns per rune,
emoji, ZWJ sequences, combining marks, and escape sequences, since nothing
sanitises them. Slicing by rune index against a column budget overflows it for
wide characters, can run past the end of the string, and can sever an escape so
its colour bleeds across the rest of the row.

Updates live as the underlying refs change. Polls the remote every 5 seconds (configurable) using git's lightweight `info/refs` endpoint to check whether the events ref or branch tip has moved, and only does a full fetch when SHAs differ.

## The header and the deploy strip

The header answers two questions with different scopes: CI is repo-wide, deploys
are per-flow. The layout says so by naming the group — `deploy:` labels the strip,
so the flows read as sub-items of deploy rather than as peers of `ci`.

A repo with one flow renders exactly what it renders today, byte for byte:

```
your-app · ci: ✓ · deploy: ✓
```

A repo with several gains the strip, and the header row becomes chrome:

```
your-app · ci: ✓ · deploy:  web ✓   ios ✗   android ·
```

**The selected flow is cut out of the chrome, not raised above it.** The header
row is painted one step off the terminal background; the selected tab is painted
in the terminal background itself. It is therefore the only thing on that row
sharing the body's colour, which reads as a tab continuous with the content it
controls — the browser/editor idiom — and is a stronger signal than raising it
would be.

The surface is **derived from the real terminal colours**, not picked from the
palette. `lipgloss.BackgroundColor` gets the background over OSC 11 before the
program starts, and the chrome is an elevation of it, so the bar picks up each
theme's own tint: Solarized's comes out blue, Gruvbox's warm. A colour chosen
from the ANSI text palette cannot do this, which is why every fixed grey looked
pasted on.

**"No answer" has two shapes, and both mean assume-dark.** On a terminal that
ignores OSC 11 `lipgloss.BackgroundColor` can return a nil colour with a *nil
error* — Windows conhost does exactly this — so a nil background is the same
case as an error: the light/dark verdict defaults to dark and the derived
surfaces are skipped (`chromeSurface(nil, …)` returns nil, there is nothing to
elevate from). Treating nil as a colour instead crashed the whole TUI on
Windows before the first frame.

**Which way to elevate is decided by contrast, not by a light/dark verdict.** The
default foreground is queried too (OSC 10, via `tea.RequestForegroundColor` in
`Init` — by then bubbletea owns stdin, so only its event loop can read the answer
without racing its input reader). Two candidates are considered: one step lighter
(`Lighten(bg, 0.10)`) and one step darker (`Darken(bg, 0.10)`, reduced to `0.06`
on light backgrounds, where multiplicative darkening reads much stronger). A
candidate is viable only while it still contrasts with the background itself
(WCAG ratio ≥ 1.1) — lightening white or darkening near-black produces no visible
bar at all. Among viable candidates, the one keeping more contrast against the
foreground wins. Near-black and near-white themes land exactly where the old
light/dark rule put them, but a mid-tone theme — Herdr's default scheme is one —
used to get the bar lightened to sit right under its own text; it now elevates
away from the text instead of into it. See `chromeSurface`.

**The bar is one surface, so every column of it is painted** — including the
two-space joins between the header's parts, which are rendered on the chrome like
everything else. lipgloss does not re-apply an outer style after an inner SGR
reset, so wrapping the assembled row in a background style cannot do this; each
constituent, joiners included, carries the background itself. A bare join is a
hole punched through the bar.

Three consequences of the derivation, all of which need handling rather than
hoping:

- **The queries can fail** — no TTY, a terminal that ignores OSC 11, piped
  output. There is no colour to derive an elevation from in that case, so no bar
  is drawn and the selected flow is marked by weight and an underline instead. A
  fill picked from the ANSI text palette was considered and rejected: a colour
  that is not made of the background can only ever look pasted on to it. The
  foreground query failing costs less: the bar still renders, and the elevation
  falls back to the directional rule (dark lightens, light darkens).
- **Elevation needs truecolor.** On a 256-colour terminal the derived shade
  quantises to the nearest cube entry and can land back on a flat grey. Not
  detected yet — a terminal that answers the query but renders few colours gets
  the quantised bar.
- **The blank row below the header must stay body-coloured.** That is what the
  cutout connects to, and it is the whole reason the tab metaphor works. It is
  written deliberately today — `Model.View` emits `"\n\n"` between the header and
  the viewport — and `headerHeight` merely sizes the viewport to match. From here
  that row is load-bearing, so changing either without the other breaks the tab.

**The bar runs the full width** and appears only when there is more than one flow.
One flow renders flat, with no bar and nothing cut out, because a lone raised tab
looks like a control and isn't one — the same reason `ci` must not look like a tab.
`ci:` and `deploy:` stay lowercase, matching the header as it ships today.

**Status sits after each name**, matching `ci: ✓`. A dot immediately after
`deploy:` would read as the deploy group's own status, which is not a thing that
exists once flows are named.

**Colour is header-only.** A flow's badge is the same `✓` / `✗` / `·` the header
badges have always used, and the header has always been where clarity spends
colour — it is the summary, and it earns the colour that the per-row icons
deliberately forgo. Coloured `●` dots were explored and dropped, and the
reason is worth keeping: two filled dots differ only by hue, so to a red/green
colourblind reader — roughly 8% of men — the strip becomes a row of identical
grey circles with no way to tell which flow is broken. That is precisely the
failure the palette rule exists to prevent, and the strip's whole job is
answering "is anything red?" at a glance.

A hollow-passed / filled-failed pair was also considered, since filled-means-
attention survives greyscale and keeps the LED look. It was dropped for a
smaller reason: a healthy repo would then show a row of hollow rings, which
reads as "nothing has happened" rather than "all good", and the healthy state is
the common one. Per-commit rows are untouched either way: still
`✓ / ✗ / spinner / ·`, still carrying meaning by shape, still reserving red for
genuinely broken.

Flows are selected with `1`–`9`, `←` / `→`, or by clicking a cell. The
horizontal arrows belong to the strip because nothing else can use them: the
body clips every line to the terminal width and never scrolls horizontally, so
only `↑`/`↓` stay with the viewport. (`tab` / `shift+tab` was the original
cycling pair and is retired — on a row of tabs, left and right are the keys
the hand reaches for.) For three to six flows direct jump beats cycling
anyway. Selection is tracked by flow name rather than index, so a config
change that reorders the strip doesn't move the selection out from under the
user, and a flow that disappears falls back to the first rather than leaving
the body blank.

Clicking a tab works, with the cost it took confined to where it buys
something. Mouse capture applies to the whole program and takes over the
terminal's own click-drag text selection — which matters in a tool people
copy SHAs and commit subjects out of — so it is enabled only when a strip is
on screen: a repo with one flow keeps native selection untouched, and most
terminals keep it reachable under `shift` even when capture is on. The hit
areas come from the same call that renders the strip, so they cannot drift
from the layout, and wheel-scrolling the body rides along for free wherever
capture is on.

On overflow the strip degrades in stages, in the order things stop earning their
columns: the refreshing indicator's word goes first (its spinner stays — one
column that still says a fetch is in flight), then names shorten to an
ellipsis, then names go entirely, then the group label, then the separators,
and the spinner only in the final extremity. A flow is never hidden and never
scrolled out of reach, since the stuck deploy is the one most worth seeing and
would be the one off-screen; at the extreme the strip becomes a row of bare
status glyphs, which is still the higher-priority half of what a tab carries.
(Herdr, whose tab treatment this otherwise follows, scrolls instead; that is
right for a multiplexer with unbounded tabs and wrong here.)

**A tab carries a name and a status, and nothing else.** Batch size and staleness —
how many commits are waiting, how long since a flow last deployed — were designed
as a second line under each tab and deliberately dropped. They cost a header row
for every repo to serve the multi-target minority, and they were the only reason
the strip needed to be more than one line.

The information is not lost, it is just read in the body rather than the header: a
flow's rows carry their own lead times, so a month-old batch reads as a month-old
batch the moment that flow is selected. What the strip gives up is seeing that
without selecting it. That is the trade, taken knowingly in favour of a header
that costs nothing.

## Why the badges follow the newest commit

The `ci: ✓ · deploy: ✓` badges on the header line summarise the whole branch in two characters, so which event they speak for matters. Each badge takes **the status of that stage on the newest commit that has resolved it** — not the latest event for the stage overall.

Two rules follow from that, and they pull in different directions:

- **Only `passed` and `failed` resolve a stage.** `started` and `skipped` (retired, but still present in old refs) are ignored, so a badge holds its colour through a retry instead of flickering to neutral every time a build kicks off, and a commit whose deploy was skipped doesn't blank out the badge the commit below it earned. A stage no commit has ever resolved renders neutral.
- **Recency is measured in commits, not timestamps.** Two pushes close together put two CI runs in flight at once, and the older commit's run can finish *last*: push a bad commit, revert it 60 seconds later, and the revert reports green seconds before the bad commit reports red. Ranking events by timestamp would then paint the header red for a commit that is no longer HEAD and was already reverted — while `deploy` stayed green, because the bad commit never emitted a deploy event to overtake with (its deploy job `needs: build`, so it was skipped). That asymmetry between the two badges is the signature of the bug. The newest commit with an answer is the one the header speaks for; a commit that has gone quiet on a stage defers to the one below it.

Per-commit rows are unaffected — they were always scoped to their own commit.

**The rule is applied per flow.** Each strip cell resolves this over that flow's
deploy events alone, so one flow's failure cannot paint another's badge. `ci` is
the exception and resolves over the whole commit, because it does not fan out.

## Local state: the cache directory

Clarity keeps its local state in `<repo>/.git/clarity`, overridable with `--cache-dir <path>` or `$CLARITY_CACHE_DIR` (flag wins over env wins over default).

The directory holds one file, `snapshot-cache.json.gz` — the last rendered snapshot, replayed at startup so the TUI paints instantly while a fresh fetch is in flight. Safe to `rm`; it's rebuilt on the next run. (`check` still accepts `--cache-dir` so existing pre-push hooks don't break, but no longer caches anything: a push gate answering from a cache would be answering about the past.)

## Authentication

Uses whatever git auth the user already has configured (SSH agent, git credential helper). No tokens or env vars required when run inside a repo the user can already pull from.
