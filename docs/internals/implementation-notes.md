# Implementation Notes (Go)

## Acceptance tests

`internal/acceptance` wires the real composition — `.ezcd.json` on disk, `config.Load`, the Lens, the renderers — and asserts on rendered output rather than on derived values. It exists because unit tests per layer are not sufficient to show a feature works.

`clarity.leadTime` proved that. Core computed all three modes correctly and had tests to prove it; both renderers then rebuilt their own groupings from the raw snapshot and threw the configured `View` away. Every unit test passed and the setting did nothing at all. The bug lived precisely in the seam that no single-layer test looked at.

The rule that falls out: **a renderer consumes the `View` it is handed and never re-derives from `Snapshot`.** The derivation layer is the single source of grouping, lead time and DORA truth — that's what `View`'s doc comment always claimed, and what the renderers quietly violated. Anything configurable needs a test that crosses from the config file to the thing a user reads, because that is the only place the claim can be checked.

## go-git vs shelling out

Use `go-git` for: walking the commit log, and building the objects a write commits.

Shell out to `git` for anything that talks to a remote — `fetch` and `push` — because git handles credentials, transports and refspecs. And for reading the events ref, because git is the only thing that knows how this particular working copy was cloned; see "Clone shapes and the object store" below. This matches pushq's pattern.

## Clone shapes and the object store

Clarity is handed a working copy made by whatever checkout action the user's CI happens to run. It must not care how that clone was shaped — but it did, because it resolved objects through go-git, which reads `.git/objects` directly.

Two layouts that CI produces routinely break that assumption, and both present identically: a bare `object not found` against a ref that is present and correct, locally *and* on the remote.

- **Partial clone** (`--filter=blob:none`). Sets `remote.origin.promisor`, and a later fetch *inherits the filter* — so fetching `refs/clarity/events` brings the commit and its trees but leaves the event JSON blobs on the server, to be retrieved lazily on access. git does that transparently; go-git has no promisor support.
- **Shared object store** (`.git/objects/info/alternates`). How CI caches avoid re-downloading a repo on every run: the checkout keeps a mirror between jobs and points the workspace at it, so the workspace holds almost no objects of its own. go-git *has* alternates support, but its filesystem abstraction refuses to cross the repository boundary, so an absolute path to a mirror elsewhere on disk never resolves.

Two rules fall out, and between them they make clarity independent of the clone:

1. **Reads of the events ref go through git, not go-git** — `git ls-tree` plus a single `git cat-file --batch`. git is the only thing that knows how this working copy was cloned, so it handles both layouts, and any future variation, by construction. It is also where a lazily-held blob gets fetched, which a direct object-store read cannot do at all.
2. **Clarity's own fetch asks for a complete object graph** (`git fetch --no-filter`), whatever filter the working copy was cloned with. `--no-filter` arrived with partial clone in git 2.19; older git rejects the flag and the fetch retries without it, which is safe because such a git cannot have made a partial clone to begin with.

Rule 1 is what makes it *work*; rule 2 is what makes it work *well*. Once reads go through git, a lazily-held blob would be fetched on access anyway — but one object at a time, mid-read, against a ref that can hold thousands of files. `--no-filter` collapses that into the single round trip that was already happening, and it is the difference between working and failing outright where lazy fetching is refused (`GIT_NO_LAZY_FETCH=1`, which CI setups set to keep builds off the network).

The commit-log walk goes through git for the same reason, which closes a third layout: a **shallow clone**. `--depth=1` is the actions/checkout default, so it is the shape most CI repositories have — and it records a graft boundary in `.git/shallow` whose oldest commit claims parents that were never downloaded. git honours the graft and stops; go-git does not read `.git/shallow`, followed the parent pointer into a missing object, and failed the entire walk. That surfaced as `git clarity --plain` dying with "context deadline exceeded".

What is left for go-git is writing: building the blob, tree and commit objects a report commits. That needs none of this — creating loose objects in the local store always works whatever the clone's shape, and the `git push` that follows reads them back with git's own rules. **go-git is never used to read; git is.** That one line is the whole invariant, and it is what makes the clone's shape stop mattering.

Trading a fix for one checkout against a break on another is the specific failure this is all guarding against: `actions/checkout` is what nearly every consumer uses, and every one of these paths keeps working exactly as before under it.

