# Weekly Metrics: `git clarity metrics`

`git clarity` answers *"is main green right now?"* — a live view of the most recent commits. It cannot answer *"are we getting better?"*, because trend is invisible in a list of commits. That is a different question, for a different reader, on a different cadence, and it gets its own command rather than a mode of the TUI.

```
git clarity metrics                      # interactive, opens on the first flow
git clarity metrics --deploy ios         # open on one deploy flow
git clarity metrics --weeks 26           # a longer window
git clarity metrics --limit 0            # read the whole history, not the default window
git clarity metrics --plain              # print once and exit
```

`←` / `→` switch flow (clicking a cell works too, same bargain as the live view's strip), `1`–`9` jump to one, `↑`/`↓` scroll a long history, `g` returns to the newest week, `q` quits.

The top bar carries what the reader needs to know they are looking at: the repo, the span of weeks on screen, and which flow is selected. A repo with no targets still gets one — the strip is what varies with targets, the bar is not.

It deliberately does **not** carry ci/deploy status, and the deploy strip here shows no status glyphs. Those answer *"is main green right now?"* — the live view's whole question — with a value frozen at launch that goes stale while this view is read. A badge that looks live and is not is worse than no badge, and leaving glyphs on the tabs while removing the badges would apply the reasoning half-way. The strip is a selector here, not a status display.

The footer lists only keys that change what is on screen; every terminal program quits, and saying so spends the line on the one hint carrying no information.

## Lead time mode

The weekly view obeys `clarity.leadTime` exactly as the commit list does — it builds its lens the same way, so `pipeline` measures from the first pipeline event here too. The mode changes the distribution and therefore the axis, not only the average, which is a wider surface than the deploy strip has.

That is guarded by a test crossing from a config file to the derived numbers, on both lens paths, because this is the shape of the bug that produced the renderer rule: core computed every mode correctly and had tests proving it, both renderers rebuilt their own groupings from the raw snapshot, and the setting did nothing while every unit test passed.

`--weeks` bounds what is drawn; `--limit` bounds what is read. They are different windows, and the second can cut the first short — when it does, a footnote says so, because an aggregate that is a few deploys short is wrong in a way no reader can see.

One row per ISO week, newest first: a box plot of that week's lead times on the left, a bar of that week's deploy count on the right.

```
             lead time  ·  median, p25–p75                                    deploys
  2026-W39       ├─────▓▓▓▓█▓▓▓───────────────┤                █████████████████  18
  2026-W38     ├───▓▓▓▓█▓▓────────────┤                   ██████████████████████  24
  2026-W37          ├─────▓▓▓▓▓▓▓█▓▓▓──────────────────┤                ████████   9
  2026-W36        ├─────▓▓▓▓▓▓█▓▓────────────────┤                    ██████████  11
  2026-W35      ├────▓▓▓▓█▓▓▓───────────────┤                    ███████████████  16
  2026-W34     ├───▓▓▓█▓▓───────────┤                        ███████████████████  21
            0─────────2h─────────4h─────────6h────────8h
```

The two halves are one signal. A bad week pushes the box right *and* pulls the bar back, so degradation reads as ink migrating rightward across the whole row — which is the entire reason the bars are mirrored rather than growing from a shared left edge.

## On a phone

The Android app draws the same view, reached from a chart icon in the repository
bar — left of the menu rather than inside it, because the chart is a place you
go and a destination behind a menu is one you forget exists. Everything in that
menu is an action on the repository; this is not one.

It is a screen with a back arrow rather than a sheet. A sheet is for a short
interaction, and this is a surface you scroll, compare rows on, and switch flows
within.

The split of labour is the proto boundary's usual one, and this view is the
clearest case of it. What crosses is the numbers and the words: each week's
five-number summary in seconds, its deploy count, the shared axis, the tick
labels, and the core's decision about whether a week has enough deploys for a
box plot to mean anything. What does not cross is a single thing the terminal
does about being a terminal — no cell reserved for the clamp mark, no one-cell
minimum bar, no three-step tick shedding. The phone reserves pixels for the
clamp chevron, floors the bar at 2dp, and keeps the ends and the midpoint of the
axis, because it is narrow in a different way.

Three decisions moved into `core` to get there, and the test that drove one of
them found a bug in the terminal's axis: `FormatAxisTick` rounded to whole
hours, so a 2h axis labelled its three-quarter tick `2h` and then labelled its
end `2h` as well — the exact collision the function's doc claimed to prevent.
Hours now carry a decimal where they need one, as days already did.

It is read when the screen opens and never again. The refresh pump leaves it
alone, for the same reason the command does not poll: re-reading two thousand
commits every five seconds spends a phone's battery to learn nothing about
history.

Opening it does fetch, though, exactly once. The feed clones shallow — 200
commits, which on a busy repository is a fortnight — and **no commit limit can
read past a shallow boundary**, so the chart asks for a deeper branch before it
reads anything. Once the device holds the history, the next open brings nothing.

That deepening fetch did not work at first, and the reason is worth recording.
go-git appends to the shallow set on every fetch carrying a depth and never
removes an entry that has stopped being a boundary — it does not act on the
server's `unshallow` lines. The deepen was in fact bringing everything, and the
walk was then stopping at the *old* graft with the rest of the history sitting
unread in the store: from the outside it looked exactly like a fetch that did
nothing. `grafts` therefore asks whether a boundary's parents are actually in
the store rather than trusting the file, which costs one object lookup per
boundary and is also what stops the feed's next shallow refresh from cutting the
history straight back again. Closing the screen drops the chart rather than keeping it, which is the
opposite of what the commit feed does with its view — a stale feed still answers
"is main green?" usefully, but a chart held over from before a fetch answers
"are we getting better?" with an older answer than the one on disk, and nothing
on it would say so.

iOS does not have this screen yet.

## Interactive, but not live

Two different things, easily conflated: **live** means a watcher polling the remote; **interactive** means an event loop responding to keys. This view is the second without the first.

It reads one snapshot and never polls. There is no tick, no re-fetch, and no staleness to signal — so it can afford to read far more history than the live view ever could.

It is still a Bubble Tea program, because the deploy strip is a control. Rendering the tab bar while requiring `--deploy` to change flows would be showing a control that does not work. Switching flows costs nothing: every flow is already derived on the `View`, so selection is local UI state with no round trip to the Lens (see `FlowView`).

Not a mode of the main TUI, though. A *view* selector there would have had to coexist with the deploy-target tabs as a second orthogonal dimension, and the tick budget would have capped the history.

Piped or redirected output falls back to a single static render, the same way `--plain` does for the live view — a tab bar is meaningless when there is no keyboard. That path uses a fixed width rather than probing the terminal, so piped output is byte-identical run to run and there is nothing to measure when the destination is a file.

Because there is no tick to keep cheap, the one-shot read takes a far larger commit window than the live view's default. The cost is paid once at startup.

## Box plots, and medians

Lead times are strongly right-skewed — one commit that sat over a weekend drags a mean badly — so the headline is the median, and the box plot gives it for free. Note this is a change in published numbers: `WeekStat.AvgLead` is a mean today.

Spread matters as much as speed. A team with a 2h median and a three-day p90 has a problem the median alone hides, which is why the whiskers stay and an interquartile band is not enough.

`WeekStat` therefore needs quantiles it does not currently carry — `Min/P25/P50/P75/Max` on the View, not raw samples, so the renderer stays dumb per the rule in "Acceptance tests".

**Below five deploys a week, plot the points instead.** A box plot over two deploys is theatre: quartiles of two numbers mean nothing. The strip form degrades honestly — two deploys look like two deploys.

That needs the individual values, not the summary: below the floor the quantiles are interpolations between two or three real numbers, so drawing them would put a smear on screen where there were only two deploys. `LeadQuantiles.Samples` carries them, and only below the floor, so it cannot grow with a busy week.

## One axis per flow, chosen from that flow's data

Per-row autoscaling would make the rows incomparable, which is the whole point of the view. A fixed ceiling is no better: it wastes the width for a fast team and squashes a slow one.

The axis is per flow, for the same reason `Weekly` is: a store review and a web deploy need different scales, and one axis across both squashes the faster of them into a column. Within a flow it is shared across every visible week, and derived from that flow's pooled lead times:

1. Take Q1 and Q3 of the **log** of every lead time in the window.
2. Fence = `exp(Q3 + 1.5 × IQR)` — Tukey's rule, which is the box plot's own definition of an outlier, so it answers exactly the question being asked.
3. Axis maximum = the next readable boundary at or above `min(largest value, fence)`.

It is computed from the raw lead times inside `core`, not from the per-week quantiles. Pooling quantiles instead would give a two-deploy week the same weight as a forty-deploy one and bias the fence. The result reaches the renderer as `FlowView.LeadAxis`, so nothing downstream re-derives it.

**The log step is load-bearing.** A raw Tukey fence clips tidy data, because a long tail is normal for lead times and the raw fence reads ordinary skew as a crowd of outliers. In log space the fence is multiplicative, which is how lead times actually vary: *five times the typical commit* is an outlier, *two hours longer than the typical commit* is not.

A percentile is the wrong instrument for the same reason it looks right. p95 is a quantile, so there is always data above it: it clips whether or not anything deserves clipping.

The result satisfies both constraints at once — tidy data is never clipped, and a single 74-hour outlier is clipped without compressing every other week into two columns.

Rounding to a readable boundary does two jobs: the ticks read as durations rather than arbitrary fractions of the data, and the axis stops moving every time a deploy lands, so weeks stay comparable between runs and not only within one render.

**The `→` marker and the `+` on the final tick must agree.** A value landing exactly on the axis is shown, not clipped. Drawing the arrow on `>=` while setting the `+` on `>` makes the view contradict itself about whether it is showing everything.

## Whole weeks only

The window is a number of whole weeks, not a commit count. A week the window cuts through is **dropped, not drawn partial** — its deploy count and its distribution would both understate, and there is no honest way to render that in a bar.

This is the `WeekUnknown` principle from the deploy strip, taken further: rather than inventing a way to say "we do not know", do not show the row. It removes a state from the code as well as from the screen.

A commit-count limit would also make how far back you can see depend on how busy the repository was, which is not a useful property for a trend view.

## Colour and weight

- **Box plot** — blue (ANSI 12), the existing accent. The interquartile box is `▓`, the median a solid `█`.
- **Deploy bars** — **ANSI 8**, the same grey as the `lead time` label, as full blocks.
- **Counts and week labels** — the terminal's default foreground.

Grey for the bars is not an absence of choice. Green already means *passed* and amber *in-flight*, so either would assert that a deploy count is good or bad; grey carries no verdict, which is right for a measurement. It is also already in the palette, so there is no 256-colour dependency and no fallback to write, and it works on light and dark backgrounds without detection.

The concern that grey would collide with the chrome — the axis and labels are ANSI 8 too — does not survive contact: a solid 22-column block and thin glyph text read as completely different things at identical colour. Shaded variants (`▓`, `▒`) were tried and are too weak, especially on light backgrounds, where short bars nearly vanish and the length comparison is lost.

## Narrow terminals squeeze the plot, they do not replace it

The box plot stays a box plot down to about 10 columns. Precision degrades, but which weeks are better or worse remains readable, which is what the view is for. Two things shed before the plot does:

- **Axis ticks**, which collide first — intermediate labels drop, leaving the ends.
- **The empty-week note**, `· no deploys`, which becomes `·`.

An earlier design fell back to text (`3h 18m (2h 08m–5h 55m)`) below a width threshold. It reads well and is more precise, but it is not scannable down a column, and scanning is the point.
