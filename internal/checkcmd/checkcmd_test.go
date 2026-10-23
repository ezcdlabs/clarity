package checkcmd_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/checkcmd"
	"github.com/ezcdlabs/clarity/internal/core"
)

func ev(stage, status string, ts int64) clarityrefs.Event {
	return clarityrefs.Event{Stage: stage, Status: status, Time: time.Unix(ts, 0)}
}

func snap(commits ...core.CommitView) core.Snapshot {
	return core.Snapshot{Commits: commits}
}

// now is the clock every case is judged against. Events are timestamped
// relative to it so "how long has it been red" is a readable number.
var now = time.Unix(10_000, 0)

// The classification table. Every row is a decision recorded in README.md's
// "What counts as red" — in particular the three things that are deliberately
// *not* red: an unfinished run, a trunk that never reported, and a failed
// deploy.
func TestDecide_TableDriven(t *testing.T) {
	cases := []struct {
		name string
		snap core.Snapshot
		want checkcmd.Outcome
	}{
		{
			name: "ci passed on the newest commit",
			snap: snap(core.CommitView{SHA: "aaa", Events: []clarityrefs.Event{
				ev("ci", "passed", 9_000),
			}}),
			want: checkcmd.Green,
		},
		{
			name: "ci failed on the newest commit",
			snap: snap(core.CommitView{SHA: "aaa", Events: []clarityrefs.Event{
				ev("ci", "failed", 9_000),
			}}),
			want: checkcmd.Red,
		},
		{
			name: "no events at all — a trunk that never reported is not red",
			snap: snap(core.CommitView{SHA: "aaa"}, core.CommitView{SHA: "bbb"}),
			want: checkcmd.Green,
		},
		{
			name: "ci still running — in-flight is not red",
			snap: snap(core.CommitView{SHA: "aaa", Events: []clarityrefs.Event{
				ev("ci", "started", 9_900),
			}}),
			want: checkcmd.Green,
		},
		{
			name: "ci running on top of a red commit — the red still counts",
			snap: snap(
				core.CommitView{SHA: "aaa", Events: []clarityrefs.Event{
					ev("ci", "started", 9_900),
				}},
				core.CommitView{SHA: "bbb", Events: []clarityrefs.Event{
					ev("ci", "failed", 9_000),
				}},
			),
			want: checkcmd.Red,
		},
		{
			name: "a newer commit went green — the older red is history",
			snap: snap(
				core.CommitView{SHA: "revert", Events: []clarityrefs.Event{
					ev("ci", "passed", 9_500),
				}},
				core.CommitView{SHA: "broken", Events: []clarityrefs.Event{
					ev("ci", "failed", 9_000),
				}},
			),
			want: checkcmd.Green,
		},
		{
			name: "ci passed, deploy failed — deploy is never checked",
			snap: snap(core.CommitView{SHA: "aaa", Events: []clarityrefs.Event{
				ev("ci", "passed", 9_000),
				ev("deploy", "failed", 9_100),
			}}),
			want: checkcmd.Green,
		},
		{
			name: "deploy failed and ci never reported — still not red",
			snap: snap(core.CommitView{SHA: "aaa", Events: []clarityrefs.Event{
				ev("deploy", "failed", 9_100),
			}}),
			want: checkcmd.Green,
		},
		{
			name: "ci re-run turned a green commit red",
			snap: snap(core.CommitView{SHA: "aaa", Events: []clarityrefs.Event{
				ev("ci", "passed", 9_000),
				ev("ci", "failed", 9_500),
			}}),
			want: checkcmd.Red,
		},
		{
			name: "ci retry turned a red commit green",
			snap: snap(core.CommitView{SHA: "aaa", Events: []clarityrefs.Event{
				ev("ci", "failed", 9_000),
				ev("ci", "passed", 9_500),
			}}),
			want: checkcmd.Green,
		},
		{
			name: "no commits — nothing was read, so nothing is known",
			snap: snap(),
			want: checkcmd.Undetermined,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkcmd.Decide(tc.snap, "main", now)
			if got.Outcome != tc.want {
				t.Errorf("outcome = %v, want %v\nreport:\n%s", got.Outcome, tc.want, got.Report)
			}
		})
	}
}