## Distinguishing "no ref yet" from "cannot reach the ref"

These look identical at the point of failure and mean opposite things, so they are reported differently:

- **The remote has no events ref yet.** The ordinary state of every repo before its first report. Not an error — the fetch returns `nil` and the write builds from an empty tree.
- **The remote could not be reached.** A `*clarityrefs.FetchError`, naming the ref, the remote and its URL, with git's own output below it. This is where a credential or configuration problem surfaces, and it must never be phrased as something missing.

The same distinction has to be made locally, and it is easier to get wrong there. `git rev-parse --verify --quiet` exits 1 and says nothing for **both** "no such ref" and "the ref exists but names an object this repository cannot produce" — `--quiet` is precisely what suppresses the difference. So existence is probed with `git show-ref --verify`, which exits 1 silently when the ref is absent and fails loudly when it is unreadable, and only then is it resolved.

Collapsing those two is worse than the bug this all started with: an unreadable history renders as a repo that has never reported, which is an empty dashboard with no diagnostic at all. It is reachable by exactly the route that motivated the change — a shared object cache evicted between jobs leaves the ref in the workspace and takes the objects away.

**Credentials never appear in an error.** `git remote get-url` returns the remote verbatim, and embedding a token in it is routine in CI — GitHub's `x-access-token`, GitLab's `CI_JOB_TOKEN`, hand-rolled runners. It also applies `url.<base>.insteadOf` rewriting, so a repository whose own remote looks clean can still hand back a URL carrying a token injected by global config. A failed report must not write a live credential into a build log.

Two things get scrubbed, and missing either one leaks:

- **Clarity's own text**, where the remote URL is named. Textually rather than via `net/url`: `url.Parse` rejects plenty of URLs git accepts — an invalid percent-escape, a non-numeric port, a stray space — and a parse-based redactor has to decide what to do when it fails. Returning the original is a leak, and falling back to scp-style handling silently does nothing, because the `//` in a scheme guarantees a `/` before the `@`.
- **git's own output**, which clarity embeds beneath its message. git is only half a safety net: it redacts the *password* but prints the username in the clear, and `https://<token>@host/...` carries the token as the username. Since clarity runs git with `GIT_TERMINAL_PROMPT=0`, the message a misconfigured runner actually produces is `could not read Password for 'http://<token>@host'`.

The username is therefore treated as secret alongside the password, and the scrub lives in `internal/gitenv` because every package that shells out to git embeds git's output in its errors, and each one is a route to the same log.

**Git is asked for untranslated messages** — `LC_ALL=C` and an empty `LANGUAGE`, set once in `internal/gitenv`. Clarity classifies several outcomes by matching git's own text: "couldn't find remote ref" to recognise a repo that has never reported, "unknown option" to fall back on old git, plus the push-rejection and damaged-object matchers. Every one of those strings is gettext-translated, so under a non-English locale they stop matching and the behaviour they guard silently inverts — most sharply on the first report in a repo, where failing to recognise "no events ref yet" turns an ordinary first run into a hard failure.

A fetch failure is fatal to a write rather than ignored. Continuing would rebuild the ref from an empty tree, so a push that somehow landed would discard every event already recorded — and a push rejected as non-fast-forward would send the retry loop back to a fetch that fails identically, forever.

The read side (the watcher) still ignores a failed events fetch: a TUI that cannot reach the remote should keep rendering the last known state rather than blank the view.

