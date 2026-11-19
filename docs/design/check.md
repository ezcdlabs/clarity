# Pre-push checking: `git clarity check`

A red trunk on a fast-moving team is rarely *discovered*. It is pushed onto. The
breakage itself is cheap — one commit, one revert — and the expensive part is the
three commits that land on top of it before anyone looks, which turn that revert
into a range and the break into somebody's archaeology.

`git clarity check` makes the moment of pushing the moment you find out.

## Why `check` is a convention, not a hook

The obvious shape is a `pre-push` hook installed by a subcommand. That was the first
design, and it was rejected for three reasons:

- **A hook rejects; it does not make anyone wait.** The thing worth having is back
  pressure — *land later*, not *land never*. A rejection only moves the decision to
  whoever ran the push, which with agents in the loop means an agent deciding,
  mid-task, how badly it wants to be unblocked.
- **The override has to be designed, and every design for it is bad.** A red trunk is
  fixed *by pushing*, so the gate must always be passable, and the person fixing it
  meets the gate first. Whatever affordance exists for them — `--no-verify`, an env
  var, a flag — becomes the thing everyone reaches for, because it is printed in the
  error already on their screen.
- **It puts clarity in the push path.** Everything else clarity does is read-only with
  respect to the user's workflow: it reads git and writes its own ref. A hook turns a
  clarity bug into "nobody on the team can ship".

`git clarity check && git push` gives up enforcement and gets all three back. The
override needs no design at all — it is *don't use the conjunction* — and that is the
one bypass that cannot calcify into a habit, because there is no flag to bake into a
wrapper, an alias or a CLAUDE.md. The person fixing the break reads the message and
runs `git push`.

What it gives up is real: it constrains only whoever remembered to run it. A hook
fires whether or not anyone was paying attention; a convention is self-enforcement by
the actor being constrained. It will not catch a `git push` typed by hand, or an agent
that never read the convention.

That trade is taken on the same grounds everything else here is: the failure is cheap
when caught and merely annoying when missed, so the enforced version has to earn its
costs. Commits landing on a red tip *after* the convention is in place is the signal to
build the hook — and clarity is where that recurrence is visible.

## What counts as red

`check` resolves CI through the same rule as the header badge — the status of the
newest commit that has **resolved** the stage (see
[Why the badges follow the newest commit](tui.md#why-the-badges-follow-the-newest-commit)).
One definition of "is main green?" per binary. A second one that could disagree with
the TUI would be worse than no check at all, because the disagreement would be
discovered during an incident.

Three consequences follow, all of them wanted:

- **In-flight is not red.** `started` does not resolve a stage, so pushing onto a
  commit whose CI has not reported yet is allowed. Serialising pushes behind an
  unfinished run is a queue, and a queue is a different tool; `check` is about
  breakage, not contention.
- **A trunk that never reported is not red.** No CI events at all, or none inside the
  commit window, exits 0. Absence of evidence is not a break, and a check that failed
  on fresh repos would be uninstalled the same day.
- **Deploy is never checked.** A failed deploy is a real problem but not one another
  commit makes worse, and deploy failure is per-flow: blocking a web-only push because
  `ios` went red is a candidacy question whose answer is already "unaffected". CI is
  repo-wide and indivisible, which is exactly what makes it the thing to gate on.

## It never asks who broke it

`check` exits non-zero whenever CI is red, for everybody — including the clone that
pushed the break. It reads no identity at all: not `user.email`, not the
remote-tracking reflog, not the red commit's author beyond printing it.

Waving through whoever broke it is the obvious refinement, and it was considered and
rejected. It cannot be done correctly in the case the command exists for. Several
agents in worktrees of one clone share a git identity *and* share
`refs/remotes/origin/main`, so every locally-checkable form of "did I push this?"
answers yes for all of them — the speed bump would vanish for precisely the cohort it
was built for, and vanish silently. The same is true of a developer with two clones on
one laptop, in the other direction.

Deciding whether *this* push is the fix needs context the tool does not have and the
pusher does. So `check` reports the fact and the consumer judges: whoever owns the fix
drops the `&&`. A predicate with one meaning is also a predicate a script can trust —
every exception would have to be learned by every caller, human and agent alike.

## It fails open

A `check` that cannot reach the remote, or that has not answered within twenty
seconds, exits 0 and says so on stderr. Offline, a stale credential, a transient
outage — none of those is a red trunk, and a tool that blocks pushes whenever it
cannot tell is a tool that blocks pushes on an aeroplane.

The warning matters as much as the exit code. Silence means green, so "could not
tell" has to look different from "clean" or the failure is invisible. It does leave
exit 0 carrying two meanings; a `--strict` mode separating them waits until some
caller needs it, because today's caller is a human or an agent reading stderr.

**Knowing that it failed is not free, and reusing the wrong port hides it.**
`core.Source` swallows its fetch and read errors on purpose: a watcher that loses
the network should keep rendering the last state it knew rather than blank the view.
Borrowing that policy here would have had `check` answer from whatever stale
remote-tracking refs the clone happened to hold — and report *green*, silently,
because the fetch that would have found the breakage never ran. That is the one
behaviour that would make the command worse than not having it. So `check` reads
through a one-shot instead (`refsource.Once`, behind a narrow `TrunkReader` port),
which returns the error the watcher discards. The requirements genuinely differ:
a dashboard should degrade quietly, and a gate must not.

## The message is the feature

The exit code decides whether the push happens. Whether anyone *learns* anything is
decided by four lines on stderr, so they carry:

- **The commit, not just the fact.** Short SHA, subject, author. "CI is red" invites a
  retry; "this commit broke it four minutes ago" invites a wait or a fix.
- **How long it has been red.** Four minutes means someone is probably on it and
  waiting is right. Forty minutes means nobody is, and the reader should take it over
  or revert it. The same two facts support both readings, and the reader is better
  placed to judge than the tool is.
- **The rule, stated.** Whoever pushed the break owns the fix; everyone else waits. The
  hook version would have had to enforce that. Saying it turns out to be most of the
  value.
- **No bypass instructions.** Whatever the message says to run is what gets run, so it
  names `git clarity` and nothing else. The way past is in `--help`, where you have to
  go looking for it.

## Open questions

- **Blocking instead of rejecting.** `check --wait` would poll until the trunk resolves
  green and then exit 0 — the only version that literally implements "wait for a fix",
  making `check --wait && git push` back pressure rather than notification. It needs a
  timeout, since an agent blocked indefinitely inside one shell call is its own
  failure mode. Not built until the plain form has been lived with.
- **How much the push would bury.** "your push adds 2 commits on top" is the sentence
  that makes the consequence concrete, but `check` reads only the remote trunk — the
  local branch is not in its snapshot.