// Exit codes are the whole interface — `check && git push` sees nothing else.
// Red is 1; green and undetermined are both 0, because a check that cannot
// tell must not block a push.
func TestOutcome_ExitCodes(t *testing.T) {
	cases := []struct {
		outcome checkcmd.Outcome
		want    int
	}{
		{checkcmd.Green, 0},
		{checkcmd.Red, 1},
		{checkcmd.Undetermined, 0},
	}
	for _, tc := range cases {
		if got := tc.outcome.ExitCode(); got != tc.want {
			t.Errorf("%v.ExitCode() = %d, want %d", tc.outcome, got, tc.want)
		}
	}
}

func TestDecide_GreenSaysNothing(t *testing.T) {
	got := checkcmd.Decide(snap(core.CommitView{SHA: "aaa", Events: []clarityrefs.Event{
		ev("ci", "passed", 9_000),
	}}), "main", now)

	if got.Report != "" {
		t.Errorf("green printed a report, want silence:\n%s", got.Report)
	}
}

// The report is the feature: the exit code decides whether the push happens,
// and these four facts decide whether anyone learns anything.
func TestDecide_RedReportNamesTheCommit(t *testing.T) {
	red := snap(core.CommitView{
		SHA:     "a3f2c1d9e8b7a6f5d4c3b2a1908172635445f6e7",
		Subject: "fix(mobile): the registry survives two writers",
		Author:  "Steven Borrie",
		Events:  []clarityrefs.Event{ev("ci", "failed", 9_748)},
	})

	report := checkcmd.Decide(red, "main", now).Report

	for _, want := range []string{
		"a3f2c1d", // short sha, so it can be acted on
		"fix(mobile): the registry survives two writers", // subject, so it is recognisable
		"Steven Borrie", // author, so the fix has an owner
		"4m 12s",        // how long it has been red
		"main",          // which branch is broken
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report does not mention %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, red.Commits[0].SHA) {
		t.Errorf("report used the full 40-char SHA, want the short form:\n%s", report)
	}
}

// Whatever the message says to run is what gets run, so it must not say how to
// get past the check. The way past is `git push` on its own — documented in
// --help, where you have to go looking for it.
func TestDecide_RedReportAdvertisesNoBypass(t *testing.T) {
	red := snap(core.CommitView{SHA: "aaa", Author: "someone", Events: []clarityrefs.Event{
		ev("ci", "failed", 9_000),
	}})

	report := strings.ToLower(checkcmd.Decide(red, "main", now).Report)

	for _, forbidden := range []string{"no-verify", "--force", "-f ", "skip", "bypass", "override"} {
		if strings.Contains(report, forbidden) {
			t.Errorf("report advertises a bypass (%q):\n%s", forbidden, report)
		}
	}
}

// A CI machine whose clock runs ahead reports an event in the future. The age
// is then negative, and "-3m 00s ago" reads as a tool that is broken rather
// than a trunk that is. Nothing here justifies dropping the report — the
// commit really did fail — so only the duration is clamped.
func TestDecide_FutureEventReadsAsJustNow(t *testing.T) {
	red := snap(core.CommitView{SHA: "aaa", Author: "someone", Events: []clarityrefs.Event{
		ev("ci", "failed", 10_180),
	}})

	got := checkcmd.Decide(red, "main", now)

	if got.Outcome != checkcmd.Red {
		t.Fatalf("outcome = %v, want Red", got.Outcome)
	}
	if strings.Contains(got.Report, "-") && strings.Contains(got.Report, "ago") {
		t.Errorf("report carries a negative age:\n%s", got.Report)
	}
	if !strings.Contains(got.Report, "just now") {
		t.Errorf("report does not say %q:\n%s", "just now", got.Report)
	}
}

