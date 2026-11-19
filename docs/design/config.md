# The config file (`.clarity`)

Clarity's shared configuration is a `.clarity` file committed at the root
of the repository, in git-config syntax — the dialect git users already
read and git tooling already edits:

```gitconfig
[clarity]
	source = events-ref
	leadTime = all
[deploy "web"]
	target = web-eu
	target = web-us
[deploy "mobile"]
```

`clarity.source` names where events come from. This build understands two
values: `events-ref` (the repo's own `refs/clarity/events`, the default)
and `exec` (a local helper command — see
[Pluggable event sources](event-sources.md#pluggable-event-sources-claritysource--exec)).
The key is the dispatch point further sources plug into — another
repository's ref, another backend — and an old binary reading an unknown
value refuses loudly rather than silently rendering the wrong repo's
events as this one's. `clarity.leadTime` is the
[lead time mode](../lead-time.md#lead-time); `[deploy "<name>"]` sections declare the
[deploy flows](deploy-targets.md#deploy-targets). Unknown sections and keys are ignored —
the forward-compat contract — but an unknown `source` is not, because it
means the events live somewhere this build cannot read.

**The file is read from the trunk's tip, never from the working tree.**
Every reader asks for `origin/<trunk>:.clarity` (the local branch head only
in a repo with no remote-tracking ref at all), so editing the file locally,
or checking out a colleague's branch that carries a different one, changes
nothing about what clarity runs. What the config says is whatever landed on
the trunk — reviewed, protected, visible in `git log` — and nothing else.
This matters most for `clarity.source`: configuration that decides where
data comes from (and will eventually name things that execute) must not be
switchable by whichever branch happens to be checked out. The one thing a
working-tree `.clarity` does is count as "set up", so the welcome pane
doesn't greet the person whose setup commit is still in flight; its values
wait for the trunk.

**Existence is the setup signal.** "Has clarity been set up on this repo?"
is one raceless, binary fact — `.clarity` is on the trunk tip or it isn't —
which is what the TUI's welcome pane keys off, and what committing the file
`git clarity init` writes establishes. No heuristics over event counts.

**Why a committed file and not a ref.** This is a decision that was made
twice, in opposite directions, so the reasoning is worth recording. The
config first shipped in a `refs/clarity/config` ref — a commit chain whose
history was the audit trail, fetched like the events ref, keeping the
working tree clean — and was walked back within days, because the ref's
convenience is exactly its flaw: **hosting platforms cannot protect custom
refs.** No branch protection, no required review, no CODEOWNERS applies to
`refs/clarity/*`; anyone with push access can rewrite it, force-push
included, and no one is notified. That is a tolerable home for machine
facts like events, and the wrong one for configuration — a config that will
one day name an event source is a config that decides what runs on
someone's machine, and "anything that decides what runs lives where code
review and branch protection apply" is the rule. Each of the ref's
rationales survives the move: the setup signal is the file's existence at
the trunk tip, equally raceless; the audit trail is `git log -- .clarity`,
now made of reviewed commits instead of unreviewable ones; the checkout-less
mobile client already fetches the trunk's commits to render them, so it
reads the same blob with no working tree; and stale copies on branches
don't matter because no reader looks at a branch. The one genuine loss is
`git clarity config` as an instant set-and-push — a set now writes the file
and the change takes effect when it lands on the trunk. That slowness is
the review, and giving it up is the point.

**There is deliberately no `branch` key.** The trunk is detected from the
repository itself (see [The TUI](tui.md#the-tui-git-clarity)); a config that
could disagree with the repo would just be a second place for that truth
to go stale. A repo whose trunk detection fails is fixed with
`git remote set-head origin --auto` — a git repair that helps every
origin/HEAD reader, not a clarity setting. (It also keeps the bootstrap
non-circular: the config lives on the trunk, so the config cannot be what
names the trunk.)

**Two legacy stores remain fallbacks, not peers.** Without a `.clarity` on
the trunk, the `refs/clarity/config` ref is still honoured — with a
one-line notice pointing at the migration — and without either, a committed
`.ezcd.json` (the original config file, shared with the broader ezcd
tooling idea that was since dropped) still loads, including its pinned
`branch`, the one place a branch pin survives. The moment `.clarity` lands
on the trunk both are ignored outright rather than merged: two half-applied
configs would be undebuggable. Migration is one command — `git clarity
init` on an unmigrated repo seeds `.clarity` from the ref's exact content
(as does setting any key), leaving the commit to the user. The legacy
parsers and this paragraph are all scheduled to go once the known installs
have migrated.
