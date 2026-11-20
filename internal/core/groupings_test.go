package core_test

import (
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
)

func cv(sha string, events ...clarityrefs.Event) core.CommitView {
	return core.CommitView{SHA: sha, Author: sha, Subject: sha, Events: events}
}

// totalDeployed sums all commits across Deployed batches.
func totalDeployed(g core.Groupings) int {
	n := 0
	for _, b := range g.Deployed {
		n += len(b.Commits)
	}
	return n
}

// totalInFlight sums all commits across in-flight batches.
func totalInFlight(g core.Groupings) int {
	n := 0
	for _, b := range g.InFlight {
		n += len(b.Commits)
	}
	return n
}

func TestGroupCommits_Empty(t *testing.T) {
	got := core.GroupCommits(nil)
	if len(got.Head) != 0 || len(got.CIPassed) != 0 || len(got.Deployed) != 0 {
		t.Errorf("expected all groups empty, got %+v", got)
	}
}

// When no commit has ever built green, every commit sits in Head.
func TestGroupCommits_NoBuildLine_AllInHead(t *testing.T) {
	commits := []core.CommitView{
		cv("c"),
		cv("b", ev("ci", "started", 100)),
		cv("a", ev("ci", "failed", 50)),
	}
	got := core.GroupCommits(commits)
	if len(got.Head) != 3 {
		t.Fatalf("expected all 3 commits in Head, got %d", len(got.Head))
	}
	if len(got.CIPassed) != 0 || len(got.Deployed) != 0 {
		t.Errorf("expected CIPassed/Deployed empty, got %+v", got)
	}
}

