package tui_test

import (
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/tui"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

func cv(sha string, events ...clarityrefs.Event) watcher.CommitView {
	return watcher.CommitView{SHA: sha, Author: sha, Subject: sha, Events: events}
}

// totalDeployed sums all commits across Deployed batches.
func totalDeployed(g tui.Groupings) int {
	n := 0
	for _, b := range g.Deployed {
		n += len(b.Commits)
	}
	return n
}

// totalInFlight sums all commits across in-flight batches.
func totalInFlight(g tui.Groupings) int {
	n := 0
	for _, b := range g.InFlight {
		n += len(b.Commits)
	}
	return n
}

func TestGroupCommits_Empty(t *testing.T) {
	got := tui.GroupCommits(nil)
	if len(got.Head) != 0 || len(got.CIPassed) != 0 || len(got.Deployed) != 0 {
		t.Errorf("expected all groups empty, got %+v", got)
	}
}

// When no commit has ever built green, every commit sits in Head.
func TestGroupCommits_NoBuildLine_AllInHead(t *testing.T) {
	commits := []watcher.CommitView{
		cv("c"),
		cv("b", ev("ci", "started", 100)),
		cv("a", ev("ci", "failed", 50)),
	}
	got := tui.GroupCommits(commits)
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
	commits := []watcher.CommitView{
		cv("c", ev("ci", "passed", 300)),
		cv("b", ev("ci", "passed", 200), ev("deploy", "passed", 250)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := tui.GroupCommits(commits)
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
	commits := []watcher.CommitView{
		cv("b", ev("ci", "passed", 200), ev("deploy", "started", 280)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := tui.GroupCommits(commits)
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
	commits := []watcher.CommitView{
		cv("d", ev("ci", "passed", 400)),                                // idle, newer
		cv("c", ev("ci", "passed", 300)),                                // idle, newer
		cv("b", ev("ci", "passed", 200), ev("deploy", "started", 280)),  // in-flight anchor
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),   // already deployed
	}
	got := tui.GroupCommits(commits)
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
	commits := []watcher.CommitView{
		cv("c", ev("ci", "passed", 300), ev("deploy", "started", 350)), // started → InFlight
		cv("b", ev("ci", "passed", 200)),                                // belongs to c's in-flight batch
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),   // passed → Deployed
	}
	got := tui.GroupCommits(commits)
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
	commits := []watcher.CommitView{
		cv("b", ev("ci", "passed", 200), ev("deploy", "failed", 250)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := tui.GroupCommits(commits)
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
	commits := []watcher.CommitView{
		cv("c", ev("ci", "passed", 300), ev("deploy", "started", 350)),
		cv("b", ev("ci", "passed", 200), ev("deploy", "failed", 220)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := tui.GroupCommits(commits)
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
	commits := []watcher.CommitView{
		cv("b", ev("ci", "passed", 200), ev("deploy", "failed", 250)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "failed", 150)),
	}
	got := tui.GroupCommits(commits)
	if len(got.InFlight) != 2 {
		t.Fatalf("expected 2 separate failed batches in InFlight, got %d", len(got.InFlight))
	}
	if len(got.Deployed) != 0 {
		t.Errorf("expected empty Deployed (no deploy:passed), got %+v", got.Deployed)
	}
}

// --- Stale stage rule (ci only) ----------------------------------------------

func TestIsStaleStage(t *testing.T) {
	commits := []watcher.CommitView{
		cv("c", ev("ci", "passed", 300)),
		cv("b", ev("ci", "failed", 200)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),
	}
	g := tui.GroupCommits(commits)

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
	commits := []watcher.CommitView{
		{SHA: "a", Time: commitTime, Events: []clarityrefs.Event{ev("ci", "started", 50)}},
	}
	g := tui.GroupCommits(commits)
	d, frozen, ok := g.LeadTime(0, commitTime, now)
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
	commits := []watcher.CommitView{
		{SHA: "a", Time: commitTime, Events: []clarityrefs.Event{
			ev("ci", "passed", 50),
			{Stage: "deploy", Status: "started", Time: time.Unix(1080, 0)},
		}},
	}
	g := tui.GroupCommits(commits)
	_, frozen, ok := g.LeadTime(0, commitTime, now)
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
	commits := []watcher.CommitView{
		{SHA: "a", Time: commitTime, Events: []clarityrefs.Event{
			ev("ci", "passed", 100),
			{Stage: "deploy", Status: "passed", Time: deployTime},
		}},
	}
	g := tui.GroupCommits(commits)
	d, frozen, ok := g.LeadTime(0, commitTime, now)
	if !ok || !frozen {
		t.Fatal("expected frozen lead time")
	}
	if d != 5*time.Minute {
		t.Errorf("expected 5m, got %v", d)
	}
}

func TestLeadTime_FixForwardedCommit_FrozenAtNewerDeploy(t *testing.T) {
	olderCommitTime := time.Unix(500, 0)
	newerDeployTime := time.Unix(2000, 0)
	commits := []watcher.CommitView{
		{SHA: "newer", Time: time.Unix(1500, 0), Events: []clarityrefs.Event{
			{Stage: "deploy", Status: "passed", Time: newerDeployTime},
		}},
		{SHA: "older", Time: olderCommitTime},
	}
	g := tui.GroupCommits(commits)
	d, frozen, ok := g.LeadTime(1, olderCommitTime, time.Unix(9999, 0))
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
	commits := []watcher.CommitView{
		{SHA: "newer", Time: time.Unix(1500, 0), Events: []clarityrefs.Event{
			{Stage: "deploy", Status: "passed", Time: newerDeployTime},
		}},
		{SHA: "older", Time: olderCommitTime, Events: []clarityrefs.Event{
			{Stage: "deploy", Status: "passed", Time: olderDeployTime},
		}},
	}
	g := tui.GroupCommits(commits)
	d, frozen, ok := g.LeadTime(1, olderCommitTime, time.Unix(9999, 0))
	if !ok || !frozen {
		t.Fatal("expected frozen lead time")
	}
	want := olderDeployTime.Sub(olderCommitTime)
	if d != want {
		t.Errorf("expected %v, got %v", want, d)
	}
}

func TestLeadTime_ZeroCommitTime_NotAvailable(t *testing.T) {
	commits := []watcher.CommitView{{SHA: "a"}}
	g := tui.GroupCommits(commits)
	_, _, ok := g.LeadTime(0, time.Time{}, time.Unix(100, 0))
	if ok {
		t.Error("expected ok=false when commit time is zero")
	}
}

// --- helpers -----------------------------------------------------------------

func commitSHAs(commits []watcher.CommitView) []string {
	out := make([]string, len(commits))
	for i, c := range commits {
		out[i] = c.SHA
	}
	return out
}

var _ = time.Time{}
