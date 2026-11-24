# Candidacy: affected / unaffected

Flows share one commit list, so every trunk commit sits above every flow's last
deploy. Left alone, an Android deploy weeks later scoops up hundreds of
web-only commits and each contributes a lead time measured from its authoring
time — turning Android's number into "the average age of every commit in the
monorepo" rather than "how long Android changes take to ship". That poisons the
DORA metric the tool exists to provide.

The fix rests on separating two questions that had been collapsed into one:

1. **Is commit X in the Android app?** No, definitively, whatever files it
   touched. Integrated CI means a commit is indivisible, so the behind-count
   stays unfiltered — and rightly: the binary is built from a tree containing all
   of those commits, and a shared-library change breaking Android is exactly why
   the CI is integrated. The batch-size risk is real.
2. **How long did it take to deliver Android change Y?** Only meaningful for
   commits that contained Android change.

Candidacy answers (2) only. It changes lead time and DORA attribution, and
touches neither the behind-count nor the deployed/not-deployed grouping.

```yaml
- run: |
    for t in web ios android; do
      if affected "$t"; then
        git clarity report affected "$t"
      else
        git clarity report unaffected "$t"
      fi
    done
```

**Why not a status.** `started` / `passed` / `failed` are outcomes of an attempt.
Candidacy is not an attempt — nothing was tried. It is a property of the commit,
true from the moment the commit exists, which is why it can be reported before
`ci started` has even run. `affected` is the word nx, turbo and bazel already
use, so whoever writes that `affected` helper is using it too. The `skipped`
event status used to be the nearest-looking tool for this job and was retired
in candidacy's favour — see [Event content](data-model.md#event-content).

**Candidacy is not convergence.** The boundary runs the other way too. Two
commits in a row where the second reverts the first both modify the target's
inputs, so both are honestly `affected` — yet the cumulative result is that
the deployable's bytes match what is already in production, and no deploy
happens. Reporting them `unaffected` would make the records per-commit lies,
and it wouldn't help anyway: candidacy touches only lead-time attribution,
never the deployed/not-deployed grouping, so the pair would still sit in CI
Passed. That case is the deploy's own outcome, discovered at build time by
comparing against what is deployed — not a property of either commit — and it
is reported as the `noop` deploy status, which completes their lead time
without counting a deploy. The rule of thumb: *unaffected* is knowable from
the commit's diff alone; *noop* is only knowable by asking production.

**The record.** Candidacy lives in a sibling `scope/` tree on the same ref, at
`scope/<sha>/<unix-ts>-<hash>.json`:

```json
{ "target": "ios", "affected": false, "ts": 1744120134000000000 }
```

`ts` is Unix **nanoseconds**, unlike an event's seconds. Candidacy supersedes
by timestamp — a later record corrects an earlier one — so the resolution has
to be finer than the rate records can be written at, or two reports in the same
second tie.

All three fields are required on read. `affected` is never omitted when written
and a record missing it is rejected rather than defaulted, because false and
absent are different answers — and an event JSON is structurally valid
candidacy JSON, so a record that strayed into the wrong tree would otherwise
read back as a confident "unaffected" for the untargeted deploy. An empty
`target` is refused on write for the same reason: empty means the untargeted
deploy, so a forgotten field would record a real claim about a real flow.

Records come back in timestamp order, which is what makes "the latest record
wins" resolvable — git returns tree entries in filename order, which is
lexicographic rather than chronological.

**A tie resolves to affected.** Two records for one target at the same instant
have no honest winner: the ref preserves both and neither is newer. Resolving
to affected errs towards counting a commit, which can only inflate a lead time,
whereas the other way could hide delivery — and a number that flatters is the
worse failure for a metric meant to drive work. Without an explicit rule the
winner would be whichever content hash sorted first, so a correction could be
discarded while the command that wrote it reported success. Correcting a record
therefore needs a later timestamp, which is automatic unless `--at` pins it.

**Three states, not two.** Reporting both directions carries more information
than reporting only exclusions:

| What the ref says | What it means |
| --- | --- |
| nothing | nobody has told us; every commit is a candidate (the default, and today's behaviour) |
| `unaffected` on 336 of 340 | we know; this flow's lead time comes from the other four |
| `affected` on all 340 | CI is explicitly saying everything affects this target — a different answer from silence |

Because the default is "candidate", nothing moves for a repo that never reports
candidacy. Same principle that makes `all` the default lead-time mode.

**Candidacy under each lead time mode.** How a record applies depends on what a
sample stands for. Under `all`, every commit is its own sample, so its own
record decides. Under `pipeline` and `reported`, one event-bearing commit holds
the sample for a whole pipeline run — CI ran on the pushed tip, and the
event-less commits swept below it went through CI in that same run. The tip's
record alone therefore cannot decide: a push's tip can be unaffected while an
older commit in the same push holds the change this flow ships, and that run
still delivered it. The sample survives if **any** commit the run carried is a
candidate, and silence means candidate here as everywhere, so one
partially-reported push can only err towards counting — the same direction the
tie-break errs. Records stay truthful per commit; the mode decides how they
aggregate. The alternative — reporting one base..head verdict on the tip,
nx-style — was rejected because it makes a record's meaning depend on which
lead mode reads it, and a later switch to `all` would turn every tip record
into a per-commit lie.

The CI recipe that follows: evaluate each commit's own diff rather than the
push's, e.g. loop `git rev-list $BASE..$HEAD` and report `affected` /
`unaffected` per commit against each target's scope.

**Rendering: recede, don't mark.** A commit resolved unaffected keeps its
place in the flow's log (never reordered, never hidden) but renders dimmed,
with an empty lead-time slot, so a stretch of unaffected commits recedes and
the one genuinely waiting commit is the bright row. The dim is the only
signal, and that is a revision: the first design also put a gray italic `∅`
in the slot, reasoning that gray is relative (light themes, colour-blindness)
and the glyph was the explicit answer in the column where the eye looks for a
number. The first real monorepo flow overturned that: per-flow candidacy
makes unaffected the *common* case, so the glyph appeared on nearly every
row — a column of `∅` repeated down the screen is texture, not signal, and
the information it claimed to carry was already carried by the bright rows'
actual numbers. A marker earns its ink by being the exception; this one was
the rule. The theme-robustness worry stands down for the same reason: if the
dim fails, an unaffected row degrades to an ordinary-looking row with no
timer, and the left-column icons still distinguish every row without colour.
A commit that is itself unaffected but holds its run's surviving sample keeps
its timer and stays bright: that row carries a real measurement. `--plain`
still spells `unaffected` in full — that renderer exists for grep and for
agents, dimming doesn't survive a pipe, and there the word is the whole
signal, not a per-row decoration. The intent is unchanged: stop the commit
skewing the number, not hide that it shipped.

**Candidacy records are not pipeline events.** The `reported` and `pipeline` lead
time modes count "commits carrying at least one pipeline event"; a commit whose
only record is `unaffected android` must not thereby acquire a lead time. The
separate `scope/` tree makes that easy to get right and easy to get wrong by
accident, so it carries a test.

**Why clarity does not infer this from paths.** A config like
`{"name": "android", "paths": ["apps/android/**"]}` is tempting — declarative, no
CI changes, and it would work retroactively on history where events only fix
things going forward. But which commits affect a target is a dependency-graph
question, not a glob question; that is what bazel, nx and turbo exist for. A
change under `libs/shared/` affects Android and matches no Android glob. Clarity
records what the build system says and does not infer build semantics.
