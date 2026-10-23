// Package checkcmd is the decision layer behind `git clarity check` — the
// pre-push question "is CI red on the trunk?" reduced to an exit code and, when
// the answer is yes, four lines of stderr.
//
// The classification is a pure function of a Snapshot. Everything that can
// fail — the fetch, the commit walk, reading the events ref — stays behind the
// Source port, so both the decision and the message it prints are testable
// against typed inputs, and the rule cannot drift from the one the TUI's
// header badge uses.
//
// Why this is a convention (`git clarity check && git push`) rather than an
// installed pre-push hook is recorded in README.md. The short version: a hook
// has to offer a bypass, because a red trunk is fixed by pushing and the person
// fixing it meets the gate first — and whatever bypass exists becomes the thing
// everyone reaches for. Dropping the conjunction is the one form of bypass that
// cannot calcify into a habit, because there is no flag to bake into a wrapper
// script.
package checkcmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ezcdlabs/clarity/internal/core"
)

// Outcome is what check concluded.
type Outcome int

const (
	// Green means CI is not red on the trunk. It covers two cases that are
	// different to a human and identical to a push: the newest commit that
	// resolved CI passed, and no commit has resolved CI at all. Absence of
	// evidence is not a break.
	Green Outcome = iota
	// Red means the newest commit that resolved CI failed.
	Red
	// Undetermined means check could not find out — offline, a credential
	// prompt, an unreadable ref. It does not block the push.
	Undetermined
)

func (o Outcome) String() string {
	switch o {
	case Green:
		return "green"
	case Red:
		return "red"
	case Undetermined:
		return "undetermined"
	}
	return fmt.Sprintf("Outcome(%d)", int(o))
}

// ExitCode is the process status for an outcome, and the entire interface
// `check && git push` sees.
//
// Only Red is non-zero. Undetermined exits 0 on purpose: a tool that blocks
// pushes whenever it cannot tell is a tool that blocks pushes on an aeroplane.
// The cost is that 0 carries two meanings, which is why Undetermined still
// writes to stderr — silence is what green means, so "could not tell" has to
// look different or the failure is invisible.
func (o Outcome) ExitCode() int {
	if o == Red {
		return 1
	}
	return 0
}

// Result is an outcome plus the text to write to stderr. Report is empty for
// Green and only for Green.
type Result struct {
	Outcome Outcome
	Report  string
}

// Result builds a Result for an outcome reached without a snapshot. Only
// Undetermined is reachable that way — it is what the caller returns when the
// fetch or the walk failed — so reason is the underlying error, quoted
// verbatim rather than flattened into "could not check": the difference
// between an expired credential and an unreachable host is the whole content
// of the message for whoever has to act on it.
//
// The verdict leads and the reason follows, because git's failures run to
// eight lines of hint text and the one thing the reader must not have to dig
// for is whether their push is about to happen.
func (o Outcome) Result(reason string) Result {
	if o != Undetermined {
		return Result{Outcome: o, Report: reason}
	}
	return Result{
		Outcome: Undetermined,
		Report: fmt.Sprintf(
			"could not tell whether ci is red, so not blocking the push:\n%s",
			strings.TrimRight(reason, "\n"),
		),
	}
}

// Decide classifies a snapshot of the trunk. branch names the branch it was
// read from, for the message; now is the clock the "how long has it been red"
// figure is measured against.
//
// CI only. A failed deploy is a real problem but not one another commit makes
// worse, and deploy failure is per-flow — blocking a web-only push because
// `ios` went red is a candidacy question whose answer is already "unaffected".
// CI is repo-wide and indivisible, which is what makes it the thing to gate on.
func Decide(snap core.Snapshot, branch string, now time.Time) Result {
	if len(snap.Commits) == 0 {
		return Undetermined.Result(fmt.Sprintf("read no commits on %s", branch))
	}

	// The same rule as the header badge, through the same function — see
	// core.CurrentStage. A second walk that could disagree with the TUI would
	// be discovered during an incident.
	res, found := core.CurrentStage(snap.Commits, "ci")
	if !found || res.Status != "failed" {
		return Result{Outcome: Green}
	}
	return Result{Outcome: Red, Report: redReport(res, branch, now)}
}

