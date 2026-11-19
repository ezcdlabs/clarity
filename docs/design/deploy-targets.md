# Deploy targets

A monorepo with one integrated CI can still have several things to deploy — a web
backend, an iOS app, an Android app. Clarity models that as an optional `target`
on deploy events, which the TUI presents as a strip of selectable deploy flows.
The intent is to make a monorepo feel like several polyrepos without becoming a
multi-repo dashboard: one flow on screen at a time, one commit list underneath
them all.

**CI never takes a target.** `git clarity report ci passed ios` is an error, not a
filter. A target that can pass CI on its own is not integrated with the rest of
the repo, and the second word in "continuous integration" is the one doing the
work. CI answers one question for the whole commit; only deploy fans out. The
practical consequence is that "is main green?" has exactly one answer no matter
which flow is selected — which is what keeps this from degenerating into a
per-subsystem status board.

**An untargeted deploy is a flow of its own, not a shared one.** `deploy passed`
and `deploy passed ios` produce two flows, not one flow plus an overlay. A team
adding mobile doesn't have to touch the web pipeline in the same commit.

**Flows are declared in the clarity config** (see [The config file](config.md#the-config-file-clarity)),
one `[deploy "<name>"]` section per flow, each mapping the flow to the
target(s) whose events it contains:

```gitconfig
[deploy "web"]
	target =
	target = web
[deploy "iOS"]
	target = ios
[deploy "Android"]
	target = android
```

The empty `target =` is the untargeted deploy — the literal value in the data,
not a keyword. A section with no `target` lines claims its own name, so a repo
that has never renamed anything writes three bare `[deploy "..."]` headers and
nothing else. The section is `deploy` rather than `tab` because it describes
the repo's deployable units, not a UI affordance: plain mode consumes the same
config to produce stacked sections, and the config is useful documentation of
what the monorepo actually ships. (The legacy `.ezcd.json` spelling —
`clarity.deploys`, entries either bare strings or `{name, targets}` objects —
is still honoured as a fallback; see the config file section.)

**Selecting a flow: `--deploy`.** A root flag, not a plain-mode one — plain mode
narrows to that flow's section, and the TUI opens with it selected. The latter is
how a monorepo gets the polyrepo feel that motivated targets in the first place:
one terminal pane per subsystem, each pinned to its own flow, with the strip
still showing the others' health so cross-flow visibility isn't traded away.

The flag says `deploy` rather than `target` because it selects a flow, and a flow
is not a target — `{"name": "web", "targets": ["", "web"]}` makes "the web
target" ambiguous where "the web deploy" is not. Matching is forgiving: flow name
first, case-insensitively, then a target name, so `--deploy=iOS` and
`--deploy=ios` both land on the iOS flow. That requires one invariant at config
load — a flow's name may not collide with another flow's target — checked
alongside the rule that two flows may not claim the same target.

An unknown value is an error naming the known flows, in both modes. It must
never fall back to rendering everything, or to some other flow: a script or
agent that quietly reported on the wrong subsystem is worse than one that
failed, and in the TUI an unrequested flow looks exactly like a deliberate
selection. The TUI therefore quits with the same error rather than opening on
something else.

The check waits for a view that can answer it. The cached lens paints a stale
frame first, from a snapshot that may predate the flow being asked for, so
resolving there would reject a flag the fresh data is about to satisfy.

In plain mode each flow's section carries its name on the header line, so the
grep-friendliness the plain renderer advertises (`grep ✗`, `grep deploying`)
survives having several flows stacked — a hit can still be attributed to a flow.

Declaration is what makes the config an **expectation**, and that is its real
value — it gives a gap in either direction a name:

- A declared flow with no events is a failed expectation: "we say we deploy
  Android; nothing has ever reported one." That is a finding, and without
  declaration it is invisible.
- An event whose target matches no declared flow is an unexpected actual — a typo,
  or a target someone added without updating config. When config is present
  `git clarity report` rejects an undeclared target at write time, so the typo
  fails the pipeline that made it. Stray events already on the ref still surface
  as an extra flow marked unexpected, because a silently dropped deploy is the
  worst outcome available.

A repo with no config accepts and discovers everything and hides nothing, so the
single-target case still needs no setup beyond install. The configuration cost
lands only on repos that have opted into multiple deploy targets.

**Many-to-one is for identity over time, not aggregation.** `{"name": "web",
"targets": ["", "web"]}` is a web backend mid-rename: both forms land in one
continuous flow, with no history rewrite, no force-push to the events ref, and no
rename record in the data. The same mechanism covers `ios` → `mobile-ios` later.
It is *not* for folding concurrently-live systems together — merging `ios` and
`android` into one "mobile" flow makes "deployed" mean "one of them deployed" and
averages two unrelated pipelines into one lead time.

Renaming is therefore adding a target to an existing flow. A rename command that
rewrote old events was considered and rejected: it requires a force-push to
`refs/clarity/events`, and a force-push racing an in-flight reporter is the only
unrecoverable write race in the system — every other loser simply replays its
commit. It would also break backfill idempotency (content-addressed filenames
would recreate the old target on a re-run) and destroy the audit property the
per-file design exists to provide.

**Flow names must be unique, and the invented one yields.** A flow is addressed
by name — in the strip, in plain output, and by anything grepping it — so two
flows sharing a label are indistinguishable to all three. The untargeted
deploy's label is the only one clarity invents, so it is the one that gives way
when a pipeline reports a target literally named `deploy`, or a config declares
a flow by that name. It falls back to `(untargeted)`. A target name is data and
must always render as itself.

**Flows never age out.** A target that last deployed months ago is the single most
valuable cell on the screen for a team trying to improve delivery; a rule that
hid it would be inverted against the point of the tool. Declared flows always
render, however stale.

Lead time and deployment frequency are computed per flow, measured to that flow's
deploys. An untargeted deploy contributes nothing to `ios`. This is strictly more
truthful than the single-flow view was for a monorepo, where a two-minute web
deploy and a multi-day store review were averaged into one meaningless number.