// Undetermined has to look different from green, because silence means green.
// It still exits 0: offline is not a red trunk.
func TestUndetermined_ExplainsItselfAndDoesNotBlock(t *testing.T) {
	got := checkcmd.Undetermined.Result("fetch origin: network is unreachable")

	if got.Outcome != checkcmd.Undetermined {
		t.Fatalf("outcome = %v, want Undetermined", got.Outcome)
	}
	if got.Outcome.ExitCode() != 0 {
		t.Errorf("undetermined blocked the push, exit code %d", got.Outcome.ExitCode())
	}
	if !strings.Contains(got.Report, "network is unreachable") {
		t.Errorf("report drops the reason:\n%s", got.Report)
	}
	if got.Report == "" {
		t.Error("undetermined said nothing, which reads as green")
	}
}

// --- FromSource --------------------------------------------------------------

// fakeSource emits the given snapshots and closes, standing in for git or the
// GitHub API. A source that emits nothing is the shape that matters most: both
// real adapters swallow their own read errors and simply go quiet, so "no
// answer" is a case FromSource has to handle rather than wait out.
type fakeSource struct{ snaps []core.Snapshot }

func (f *fakeSource) Watch(ctx context.Context) <-chan core.Snapshot {
	ch := make(chan core.Snapshot, len(f.snaps))
	for _, s := range f.snaps {
		ch <- s
	}
	close(ch)
	return ch
}

func TestFromSource_ClassifiesTheFirstSnapshot(t *testing.T) {
	red := snap(core.CommitView{SHA: "aaa", Author: "someone", Events: []clarityrefs.Event{
		ev("ci", "failed", 9_000),
	}})

	got := checkcmd.FromSource(context.Background(), &fakeSource{snaps: []core.Snapshot{red}}, "main", now)

	if got.Outcome != checkcmd.Red {
		t.Errorf("outcome = %v, want Red\nreport:\n%s", got.Outcome, got.Report)
	}
}

// A source that never answers must not block the push. The push is the thing
// the user is trying to do; the check is a courtesy.
func TestFromSource_SilentSourceIsUndetermined(t *testing.T) {
	got := checkcmd.FromSource(context.Background(), &fakeSource{}, "main", now)

	if got.Outcome != checkcmd.Undetermined {
		t.Errorf("outcome = %v, want Undetermined", got.Outcome)
	}
	if got.Outcome.ExitCode() != 0 {
		t.Errorf("a silent source blocked the push, exit code %d", got.Outcome.ExitCode())
	}
	if !strings.Contains(got.Report, "main") {
		t.Errorf("report does not say which branch went unanswered:\n%s", got.Report)
	}
}

// Sources poll: refsource re-emits every time the remote moves. check wants
// the first answer and nothing after it — a push should not wait out a poll
// interval to be told something it already knows.
func TestFromSource_IgnoresSnapshotsAfterTheFirst(t *testing.T) {
	first := snap(core.CommitView{SHA: "aaa", Events: []clarityrefs.Event{ev("ci", "passed", 9_000)}})
	later := snap(core.CommitView{SHA: "bbb", Author: "someone", Events: []clarityrefs.Event{
		ev("ci", "failed", 9_500),
	}})

	got := checkcmd.FromSource(context.Background(), &fakeSource{snaps: []core.Snapshot{first, later}}, "main", now)

	if got.Outcome != checkcmd.Green {
		t.Errorf("outcome = %v, want Green — the second snapshot was read", got.Outcome)
	}
}

// A cancelled context is the deadline in runCheck expiring. Same answer as a
// silent source: nobody said the trunk was red.
func TestFromSource_CancelledContextIsUndetermined(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got := checkcmd.FromSource(ctx, &blockingSource{}, "main", now)

	if got.Outcome != checkcmd.Undetermined {
		t.Errorf("outcome = %v, want Undetermined", got.Outcome)
	}
}

// blockingSource never emits and never closes its channel until ctx is done —
// the shape of a real adapter mid-fetch.
type blockingSource struct{}

func (b *blockingSource) Watch(ctx context.Context) <-chan core.Snapshot {
	ch := make(chan core.Snapshot)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch
}

// --- Read --------------------------------------------------------------------

// fakeReader is a TrunkReader whose answer — or failure, or silence — the test
// chooses.
type fakeReader struct {
	snap  core.Snapshot
	err   error
	block chan struct{} // when non-nil, Once waits on it before returning
}