// redReport is the four lines that do the actual work. The exit code only
// decides whether the push happens; whether anyone learns anything is decided
// here, so it carries the commit rather than the fact ("ci is red" invites a
// retry; a named commit invites a wait or a fix), who owns the fix, and how
// long it has been broken — four minutes means somebody is probably on it,
// forty means nobody is, and the reader is better placed to judge which than
// this function is.
//
// What it must never carry is the way past itself. Whatever the message says
// to run is what gets run.
func redReport(res core.StageResolution, branch string, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ci is red on %s\n\n", branch)
	fmt.Fprintf(&b, "  %s  %s\n", shortSHA(res.Commit.SHA), res.Commit.Subject)
	fmt.Fprintf(&b, "  %s, %s\n\n", res.Commit.Author, age(res.Time, now))
	b.WriteString("whoever pushed that commit owns the fix. if that is not you, wait for\n")
	b.WriteString("green rather than landing on top of it — your commits will bury the\n")
	b.WriteString("break and turn a one-commit revert into a range.\n\n")
	b.WriteString("  git clarity    watch for green\n")
	return b.String()
}

// age renders how long the stage has been in this state. A CI machine whose
// clock runs ahead reports an event in the future, and "-3m 00s ago" reads as
// a broken tool rather than a broken trunk — but the commit really did fail,
// so the duration is clamped and the report stands.
func age(when, now time.Time) string {
	d := now.Sub(when)
	if d <= 0 {
		return "just now"
	}
	return core.FormatElapsed(d) + " ago"
}

// shortSHA is the 7-character form every git UI shows and every git command
// accepts, so the SHA in the message can be pasted straight into a revert.
func shortSHA(sha string) string {
	if len(sha) <= 7 {
		return sha
	}
	return sha[:7]
}

// FromSource reads one snapshot from src and classifies it. The caller owns
// the deadline: pass a ctx with a timeout, because both real Source adapters
// swallow their own read errors and simply go quiet rather than failing, so a
// clock is the only thing that distinguishes "still fetching" from "never
// going to answer".
//
// The first snapshot wins and the rest are dropped. Sources poll — refsource
// re-emits whenever the remote moves — and a push must not wait out a poll
// interval to be told something it has already been told.
//
// A source that answers nothing before its channel closes is Undetermined,
// never Red. The push is what the user is trying to do; the check is a
// courtesy, and a courtesy that fails closed is an obstacle.
func FromSource(ctx context.Context, src core.Source, branch string, now time.Time) Result {
	for snap := range src.Watch(ctx) {
		return Decide(snap, branch, now)
	}
	return Undetermined.Result(fmt.Sprintf("no answer about %s before giving up", branch))
}

// TrunkReader reads one up-to-date snapshot of the trunk and reports why it
// could not.
//
// It is what check wants and what core.Source deliberately is not. Source.Watch
// swallows its read and fetch errors so a watcher that loses the network keeps
// showing the last state it knew instead of blanking the view — the right call
// for a dashboard, and the wrong one for a push gate, where it means answering
// from stale refs and calling it green.
type TrunkReader interface {
	Once() (core.Snapshot, error)
}

// Read classifies one snapshot from r. A read that fails, or that has not
// answered by the time ctx is done, is Undetermined — never Red.
//
// The deadline is honoured rather than waited out because the reader is doing
// network I/O on the critical path of somebody's push. git runs with
// GIT_TERMINAL_PROMPT=0 so it cannot sit on a credential prompt, but a dead
// connection still hangs until some timeout of its own, and the timeout that
// matters is the caller's. The read is abandoned, not cancelled: it holds no
// lock and writes nothing but remote-tracking refs, and the process is about
// to exit anyway.
func Read(ctx context.Context, r TrunkReader, branch string, now time.Time) Result {
	type answer struct {
		snap core.Snapshot
		err  error
	}
	done := make(chan answer, 1)
	go func() {
		snap, err := r.Once()
		done <- answer{snap: snap, err: err}
	}()

	select {
	case a := <-done:
		if a.err != nil {
			return Undetermined.Result(a.err.Error())
		}
		return Decide(a.snap, branch, now)
	case <-ctx.Done():
		return Undetermined.Result(fmt.Sprintf("no answer about %s before giving up", branch))
	}
}
