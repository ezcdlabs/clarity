# Lead time

Each row's right-hand column is that commit's lead time, and the weekly `avg` is the mean of the ones visible in the Deployed section. What "lead time" starts from is configurable, because the honest answer depends on what you intend the number to change:

```sh
git clarity config leadTime all
```

| Mode | Which commits count | Measured from |
| --- | --- | --- |
| `all` | every commit that reached production | its authoring time |
| `reported` | commits carrying at least one pipeline event | its authoring time |
| `pipeline` (default) | commits carrying at least one pipeline event | its earliest event |

**Why the choice exists.** CI usually runs on the pushed head, not on every commit in a push. A developer who makes five small commits and pushes them together produces one commit with events and four without — but all five reached production, so under `all` they contribute five samples, each carrying however long the work sat unpushed. The average ends up describing commit-and-push habits as much as the pipeline. Someone who commits on Friday and pushes on Monday moves the team's number more than any pipeline change would.

**`pipeline` is the default** because that Friday-to-Monday batch is the first thing every new user saw: a dashboard claiming days of lead time over a pipeline that takes minutes, which reads as a broken tool rather than a measured truth. An unset config now measures the pipeline — the thing clarity's pitch says it watches — and the DORA-faithful `all` is a deliberate, visible opt-in rather than a default nobody read. (`all` was the default originally, chosen so upgrading never moved anyone's numbers; field experience ranked the first impression above that stability, and the flip is the one time the numbers move without a config edit.)

`reported` removes the multiplier — one sample per pushed batch instead of one per commit — but not the magnitude. The surviving commit is still timed from when it was authored, so the Friday-to-Monday gap is still in the average, just once instead of five times. Choose it to keep the DORA definition intact and only stop the double-counting.

`pipeline` is the one to choose if the number is meant to drive pipeline work. Time spent unpushed, in review, or otherwise before CI took the code stops counting. Three consequences, all deliberate:

- It is **not** DORA lead time for changes, which includes commit-to-deploy precisely because unpushed work is real delay in the value stream. Don't report it as DORA.
- Anything before the first event is invisible, **including runner queueing** — which is genuine pipeline performance you stop seeing. `ci started` is reported from inside the job, so a saturated runner pool looks free.
- The number depends on reporting discipline. A team reporting only `ci passed` and `deploy passed` measures from the *end* of CI and gets a much smaller number than one that also reports `ci started`.

**Commits without a lead time still appear in the log.** Under `reported` and `pipeline` they render without a right-hand column and contribute nothing to the average. The intent is to stop them skewing the number, not to hide that they shipped.

**Non-positive intervals are dropped, not clamped.** A rebased or amended commit can carry an author date later than the deploy that shipped it; a commit can inherit a deploy from a newer commit that landed earlier; and under `pipeline`, a team reporting only terminal events has its start land on the deploy itself. All three produce a zero or negative interval, none of them represent fast delivery, and a run of them would quietly report a perfect pipeline. Such commits contribute nothing.