func (f *fakeReader) Once() (core.Snapshot, error) {
	if f.block != nil {
		<-f.block
	}
	return f.snap, f.err
}

func TestRead_ClassifiesWhatTheReaderReturns(t *testing.T) {
	red := snap(core.CommitView{SHA: "aaa", Author: "someone", Events: []clarityrefs.Event{
		ev("ci", "failed", 9_000),
	}})

	got := checkcmd.Read(context.Background(), &fakeReader{snap: red}, "main", now)

	if got.Outcome != checkcmd.Red {
		t.Errorf("outcome = %v, want Red\nreport:\n%s", got.Outcome, got.Report)
	}
}

// The whole reason this port exists rather than core.Source: a fetch that
// failed must be distinguishable from a trunk that is green. Watch cannot tell
// us, so it would have answered from whatever stale refs the clone happened to
// have — silently, since silence is what green looks like.
func TestRead_FailureIsUndeterminedAndQuotesTheReason(t *testing.T) {
	got := checkcmd.Read(context.Background(),
		&fakeReader{err: errors.New("fetch branch: could not read from remote repository")},
		"main", now)

	if got.Outcome != checkcmd.Undetermined {
		t.Fatalf("outcome = %v, want Undetermined", got.Outcome)
	}
	if got.Outcome.ExitCode() != 0 {
		t.Errorf("a failed fetch blocked the push, exit code %d", got.Outcome.ExitCode())
	}
	if !strings.Contains(got.Report, "could not read from remote repository") {
		t.Errorf("report drops the reason:\n%s", got.Report)
	}
}

// A fetch that never returns must not hold the push open. git is run with
// GIT_TERMINAL_PROMPT=0 so it cannot sit on a credential prompt, but a dead
// connection still hangs until some timeout, and the one that matters is the
// caller's.
func TestRead_HonoursTheDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	blocked := &fakeReader{block: make(chan struct{})}
	t.Cleanup(func() { close(blocked.block) })
	cancel()

	got := checkcmd.Read(ctx, blocked, "main", now)

	if got.Outcome != checkcmd.Undetermined {
		t.Errorf("outcome = %v, want Undetermined", got.Outcome)
	}
	if !strings.Contains(got.Report, "main") {
		t.Errorf("report does not say which branch went unanswered:\n%s", got.Report)
	}
}

// --- Identity never enters into it -------------------------------------------

// Red is red for everyone. Decide takes no identity — not the local
// user.email, not the pushing clone — and these two calls differ only in who
// authored the break, so an outcome that varied with the author could not be
// expressed here at all. That is the point: the decision is deliberately
// outside this function's reach.
//
// Tempting as it is to wave through the person who broke it (they are the one
// who has to push the fix), it cannot be done correctly where it matters. The
// case the command exists for is several agents in worktrees of one clone:
// they share user.email *and* the remote-tracking reflog, so every identity
// check available answers "you broke it" to all of them, and the speed bump
// disappears for exactly the cohort it was built for. Blocking uniformly and
// letting the consumer judge is both simpler and more honest.
func TestDecide_DoesNotCareWhoBrokeIt(t *testing.T) {
	red := func(author string) core.Snapshot {
		return snap(core.CommitView{
			SHA:     "a3f2c1d9e8b7a6f5d4c3b2a19081726354",
			Subject: "feat: breaks ci",
			Author:  author,
			Events:  []clarityrefs.Event{ev("ci", "failed", 9_000)},
		})
	}

	mine := checkcmd.Decide(red("Steven Borrie"), "main", now)
	theirs := checkcmd.Decide(red("Someone Else"), "main", now)

	if mine.Outcome != checkcmd.Red {
		t.Errorf("outcome for my own break = %v, want Red", mine.Outcome)
	}
	if theirs.Outcome != checkcmd.Red {
		t.Errorf("outcome for someone else's break = %v, want Red", theirs.Outcome)
	}
	if mine.Outcome.ExitCode() != theirs.Outcome.ExitCode() {
		t.Errorf("exit code depends on the author: %d vs %d",
			mine.Outcome.ExitCode(), theirs.Outcome.ExitCode())
	}
}