**But that policy only holds once a first state exists.** Applied before one does, "degrade quietly" degenerates into a TUI that hangs blank forever: on a repo whose trunk isn't the "main" default and that has no `.ezcd.json` yet, the snapshot build failed on the missing branch on every poll, the watcher swallowed it every time, and nothing was ever emitted or explained. So the events-ref source gets the same startup pre-flight the GitHub source has always had: `buildSource` calls `refsource.Validate`, which fetches (best-effort, like the watcher's own first tick — a branch alive only on the remote is still watchable) and then requires the configured branch to resolve. The failure names the branch and the refs tried, and — since the usual cause is a stale or missing origin/HEAD rather than a wrong config — `main.go` appends what the trunk appears actually to be (via `initcmd.DetectTrunk`) and the git repair that fixes it: `git remote set-head origin --auto`. Since trunk auto-detection landed (see [The TUI](../design/tui.md#the-tui-git-clarity)), this pre-flight is the backstop rather than the first line: it fires only when no origin/HEAD and neither conventional name resolves — and in the TUI it renders in-frame, part of the startup machine rather than a stderr line the alt-screen would eat.

## TUI library

Bubble Tea (`charmbracelet/bubbletea`) is the standard choice in Go for TUIs of this kind. The render loop receives state updates from the watcher goroutine and re-renders.

## The watcher

A single goroutine that polls the remote on the configured interval and emits `[]Commit` snapshots whenever the events ref or branch tip moves. The TUI subscribes via a channel.

## Optimistic push loop

The same pattern pushq uses, applied to `refs/clarity/events`:

```go
for {
    err := fetchEventsRef()
    addEventFile()
    commit()
    err = pushEventsRef()
    if err == nil {
        break
    }
    if !isFastForwardRejected(err) {
        return err
    }
    // lost the race, retry
}
```

A lost race has two shapes, and both must be recognised as retryable. When the
client can already tell it is behind from the ref advertisement it declines the
push itself: `! [rejected] ... (fetch first)`. When two jobs push at the same
instant neither client knows it is behind, so the loser's compare-and-swap is
refused by the *server*, which reports it as a lock failure:

```
 ! [remote rejected] refs/clarity/events -> refs/clarity/events
   (cannot lock ref 'refs/clarity/events': is at a9f4824... but expected 1668810...)
```

Note the `remote` inside the brackets — matching `[rejected]` never sees that
line. The second shape is matched on `cannot lock ref` rather than on
`[remote rejected]`, because a remote fsck rejection is worded the same way and
needs a different recovery (dropping the local ref, not just replaying the
commit).

## Surviving a concurrent gc

`git gc` and clarity's writes contend for the same object store, and the loser is always clarity. go-git stores a loose object by streaming it to `.git/objects/pack/tmp_obj_*` and renaming it into `.git/objects/<xx>/<rest>`; go-billy creates the destination's parent directory immediately before the rename. `git gc` rmdirs empty `.git/objects/<xx>` directories — at its default two-week prune expiry, not just under `--prune=now` — so a gc landing between those two syscalls makes the rename fail with `no such file or directory`. The read side has its own version of this: a gc that repacks mid-read deletes a packfile go-git has already resolved from its `.idx`, surfacing as `packfile not found`.

CI is where this bites, because a fresh checkout has every object packed and no `.git/objects/<xx>` directories at all, so nearly every event write has to create one.

There is a second, narrower way a gc breaks a write. The objects a write creates are unreachable until the events ref moves to point at them; a gc pruning aggressively enough to collect objects that young deletes them in that window, and the push then fails locally while packing what it can no longer read. This one needs `--prune=now` or a configured `gc.pruneExpire` — auto-gc's two-week default never collects an object that young — so it only affects repos whose workflows run their own gc.

Three layers of defence, because none is sufficient alone:

- **Prevent the gc we cause.** Every git command clarity runs is prefixed with `-c gc.auto=0 -c maintenance.auto=false`. Without this, the `git fetch` at the top of the push loop spawns a detached `gc --auto` that then races the tree build a few statements later — clarity's own fetch is the most likely trigger of clarity's own failure.
- **Retry the write.** Another job, another tool in the same workflow, or a repo with `gc.auto` configured can still start one. Each loose-object write retries a bounded number of times on a rename that failed because a path component vanished. Git objects are content-addressed, so a retried write is idempotent, and the retry re-creates the pruned directory. Retries are matched on the typed `*os.LinkError` rather than on message text, so an unrelated missing file still fails fast.
- **Rebuild after a prune.** Retrying the write cannot help once the objects are already gone, so a push that fails because the local store can no longer read what it is packing drops the local ref and rebuilds from scratch, a bounded number of times. The tree is derived from the events the caller supplied, so every pruned object is simply written again. Git's wording here varies by object type — `unable to read <id>` for a blob, `bad tree object <id>` for a tree, `bad object <id>` for a commit — and mentions gc in none of them, so the match keys on the shape they share: a diagnostic naming a raw object id.
