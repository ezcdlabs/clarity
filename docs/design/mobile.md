# Mobile: native apps over the same core

The TUI and the phone apps answer the same question and share everything that
decides the answer. What differs is only the drawing.

## Why mobile needs its own source

The CLI's rule is that **go-git is never used to read; git is** — git is the only
thing that knows how a working copy was cloned, and a blobless partial clone, a
shared object store and a shallow graft each break a naive reader in a different
way (see "Clone shapes and the object store").

A phone has no git binary and no working copy, so that rule cannot apply and does
not need to: clarity did the cloning itself, so it already knows the shape.
`mobile/internal/gitsource` is a go-git `core.Source` that fetches the events ref
in full and the branch at a depth, and reads back what it fetched. It is the one
place in the project go-git reads, and it is allowed to because nothing else
produced the store it reads from.

The shallow boundary is the subtlety. `Repository.Log()` walks straight past a
graft and fails on the first missing parent, but `Storer.Shallow()` records where
the boundary is — so the walk in `mobile/internal/gitsource/walk.go` stops there
deliberately and reports the history as truncated, which is the same thing the
TUI's commit limit does.

## Light mode, for the reason the terminal already had it

The TUI works on a light terminal without a line of code about light terminals,
because it never picks a colour: it asks for ANSI 1/2/3/8/12 and the emulator
answers. The phone had the same structure — nine roles, named — but only one
answer, so it stayed dark under a light system and left the status bar
unreadable.

So there are two answers now, behind a composition local. The light palette is
not the dark one lightened: a pastel green legible on near-black vanishes on
near-white, so each role is answered again at a weight that carries against a
light ground, which is what a terminal theme does when it maps the same codes
for a light profile.

**Two tones.** The chrome — the top bar, the flow strip, the ground the content
is laid on — and a darker sheet the commits are read on. A reading surface
should be the extreme of the pair: every role gains contrast against it, and no
role loses any.

**Both are neutral, and that is the second answer rather than the first.** The
brand navy was tried as the ground, for the sake of a seamless handover from a
navy launch screen, and it was too much — a tint across a whole screen is not a
brand, it is a cast, and it competed with the one colour on the screen that is
supposed to mean something. The fix was the other way round: the launch screen
follows the theme and the ground stays out of the way. The brand lives in the
mark, in the two accents taken from it, and in the navy that light mode reads
its text in — a tint you read rather than one you sit in.

On the repository page the title starts tall and shrinks into the bar as the
list rises to meet it, the sheet coming to rest under a compact bar. That is
what makes the two levels read as layers rather than as two stacked panels, and
it costs nothing: the space is only spent while you are at the top, which is
when there is nothing underneath it to spend it on.

What deliberately does *not* collapse is the lifecycle strip. "Is it green?" is
the question the app exists to answer, and an answer that scrolls away is one
you have to go back for.

The step between the two tones is deliberately small, which is why the content
has turned top corners. Close tones plus a shape read as a sheet laid on a ground;
close tones alone read as two panels that failed to match. The pattern is
Google Messages', and it is also one the TUI already has — it paints the header
row one step off the background and cuts the selected flow out of it, so a tab
is continuous with the body it controls. On the phone the selected tab is cut
to the content tone and the sheet begins immediately below it, so that idiom now
works for the reason it was invented.

The mark's yellow and blue replace the ad-hoc accents, which were within a few
points of them anyway.

**Light takes only the navy, as ink.** The mark's yellow on white is barely a
colour and its blue is a highlight rather than something legible, so neither
carries. The navy does: as an off-black it holds the brand's hue at 17:1 against
the page, which is the one place a light theme can carry the identity without
giving up contrast.

This is the one deliberate departure from the terminal, and the asymmetry is the
point. The TUI never sets a background — it inherits the terminal's, which is
why it has no brand colour and cannot have one. The phone owns its whole
surface, so it can carry what the terminal structurally cannot.

Three things had to agree for the system bars, and only all three together fix
it: the window background follows the system (a `values-night` override rather
than a hardcoded dark), the activity's theme parent follows it too, and
`enableEdgeToEdge` picks the bar icon colours from the system's own dark-mode
setting — which was right all along, and was drawing dark icons over a window
painted dark.

Nothing may reach for a literal colour. Anything that does is a thing that
looks wrong under one of the two themes, and the only way to find out is to be
holding the phone.

## The phone follows the terminal's two colour tables

Not one table. The header is the summary and earns the colour the per-row icons
deliberately forgo, which is why `ci: ✓` is green and a tick in a commit row is
grey. Red is spent only on a failure that is still breaking something: once a
newer commit has gone green the older failure is history, and goes grey with
everything else. A started stage shows as unresolved in the header rather than
as progress — the header is a verdict, and "something is happening" is not one.

