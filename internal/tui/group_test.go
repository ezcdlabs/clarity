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

// Commits with deploy:started belong in Deployed (not CIPassed) — Deployed
// includes "currently being deployed".
func TestGroupCommits_DeployStartedMakesItDeployed(t *testing.T) {
	commits := []watcher.CommitView{
		cv("b", ev("ci", "passed", 200), ev("deploy", "started", 280)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := tui.GroupCommits(commits)
	if totalDeployed(got) != 2 {
		t.Errorf("expected both b and a in Deployed, got %d", totalDeployed(got))
	}
	if len(got.CIPassed) != 0 {
		t.Errorf("expected CIPassed empty (b's deploy:started moves it into Deployed), got %+v", got.CIPassed)
	}
}

// --- DeployBatch shape -------------------------------------------------------

// A passed deploy and a started deploy are SEPARATE batches.
func TestGroupCommits_DeployBatches_DistinctAttempts(t *testing.T) {
	commits := []watcher.CommitView{
		cv("c", ev("ci", "passed", 300), ev("deploy", "started", 350)), // started
		cv("b", ev("ci", "passed", 200)),                                // belongs to c's batch
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),   // passed (older)
	}
	got := tui.GroupCommits(commits)
	if len(got.Deployed) != 2 {
		t.Fatalf("expected 2 batches (one started, one passed), got %d", len(got.Deployed))
	}
	if got.Deployed[0].Status != "started" {
		t.Errorf("expected newest batch status=started, got %q", got.Deployed[0].Status)
	}
	if len(got.Deployed[0].Commits) != 2 || got.Deployed[0].Commits[0].SHA != "c" || got.Deployed[0].Commits[1].SHA != "b" {
		t.Errorf("expected newest batch [c, b], got %+v", commitSHAs(got.Deployed[0].Commits))
	}
	if got.Deployed[1].Status != "passed" {
		t.Errorf("expected older batch status=passed, got %q", got.Deployed[1].Status)
	}
	if len(got.Deployed[1].Commits) != 1 || got.Deployed[1].Commits[0].SHA != "a" {
		t.Errorf("expected older batch [a], got %+v", commitSHAs(got.Deployed[1].Commits))
	}
}

// A failed deploy with NO newer batch stands alone as its own group.
func TestGroupCommits_FailedDeploy_NoNewerAttempt_StandsAlone(t *testing.T) {
	commits := []watcher.CommitView{
		cv("b", ev("ci", "passed", 200), ev("deploy", "failed", 250)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := tui.GroupCommits(commits)
	if len(got.Deployed) != 2 {
		t.Fatalf("expected 2 batches (one failed standalone, one passed), got %d", len(got.Deployed))
	}
	if got.Deployed[0].Status != "failed" {
		t.Errorf("expected newest batch status=failed, got %q", got.Deployed[0].Status)
	}
}

// A failed deploy followed by a NEWER deploy (started OR passed) gets MERGED
// into the newer batch — its commits are absorbed and no separate "failed"
// subgroup is rendered.
func TestGroupCommits_FailedDeploy_NewerStarted_Merged(t *testing.T) {
	commits := []watcher.CommitView{
		cv("c", ev("ci", "passed", 300), ev("deploy", "started", 350)),
		cv("b", ev("ci", "passed", 200), ev("deploy", "failed", 220)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := tui.GroupCommits(commits)
	if len(got.Deployed) != 2 {
		t.Fatalf("expected 2 batches (started absorbing failed, then passed), got %d", len(got.Deployed))
	}
	if got.Deployed[0].Status != "started" {
		t.Errorf("expected newest batch status=started, got %q", got.Deployed[0].Status)
	}
	if len(got.Deployed[0].Commits) != 2 || got.Deployed[0].Commits[0].SHA != "c" || got.Deployed[0].Commits[1].SHA != "b" {
		t.Errorf("expected merged batch [c, b] (b absorbed), got %+v", commitSHAs(got.Deployed[0].Commits))
	}
}

// Multiple consecutive failed deploys with no newer attempt stay separate.
// (The merge rule only kicks in when a newer non-failed attempt exists.)
func TestGroupCommits_TwoFailedDeploys_NoNewer_StaySeparate(t *testing.T) {
	commits := []watcher.CommitView{
		cv("b", ev("ci", "passed", 200), ev("deploy", "failed", 250)),
		cv("a", ev("ci", "passed", 100), ev("deploy", "failed", 150)),
	}
	got := tui.GroupCommits(commits)
	if len(got.Deployed) != 2 {
		t.Fatalf("expected 2 separate failed batches, got %d", len(got.Deployed))
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