// Newest commit has built green; older has been deployed. Built commits
// without any deploy event sit in CI Passed.
func TestGroupCommits_BuildPassedAboveDeployLine(t *testing.T) {
	commits := []core.CommitView{
		cv("c", ev("ci", "passed", 300)),
		cv("b", ev("ci", "passed", 200), ev("deploy", "passed", 250)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := core.GroupCommits(commits)
	if len(got.Head) != 0 {
		t.Errorf("expected no Head commits, got %d", len(got.Head))
	}
	if len(got.CIPassed) != 1 || got.CIPassed[0].SHA != "c" {
		t.Errorf("expected only 'c' in CIPassed, got %+v", got.CIPassed)
	}
	if totalDeployed(got) != 2 {
		t.Errorf("expected 2 commits across Deployed batches, got %d", totalDeployed(got))
	}
}

// Commits with deploy:started belong in InFlight (bottom of CI Passed) —
// Deployed is only for completed (deploy:passed) batches.
func TestGroupCommits_DeployStarted_GoesToInFlight(t *testing.T) {
	commits := []core.CommitView{
		cv("b", ev("ci", "passed", 200), ev("deploy", "started", 280)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := core.GroupCommits(commits)
	if totalDeployed(got) != 1 || got.Deployed[0].Commits[0].SHA != "a" {
		t.Errorf("expected only 'a' in Deployed, got %+v", got.Deployed)
	}
	if totalInFlight(got) != 1 || got.InFlight[0].Commits[0].SHA != "b" {
		t.Errorf("expected only 'b' in InFlight, got %+v", got.InFlight)
	}
	if got.InFlight[0].Status != "started" {
		t.Errorf("expected in-flight batch status=started, got %q", got.InFlight[0].Status)
	}
	if len(got.CIPassed) != 0 {
		t.Errorf("expected CIPassed empty (b is in an in-flight batch), got %+v", got.CIPassed)
	}
}

// Idle CI-passed commits (no deploy event) at the top of the section
// coexist with an in-flight batch at the bottom.
func TestGroupCommits_IdleAboveInFlight(t *testing.T) {
	commits := []core.CommitView{
		cv("d", ev("ci", "passed", 400)),                               // idle, newer
		cv("c", ev("ci", "passed", 300)),                               // idle, newer
		cv("b", ev("ci", "passed", 200), ev("deploy", "started", 280)), // in-flight anchor
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),  // already deployed
	}
	got := core.GroupCommits(commits)
	if len(got.CIPassed) != 2 || got.CIPassed[0].SHA != "d" || got.CIPassed[1].SHA != "c" {
		t.Errorf("expected idle [d, c] in CIPassed, got %+v", commitSHAs(got.CIPassed))
	}
	if totalInFlight(got) != 1 || got.InFlight[0].Commits[0].SHA != "b" {
		t.Errorf("expected 'b' in InFlight, got %+v", got.InFlight)
	}
	if totalDeployed(got) != 1 || got.Deployed[0].Commits[0].SHA != "a" {
		t.Errorf("expected 'a' in Deployed, got %+v", got.Deployed)
	}
}

// --- DeployBatch shape -------------------------------------------------------

// A passed deploy and a started deploy go to DIFFERENT sections now —
// started is in-flight (bottom of CI Passed), passed is in Deployed.
func TestGroupCommits_StartedAndPassed_DifferentSections(t *testing.T) {
	commits := []core.CommitView{
		cv("c", ev("ci", "passed", 300), ev("deploy", "started", 350)), // started → InFlight
		cv("b", ev("ci", "passed", 200)),                               // belongs to c's in-flight batch
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),  // passed → Deployed
	}
	got := core.GroupCommits(commits)
	if len(got.InFlight) != 1 || got.InFlight[0].Status != "started" {
		t.Fatalf("expected one started batch in InFlight, got %+v", got.InFlight)
	}
	if len(got.InFlight[0].Commits) != 2 ||
		got.InFlight[0].Commits[0].SHA != "c" || got.InFlight[0].Commits[1].SHA != "b" {
		t.Errorf("expected in-flight batch [c, b], got %+v", commitSHAs(got.InFlight[0].Commits))
	}
	if len(got.Deployed) != 1 || got.Deployed[0].Status != "passed" ||
		got.Deployed[0].Commits[0].SHA != "a" {
		t.Errorf("expected one passed batch [a] in Deployed, got %+v", got.Deployed)
	}
}

// A failed deploy with NO newer batch stands alone as its own InFlight group.
func TestGroupCommits_FailedDeploy_NoNewerAttempt_StandsAloneInFlight(t *testing.T) {
	commits := []core.CommitView{
		cv("b", ev("ci", "passed", 200), ev("deploy", "failed", 250)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := core.GroupCommits(commits)
	if len(got.InFlight) != 1 || got.InFlight[0].Status != "failed" {
		t.Fatalf("expected a standalone failed batch in InFlight, got %+v", got.InFlight)
	}
	if len(got.Deployed) != 1 || got.Deployed[0].Status != "passed" {
		t.Fatalf("expected a passed batch in Deployed, got %+v", got.Deployed)
	}
}

// A failed deploy followed by a NEWER non-failed deploy gets MERGED into the
// newer batch — its commits are absorbed.
func TestGroupCommits_FailedDeploy_NewerStarted_MergedIntoInFlight(t *testing.T) {
	commits := []core.CommitView{
		cv("c", ev("ci", "passed", 300), ev("deploy", "started", 350)),
		cv("b", ev("ci", "passed", 200), ev("deploy", "failed", 220)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := core.GroupCommits(commits)
	if len(got.InFlight) != 1 || got.InFlight[0].Status != "started" {
		t.Fatalf("expected one merged started batch in InFlight, got %+v", got.InFlight)
	}
	if len(got.InFlight[0].Commits) != 2 ||
		got.InFlight[0].Commits[0].SHA != "c" || got.InFlight[0].Commits[1].SHA != "b" {
		t.Errorf("expected merged in-flight [c, b], got %+v", commitSHAs(got.InFlight[0].Commits))
	}
	if len(got.Deployed) != 1 || got.Deployed[0].Status != "passed" {
		t.Errorf("expected one passed batch in Deployed, got %+v", got.Deployed)
	}
}

// Multiple consecutive failed deploys with no newer attempt stay separate in
// the InFlight section.
func TestGroupCommits_TwoFailedDeploys_NoNewer_StaySeparateInFlight(t *testing.T) {
	commits := []core.CommitView{
		cv("b", ev("ci", "passed", 200), ev("deploy", "failed", 250)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "failed", 150)),
	}
	got := core.GroupCommits(commits)
	if len(got.InFlight) != 2 {
		t.Fatalf("expected 2 separate failed batches in InFlight, got %d", len(got.InFlight))
	}
	if len(got.Deployed) != 0 {
		t.Errorf("expected empty Deployed (no deploy:passed), got %+v", got.Deployed)
	}
}

// A deploy:skipped event is inert in grouping. The status is retired from
// the write path, but old refs still carry it, and it used to anchor an
// unlabeled in-flight batch with a ticking timer — a commit that reads as
// "waiting to ship" on the say-so of an event that ships nothing. A skipped
// commit is an ordinary idle commit.
func TestGroupCommits_SkippedDeploy_IsInert(t *testing.T) {
	commits := []core.CommitView{
		cv("b", ev("ci", "passed", 200), ev("deploy", "skipped", 250)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := core.GroupCommits(commits)
	if len(got.InFlight) != 0 {
		t.Fatalf("a skipped deploy anchored an in-flight batch: %+v", got.InFlight)
	}
	if len(got.CIPassed) != 1 || got.CIPassed[0].SHA != "b" {
		t.Errorf("expected b idle in CIPassed, got %+v", got.CIPassed)
	}
}

// Below the deploy line a skipped commit sweeps into the newer passed batch
// exactly like a commit with no deploy events at all — the newer deploy's
// tree really does contain it.
func TestGroupCommits_SkippedDeploy_SweptLikeABareCommit(t *testing.T) {
	commits := []core.CommitView{
		cv("c", ev("ci", "passed", 300), ev("deploy", "passed", 350)),
		cv("b", ev("ci", "passed", 200), ev("deploy", "skipped", 250)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := core.GroupCommits(commits)
	if len(got.Deployed) != 2 {
		t.Fatalf("expected 2 deployed batches, got %+v", got.Deployed)
	}
	if len(got.Deployed[0].Commits) != 2 || got.Deployed[0].Commits[1].SHA != "b" {
		t.Errorf("expected b swept into c's batch, got %+v", got.Deployed[0].Commits)
	}
}

// --- Candidacy under the lead time modes --------------------------------------

// TestGroupCommitsForFlow_CandidacyByMode pins how candidacy meets each lead
// time mode. Under `all` every commit is its own sample, so its own record
// decides. Under `pipeline` and `reported` one event-bearing commit holds the
// sample for its whole run — itself plus the event-less commits swept below
// it, down to the previous event-bearing commit — so the sample survives if
// ANY commit the run carried is a candidate: the tip of a push can be
// unaffected while an older commit in the same push holds the change this
// flow ships, and that run still delivered it. Silence means candidate, so a
// swept commit with no record keeps its run counting.
func TestGroupCommitsForFlow_CandidacyByMode(t *testing.T) {
	x := core.Flow{Name: "x", Targets: []string{"x"}}
	// Per-commit spec: withEvents gives the commit a ci started/passed pair
	// (it is the pushed tip CI ran on); scope is "aff", "unaff" or "" (no
	// record — silence).
	type spec struct {
		withEvents bool
		scope      string
	}
	build := func(specs []spec) []core.CommitView {
		commits := make([]core.CommitView, len(specs))
		for i, s := range specs {
			c := cv(string(rune('a' + i)))
			c.Time = time.Unix(int64(50+i), 0)
			if s.withEvents {
				c.Events = []clarityrefs.Event{ev("ci", "started", 100), ev("ci", "passed", 200)}
			}
			switch s.scope {
			case "aff":
				c.Scope = []clarityrefs.Scope{{Target: "x", Affected: true, Time: time.Unix(0, 10)}}
			case "unaff":
				c.Scope = []clarityrefs.Scope{{Target: "x", Affected: false, Time: time.Unix(0, 10)}}
			}
			commits[i] = c
		}
		return commits
	}

	cases := []struct {
		name    string
		mode    core.LeadTimeMode
		commits []spec
		wantOK  []bool // per index: does LeadTime return a measurement?
	}{
		{
			name: "pipeline: swept affected commit keeps the run's sample",
			mode: core.LeadPipeline,
			commits: []spec{
				{withEvents: true, scope: "unaff"}, // the tip CI ran on
				{scope: "unaff"},
				{scope: "aff"}, // the change this flow ships
			},
			wantOK: []bool{true, false, false},
		},
		{
			name: "pipeline: a run of only unaffected commits is excluded",
			mode: core.LeadPipeline,
			commits: []spec{
				{withEvents: true, scope: "unaff"},
				{scope: "unaff"},
			},
			wantOK: []bool{false, false},
		},
		{
			name: "pipeline: a silent swept commit keeps the run counting",
			mode: core.LeadPipeline,
			commits: []spec{
				{withEvents: true, scope: "unaff"},
				{scope: ""},
			},
			wantOK: []bool{true, false},
		},
		{
			name: "pipeline: spans end at the previous event-bearing commit",
			mode: core.LeadPipeline,
			commits: []spec{
				{withEvents: true, scope: "unaff"}, // run 1: this and the next
				{scope: "unaff"},
				{withEvents: true, scope: "unaff"}, // run 2: this and the next
				{scope: "aff"},
			},
			// Run 2 carried the affected commit; run 1 did not and must not
			// be rescued by it.
			wantOK: []bool{false, false, true, false},
		},
		{
			name: "all: every commit is its own sample, own record decides",
			mode: core.LeadAll,
			commits: []spec{
				{withEvents: true, scope: "unaff"},
				{scope: "unaff"},
				{scope: "aff"},
			},
			wantOK: []bool{false, false, true},
		},
		{
			name: "reported: same run semantics as pipeline",
			mode: core.LeadReported,
			commits: []spec{
				{withEvents: true, scope: "unaff"},
				{scope: "aff"},
			},
			wantOK: []bool{true, false},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := core.GroupCommitsForFlow(build(c.commits), c.mode, x)
			for i, want := range c.wantOK {
				if _, _, ok := g.LeadTime(i, time.Unix(300, 0)); ok != want {
					t.Errorf("commit %d: lead time ok = %v, want %v", i, ok, want)
				}
			}
		})
	}
}

// TestGroupCommitsForFlow_ExposesUnaffected: renderers must tell a commit
// with no measurement from one resolved unaffected — blank and the ∅ marker
// are different answers. The exposure is per-commit truth (its own record),
// not the per-run OR that lead-time survival uses: the marker marks the
// commit itself. Without a flow there is no target to be unaffected for, so
// flow-less groupings mark nothing.
func TestGroupCommitsForFlow_ExposesUnaffected(t *testing.T) {
	x := core.Flow{Name: "x", Targets: []string{"x"}}
	commits := []core.CommitView{
		{SHA: "b", Scope: []clarityrefs.Scope{{Target: "x", Affected: false, Time: time.Unix(0, 10)}}},
		{SHA: "a"},
	}

	g := core.GroupCommitsForFlow(commits, core.LeadPipeline, x)
	if !g.Unaffected(0) {
		t.Errorf("commit 0 is resolved unaffected and should say so")
	}
	if g.Unaffected(1) {
		t.Errorf("a silent commit is a candidate, not unaffected")
	}
	if g.Unaffected(99) || g.Unaffected(-1) {
		t.Errorf("out of range must be false, not a panic or a claim")
	}

	if flowless := core.GroupCommits(commits); flowless.Unaffected(0) {
		t.Errorf("a grouping derived without a flow has no target to be unaffected for")
	}
}

// --- Stale stage rule (ci only) ----------------------------------------------

func TestIsStaleStage(t *testing.T) {
	commits := []core.CommitView{
		cv("c", ev("ci", "passed", 300)),
		cv("b", ev("ci", "failed", 200)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),
	}
	g := core.GroupCommits(commits)

	cases := []struct {
		name  string
		index int
		stage string
		want  bool
	}{
		{"c is ci line — not stale", 0, "ci", false},
		{"b is older than ci line — stale", 1, "ci", true},
		{"a is older than ci line — stale", 2, "ci", true},
		{"a is the only deploy:passed — not stale", 2, "deploy", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := g.IsStaleStage(c.index, c.stage)
			if got != c.want {
				t.Errorf("IsStaleStage(%d, %q) = %v, want %v", c.index, c.stage, got, c.want)
			}
		})
	}
}

// --- LeadTime ----------------------------------------------------------------

func TestLeadTime_NotDeployed_LiveAgainstNow(t *testing.T) {
	commitTime := time.Unix(1000, 0)
	now := time.Unix(1090, 0)
	commits := []core.CommitView{
		// CI starts at the authoring time, so the pipeline-default lead is
		// the same 90s the elapsed clock shows.
		{SHA: "a", Time: commitTime, Events: []clarityrefs.Event{ev("ci", "started", 1000)}},
	}
	g := core.GroupCommits(commits)
	d, frozen, ok := g.LeadTime(0, now)
	if !ok {
		t.Fatal("expected lead time to be available")
	}
	if frozen {
		t.Error("expected live (not frozen) lead time for not-yet-deployed commit")
	}
	if d != 90*time.Second {
		t.Errorf("expected 90s elapsed, got %v", d)
	}
}

func TestLeadTime_DeployStarted_StillLive(t *testing.T) {
	// A commit whose deploy is in flight (started) hasn't reached prod —
	// its lead time should still tick.
	commitTime := time.Unix(1000, 0)
	now := time.Unix(1090, 0)
	commits := []core.CommitView{
		{SHA: "a", Time: commitTime, Events: []clarityrefs.Event{
			ev("ci", "passed", 50),
			{Stage: "deploy", Status: "started", Time: time.Unix(1080, 0)},
		}},
	}
	g := core.GroupCommits(commits)
	_, frozen, ok := g.LeadTime(0, now)
	if !ok {
		t.Fatal("expected lead time")
	}
	if frozen {
		t.Error("expected live lead time during deploy:started (not yet in prod)")
	}
}

func TestLeadTime_Deployed_FrozenAtDeployTime(t *testing.T) {
	commitTime := time.Unix(1000, 0)
	deployTime := time.Unix(1300, 0)
	now := time.Unix(9000, 0)
	commits := []core.CommitView{
		{SHA: "a", Time: commitTime, Events: []clarityrefs.Event{
			ev("ci", "started", 1000), // pipeline opens at authoring time
			{Stage: "deploy", Status: "passed", Time: deployTime},
		}},
	}
	g := core.GroupCommits(commits)
	d, frozen, ok := g.LeadTime(0, now)
	if !ok || !frozen {
		t.Fatal("expected frozen lead time")
	}
	if d != 5*time.Minute {
		t.Errorf("expected 5m, got %v", d)
	}
}

func TestLeadTime_FixForwardedCommit_FrozenAtNewerDeploy(t *testing.T) {
	// Explicitly LeadAll: inheriting a lead time with no events of your own
	// is that mode's defining behaviour. Under the pipeline default an
	// event-less commit contributes no lead time at all (covered by
	// TestLeadTimeModes_BatchedPush in leadtime_test.go).
	olderCommitTime := time.Unix(500, 0)
	newerDeployTime := time.Unix(2000, 0)
	commits := []core.CommitView{
		{SHA: "newer", Time: time.Unix(1500, 0), Events: []clarityrefs.Event{
			{Stage: "deploy", Status: "passed", Time: newerDeployTime},
		}},
		{SHA: "older", Time: olderCommitTime},
	}
	g := core.GroupCommitsMode(commits, core.LeadAll)
	d, frozen, ok := g.LeadTime(1, time.Unix(9999, 0))
	if !ok || !frozen {
		t.Fatal("expected frozen lead time")
	}
	want := newerDeployTime.Sub(olderCommitTime)
	if d != want {
		t.Errorf("expected %v, got %v", want, d)
	}
}

func TestLeadTime_OwnDeploy_NotOverriddenByNewerDeploy(t *testing.T) {
	olderCommitTime := time.Unix(500, 0)
	olderDeployTime := time.Unix(800, 0)
	newerDeployTime := time.Unix(2000, 0)
	commits := []core.CommitView{
		{SHA: "newer", Time: time.Unix(1500, 0), Events: []clarityrefs.Event{
			{Stage: "deploy", Status: "passed", Time: newerDeployTime},
		}},
		{SHA: "older", Time: olderCommitTime, Events: []clarityrefs.Event{
			ev("ci", "started", 500), // pipeline opens at authoring time
			{Stage: "deploy", Status: "passed", Time: olderDeployTime},
		}},
	}
	g := core.GroupCommits(commits)
	d, frozen, ok := g.LeadTime(1, time.Unix(9999, 0))
	if !ok || !frozen {
		t.Fatal("expected frozen lead time")
	}
	want := olderDeployTime.Sub(olderCommitTime)
	if d != want {
		t.Errorf("expected %v, got %v", want, d)
	}
}

func TestLeadTime_ZeroCommitTime_NotAvailable(t *testing.T) {
	commits := []core.CommitView{{SHA: "a"}}
	g := core.GroupCommits(commits)
	_, _, ok := g.LeadTime(0, time.Unix(100, 0))
	if ok {
		t.Error("expected ok=false when commit time is zero")
	}
}

// --- helpers -----------------------------------------------------------------

func commitSHAs(commits []core.CommitView) []string {
	out := make([]string, len(commits))
	for i, c := range commits {
		out[i] = c.SHA
	}
	return out
}

var _ = time.Time{}

// --- Deploy no-op ------------------------------------------------------------

// A deploy:noop event is a completed delivery that shipped nothing: the
// pipeline ran, compared the desired state against what is in production, and
// found them already equal — the revert-pair case, where two commits both
// affect a target but their cumulative diff is empty. Unlike the retired
// "skipped" it is a positive verdict, so it anchors a Deployed batch exactly
// as a passed deploy does: the commits it carries are in production
// (trivially — the bytes already match) and must leave CI Passed.
func TestGroupCommits_NoopDeploy_AnchorsDeployedBatch(t *testing.T) {
	commits := []core.CommitView{
		cv("c", ev("ci", "passed", 300), ev("deploy", "noop", 350)),
		cv("b", ev("ci", "passed", 200)), // swept into c's no-op batch
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := core.GroupCommits(commits)
	if len(got.CIPassed) != 0 || len(got.InFlight) != 0 {
		t.Fatalf("expected no idle/in-flight commits, got %+v / %+v", got.CIPassed, got.InFlight)
	}
	if len(got.Deployed) != 2 {
		t.Fatalf("expected a no-op batch above a passed batch, got %+v", got.Deployed)
	}
	if got.Deployed[0].Status != "noop" || len(got.Deployed[0].Commits) != 2 ||
		got.Deployed[0].Commits[1].SHA != "b" {
		t.Errorf("expected no-op batch [c, b], got %+v", got.Deployed[0])
	}
	if got.Deployed[1].Status != "passed" || got.Deployed[1].Commits[0].SHA != "a" {
		t.Errorf("expected passed batch [a], got %+v", got.Deployed[1])
	}
}

// A no-op completes delivery, so it stops the lead-time clock the way a
// passed deploy does: the moment the pipeline verified production already
// matched the desired state, there was nothing left to wait for.
func TestLeadTime_NoopDeploy_FrozenAtNoopTime(t *testing.T) {
	commitTime := time.Unix(1000, 0)
	noopTime := time.Unix(1300, 0)
	commits := []core.CommitView{
		{SHA: "a", Time: commitTime, Events: []clarityrefs.Event{
			ev("ci", "started", 1000),
			ev("deploy", "noop", 1300),
		}},
	}
	g := core.GroupCommits(commits)
	d, frozen, ok := g.LeadTime(0, time.Unix(9999, 0))
	if !ok || !frozen {
		t.Fatalf("expected a frozen lead time, got ok=%v frozen=%v", ok, frozen)
	}
	if want := noopTime.Sub(commitTime); d != want {
		t.Errorf("expected lead %v frozen at the no-op, got %v", want, d)
	}
}

// A failed deploy fix-forwards into a newer no-op batch the same way it does
// into a passed one: production converged on a tree containing the failed
// commits, so they are delivered.
func TestGroupCommits_FailedDeploy_NewerNoop_MergedIntoNoopBatch(t *testing.T) {
	commits := []core.CommitView{
		cv("c", ev("ci", "passed", 300), ev("deploy", "noop", 350)),
		cv("b", ev("ci", "passed", 200), ev("deploy", "failed", 220)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := core.GroupCommits(commits)
	if len(got.Deployed) != 2 {
		t.Fatalf("expected [noop, passed] batches, got %+v", got.Deployed)
	}
	if got.Deployed[0].Status != "noop" || len(got.Deployed[0].Commits) != 2 ||
		got.Deployed[0].Commits[1].SHA != "b" {
		t.Errorf("expected b fix-forwarded into the no-op batch, got %+v", got.Deployed[0])
	}
}

// The live marker is a state claim — "this is production" — not an event
// claim, so a no-op batch on top holds it: the pipeline just verified
// production equals that batch's tree, which is the freshest possible
// statement of the present state. Decided in core so the three renderers
// cannot disagree about which batch is the present.
func TestLiveBatchIndex_NoopOnTop_HoldsTheMarker(t *testing.T) {
	commits := []core.CommitView{
		cv("c", ev("ci", "passed", 300), ev("deploy", "noop", 350)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),
	}
	g := core.GroupCommits(commits)
	if got := g.LiveBatchIndex(); got != 0 {
		t.Errorf("LiveBatchIndex = %d, want 0 (the no-op batch is the present state)", got)
	}
}

func TestLiveBatchIndex_NothingDeployed_IsAbsent(t *testing.T) {
	g := core.GroupCommits([]core.CommitView{cv("a", ev("ci", "passed", 100))})
	if got := g.LiveBatchIndex(); got != -1 {
		t.Errorf("LiveBatchIndex = %d, want -1 for an empty Deployed section", got)
	}
}