A screen of green ticks would spend the one colour that means "look at this" on
the state needing no attention at all, and the first mobile build did exactly
that.

A commit row carries what the terminal's does — status icon, author, subject,
lead time — over two lines rather than one. This is the one place the phone
departs from the row and it is the screen's fault, not a preference: at this
width a single row clips the subject, which is the thing the list is being read
for. So identity and lead time share the top line and the subject gets the full
width below, indented to line up under the author. Section labels, batch
subheaders and week dividers all indent to that same column, as the terminal's
dividers do.

No sha — the terminal does not print one, and nothing on a phone can be copied
out of a list row anyway. No commit age either: the only timer on a row is the
lead time, and a second one beside it invites the reader to work out which is
which. `Commit.age` was removed rather than left unrendered, because a
preformatted string nothing draws is a string free to drift.

A deploy subheader keeps its time inline — "live on production · deployed 4m 43s
ago" is a sentence, and pushing the time to the right edge turned it into a
column and broke it.

## Weeks are divided where the terminal divides them

The Deployed section carries this week's throughput on its own rule — "2026-W40
 3 deploys  2h 14m avg" — and every other week is named above its first batch.
Both strings come from `core.WeekDividerLabel`, the same function the TUI and
the plain renderer use, so the format cannot drift between them.

Which batch carries a divider is a decision rather than a position: batches are
ordered by commit and not by deploy time, so a redeploy of an older commit puts
a week out of sequence, and a client tracking only "did the week change" would
name the same week twice. The summary is empty when the window was truncated
through this week's bucket — the count is then unknown rather than zero, and
"0 deploys" printed above the very deploys it denies is worse than nothing.

## Actions are glyphs, and rare ones are hidden

A row of words across the top of a phone is a terminal habit, not a phone one.
Add, refresh and dismiss are glyphs; the device key — a once-ever action — is in
an overflow menu rather than holding a permanent seat in the bar; and removing a
repository is behind a long press, because it is rare, it is destructive, and a
row whose whole purpose is being tapped should not carry a second thing to tap
by mistake.

Every glyph carries a label that is never drawn. It is what a screen reader
announces, and an unlabelled icon button is a button only sighted users have.

## One screen, with everything else in front of it

The repository is the app. The switcher is a bottom sheet opened from the title
you are already looking at, and connect and the device key are full-screen
flows. An earlier version put the list beside the repository as a two-page
pager — a nice gesture, and the wrong trade: a list of three rows does not earn
half the screen's horizontal swipe, and that swipe is wanted for changing deploy
target.

The bar is fixed at 64dp with a two-line title, and deliberately does not
collapse. A large title that shrinks as you scroll is a tap target that moves,
and this one opens the switcher — a control you have to look at before you can
hit it is a worse control than a smaller one that stays put.

What does not scroll away either is the lifecycle strip. "Is it green?" is the
question the app exists to answer, and an answer you have to scroll back for is
one you stop trusting.

## Connecting is a flow, not a form

Everything that can go wrong on a first connection is a step on the way rather
than an error thrown back at a text field. An unknown host is a question with an
answer and a fingerprint to check; a refused key is something to go and fix,
with the key right there to copy and git's own words behind a disclosure for
whoever wants them.

That shape is only possible because the core reports outcomes rather than
throwing: `SyncResult` distinguishes an unknown host, a changed host key and a
rejected key from everything else, so three clients can draw three screens
without any of them parsing prose.

## What ticks, and what refetches

The TUI is live; the phone has to be too, without being a battery.

Durations cross the boundary preformatted, which is right for the instant the
view was built and wrong a second later. So the model advances a clock once a
second and every timer on screen is drawn against it — one clock, so the whole
screen stays consistent with itself. A live lead time carries the instant it
started counting, which is what lets a client keep it honest without asking Go
to walk the commit graph again every second.

The formatting itself stays in Go, exposed as `Elapsed`. The alternative was
reimplementing the rule in Kotlin and in Swift: two more places for "3m 29s" to
drift into "3:29".

The open repository refetches every thirty seconds. Both the clock and the
refresh stop when the window stops being visible — a timer nobody can see only
spends battery, and a fetch nobody asked for spends their data too.

A *background* refresh that fails says nothing when there is already a view on
screen. The alternative is an error bar reappearing every thirty seconds for as
long as you are on a train, over a dashboard that reads perfectly well. The
failure is reported when you asked for it, or when there is nothing behind it to
read.
