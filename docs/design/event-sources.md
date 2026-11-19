# Event sources

## Pluggable event sources: `clarity.source = exec`

The events ref is honest about what it is: **claims, not evidence**.
Anyone with push access can write to `refs/clarity/events`, and a
`git clarity report ci passed` from a laptop looks exactly like one from
CI. For a high-trust trunk team that is fine — the ref is a shared
noticeboard, not a control. But nothing should *act* on it, and a team
that wants trustworthy answers needs events from a store whose writers
are authenticated: ClickHouse rows written by a dedicated CI user, GitHub
check runs, an internal API.

Clarity gains no transport, auth, database or verification code for this.
Instead it gains the pattern of git credential helpers and kubectl exec
plugins: a **local helper command** the team brings, which clarity runs
and believes.

```gitconfig
[clarity]
	source = exec
[source]
	arg = ./tools/clarity-source
	arg = --store=clickhouse
	timeout = 10s
```

**Protocol v1, one-shot and polled.** The command is exec'd directly with
its configured argument list plus `--protocol 1` — the argv is repeated
`arg =` lines precisely so there is nothing for a shell to parse, and no
shell ever runs; metacharacters in an argument arrive as the bytes they
are. stdin carries the trunk commit SHAs clarity is about to display, one
per line, newest first, so the helper needs no git knowledge and its
query stays bounded. stdout is one event JSON object per line —
`{"sha","stage","status","ts","target","ci"}`, the existing schema plus
the sha — and **everything the helper prints is what clarity treats as
true**. Deciding which events to trust is the helper's job, because only
it knows its store's writers; clarity applies no trust logic of its own.
Exit 0 means the output is complete. A non-zero exit, a timeout
(`source.timeout`, default 10s), or unparseable output is a failed read:
the dashboard keeps the **last good snapshot, visibly marked** — `⚠
source degraded` in the TUI header, a `source degraded: …` line in plain
output, helper stderr included — instead of the silent watcher it would
otherwise be. Helper events then flow through the same snapshot, lens and
renderers as ref events; no code path downstream distinguishes the two.

**The command never runs without this clone's human approving it.** The
configuration is shared — that is the point of it being on the trunk —
but shared config must not be able to run code on someone's machine just
because they ran a dashboard. The model is `direnv allow` / VS Code
Workspace Trust:

```sh
git clarity trust
```

shows the exact argument list and records a hash of it in **local**
`.git/config` after a typed confirmation — the word `trust`, not a
keypress. The approval pins the exact argv: any change to the configured
command invalidates it (the prompt then shows what it changed from), and
`.git/config` travels with no push or fetch, so a fresh clone always
asks its own human again. Where no human can answer — plain mode, hooks,
`check`, any non-TTY — clarity never prompts and never runs an
unapproved command: the events ref serves instead, with every frame
marked `source degraded: … run \`git clarity trust\`` so the fallback is
never mistaken for the real thing. The prompt is honest about its limits:
if the command runs code from the checkout (`bazel run //tools/...`),
approving it approves future versions of that code too.

Two placements follow from the security model rather than convenience:
the source command is honoured **only from `.clarity` on the trunk**,
never from the legacy config ref — a silently rewritable ref is exactly
where an attacker would put an executable command, so one found there is
dropped with a warning and the events ref serves. And `check` holds the
same lines: an approved helper answers it through the strict one-shot
read (a helper failure is "could not tell", which lets the push through —
never a stale green), while an unapproved one simply never runs.

**Writes are out of scope for the helper.** CI writes to its own
authenticated store through whatever path that store requires;
`git clarity report` and the events ref keep working for teams that use
no helper. The mobile client reads the events ref only, so a repo whose
source is a helper renders its trunk with no events there — a recorded
limitation until a relay exists.

## The GitHub Actions live source: retired

Earlier builds could point the TUI straight at the GitHub Actions API instead of the events ref, via a `clarity.github` workflow/job mapping in `.ezcd.json` written by an interactive `init --github` wizard. That adapter is gone, and the decision is worth recording:

- **Nobody used it, and everybody paid for it.** It carried a runtime dependency on an installed-and-authenticated `gh`, and essentially every field failure of the early-tester period — unauthenticated gh on WSL2, repo-slug resolution, workflow mapping against the wrong branch — was its surface.
- **Agent-driven backfill removed its reason to exist.** The adapter's value was "a dashboard before your pipeline reports events"; `git clarity onboard` gets a repo to the same place by backfilling real events once, after which the events ref — the format every other part of clarity already speaks — carries everything.
- **It was the only read-only backend, and that is the honest cost.** The events ref requires CI to push. Removing the GitHub poller removes today's answer to "my reporter can't have write access" — a deliberate bet that the better answer is a future `clarity.source` pointing at a separate events repository, which fits the ref model instead of bypassing it, and helps every CI system rather than only GitHub Actions.

The hexagonal split keeps the bet cheap: `core.Source` is unchanged, and a future adapter re-enters through the same port the retired one left by. Two lessons from its code outlived it: subprocess stderr belongs in the error (`exit status 4` is actionable only once gh's own "run `gh auth login`" rides along, credential-redacted via `gitenv.Redact`), and every Source gets a startup `Validate` pre-flight so a misconfiguration fails with words rather than an empty dashboard.
