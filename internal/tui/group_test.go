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

func TestGroupCommits_Empty(t *testing.T) {
	got := tui.GroupCommits(nil)
	if len(got.NeedsCI) != 0 || len(got.NextDeploy) != 0 || len(got.Deployed) != 0 {
		t.Errorf("expected all groups empty, got %+v", got)
	}
}

// When no commit has ever built green, every commit sits in needs-CI.
func TestGroupCommits_NoBuildLine_AllNeedsCI(t *testing.T) {
	commits := []watcher.CommitView{
		cv("c"),
		cv("b", ev("build", "started", 100)),
		cv("a", ev("build", "failed", 50)),
	}
	got := tui.GroupCommits(commits)
	if len(got.NeedsCI) != 3 {
		t.Fatalf("expected all 3 commits in NeedsCI, got %d", len(got.NeedsCI))
	}
	if len(got.NextDeploy) != 0 || len(got.Deployed) != 0 {
		t.Errorf("expected NextDeploy/Deployed empty, got next=%d deployed=%d",
			len(got.NextDeploy), len(got.Deployed))
	}
}

// Newest commit has a passing build; older commit has a passing deploy.
// Newest sits in NextDeploy (above deploy line), older in Deployed.
func TestGroupCommits_BuildPassedAboveDeployLine(t *testing.T) {
	commits := []watcher.CommitView{
		cv("c", ev("build", "passed", 300)),
		cv("b", ev("build", "passed", 200), ev("deploy", "passed", 250)),
		cv("a", ev("build", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := tui.GroupCommits(commits)
	if len(got.NeedsCI) != 0 {
		t.Errorf("expected no NeedsCI commits, got %d", len(got.NeedsCI))
	}
	if len(got.NextDeploy) != 1 || got.NextDeploy[0].SHA != "c" {
		t.Errorf("expected only 'c' in NextDeploy, got %+v", got.NextDeploy)
	}
	if len(got.Deployed) != 2 || got.Deployed[0].SHA != "b" || got.Deployed[1].SHA != "a" {
		t.Errorf("expected [b, a] in Deployed, got %+v", got.Deployed)
	}
}

// Build is broken at HEAD: head is in NeedsCI, lower commits split by deploy line.
func TestGroupCommits_BrokenBuildAtHead(t *testing.T) {
	commits := []watcher.CommitView{
		cv("d", ev("build", "failed", 400)),
		cv("c", ev("build", "started", 300)),
		cv("b", ev("build", "passed", 200), ev("deploy", "passed", 250)),
		cv("a", ev("build", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := tui.GroupCommits(commits)
	if len(got.NeedsCI) != 2 || got.NeedsCI[0].SHA != "d" || got.NeedsCI[1].SHA != "c" {
		t.Errorf("expected [d, c] in NeedsCI, got %+v", got.NeedsCI)
	}
	if len(got.NextDeploy) != 0 {
		t.Errorf("expected no NextDeploy, got %+v", got.NextDeploy)
	}
	if len(got.Deployed) != 2 {
		t.Errorf("expected 2 Deployed, got %+v", got.Deployed)
	}
}

// Built but not deployed: a batch of commits sits in NextDeploy.
func TestGroupCommits_BuiltNotDeployed_BatchInNextDeploy(t *testing.T) {
	commits := []watcher.CommitView{
		cv("d", ev("build", "passed", 400)),
		cv("c", ev("build", "passed", 300)),
		cv("b", ev("build", "passed", 200), ev("deploy", "passed", 250)),
		cv("a", ev("build", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := tui.GroupCommits(commits)
	if len(got.NextDeploy) != 2 || got.NextDeploy[0].SHA != "d" || got.NextDeploy[1].SHA != "c" {
		t.Errorf("expected [d, c] in NextDeploy, got %+v", got.NextDeploy)
	}
	if len(got.Deployed) != 2 {
		t.Errorf("expected 2 Deployed, got %+v", got.Deployed)
	}
}

// A deploy is currently in flight.
func TestGroupCommits_Deploying_FlagsTheGroup(t *testing.T) {
	commits := []watcher.CommitView{
		cv("c", ev("build", "passed", 300)),
		cv("b", ev("build", "passed", 200), ev("deploy", "started", 280)),
		cv("a", ev("build", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := tui.GroupCommits(commits)
	if !got.Deploying {
		t.Error("expected Deploying=true when a NextDeploy commit has deploy:started without completion")
	}
	if len(got.NextDeploy) != 2 {
		t.Errorf("expected 2 commits in NextDeploy (c and b), got %d", len(got.NextDeploy))
	}
}

// A deploy started, then completed: the deploy line moves up; not deploying.
func TestGroupCommits_DeployStartedThenPassed_NotDeploying(t *testing.T) {
	commits := []watcher.CommitView{
		cv("b", ev("build", "passed", 200),
			ev("deploy", "started", 280),
			ev("deploy", "passed", 290)),
		cv("a", ev("build", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := tui.GroupCommits(commits)
	if got.Deploying {
		t.Error("expected Deploying=false once deploy:passed lands")
	}
}

// Deploy failed: deploy line does not advance.
func TestGroupCommits_DeployFailed_DeployLineDoesNotAdvance(t *testing.T) {
	commits := []watcher.CommitView{
		cv("c", ev("build", "passed", 300)),
		cv("b", ev("build", "passed", 200), ev("deploy", "failed", 280)),
		cv("a", ev("build", "passed", 100), ev("deploy", "passed", 150)),
	}
	got := tui.GroupCommits(commits)
	// b's failed deploy means deploy line stays at a.
	if len(got.Deployed) != 1 || got.Deployed[0].SHA != "a" {
		t.Errorf("expected only [a] in Deployed, got %+v", got.Deployed)
	}
	if len(got.NextDeploy) != 2 {
		t.Errorf("expected [c, b] in NextDeploy, got %+v", got.NextDeploy)
	}
}

// IsStaleStage: a stage's icon should dim when a newer commit succeeded on the same stage.
func TestIsStaleStage(t *testing.T) {
	commits := []watcher.CommitView{
		cv("c", ev("build", "passed", 300)),                                           // newest, build passed
		cv("b", ev("build", "failed", 200)),                                           // older, build failed; b's build icon should be stale
		cv("a", ev("build", "passed", 100), ev("deploy", "passed", 150)),              // older still, build passed; stale (build); deploy not stale (no newer deploy passed)
	}
	g := tui.GroupCommits(commits)

	cases := []struct {
		name  string
		index int
		stage string
		want  bool
	}{
		{"c is build line — not stale", 0, "build", false},
		{"b is older than build line — stale", 1, "build", true},
		{"a is older than build line — stale", 2, "build", true},
		{"a is the only deploy — not stale", 2, "deploy", false},
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

// A commit not yet deployed has a live (un-frozen) lead time relative to now.
func TestLeadTime_NotDeployed_LiveAgainstNow(t *testing.T) {
	commitTime := time.Unix(1000, 0)
	now := time.Unix(1090, 0) // 90s later
	commits := []watcher.CommitView{
		{SHA: "a", Time: commitTime, Events: []clarityrefs.Event{ev("build", "started", 50)}},
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

// A deployed commit's lead time is frozen at the time of the deploy that
// pushed it to production — not the commit's own time and not now.
func TestLeadTime_Deployed_FrozenAtDeployTime(t *testing.T) {
	commitTime := time.Unix(1000, 0)
	deployTime := time.Unix(1300, 0) // 5 minutes after commit
	now := time.Unix(9000, 0)        // long after — should NOT affect frozen value
	commits := []watcher.CommitView{
		{SHA: "a", Time: commitTime, Events: []clarityrefs.Event{
			ev("build", "passed", 100),
			{Stage: "deploy", Status: "passed", Time: deployTime},
		}},
	}
	g := tui.GroupCommits(commits)
	d, frozen, ok := g.LeadTime(0, commitTime, now)
	if !ok {
		t.Fatal("expected lead time to be available")
	}
	if !frozen {
		t.Error("expected frozen lead time for deployed commit")
	}
	if d != 5*time.Minute {
		t.Errorf("expected 5m frozen lead time, got %v", d)
	}
}

// An older commit gets fix-forwarded to production by a newer commit's deploy.
// Its lead time freezes at the *newer* commit's deploy time.
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
	if !ok {
		t.Fatal("expected lead time")
	}
	if !frozen {
		t.Error("expected frozen — older commit was fix-forwarded by newer deploy")
	}
	wantLead := newerDeployTime.Sub(olderCommitTime)
	if d != wantLead {
		t.Errorf("expected %v frozen lead time (newer deploy - older commit), got %v", wantLead, d)
	}
}

// A commit's own deploy time is its freeze point even if a NEWER commit
// later deploys at a later real-time. "As soon as" means the EARLIEST
// moment the commit reached production.
func TestLeadTime_OwnDeploy_NotOverriddenByNewerDeploy(t *testing.T) {
	olderCommitTime := time.Unix(500, 0)
	olderDeployTime := time.Unix(800, 0)  // older commit deployed first, at t=800
	newerDeployTime := time.Unix(2000, 0) // a later commit deployed at t=2000
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
		t.Errorf("expected lead time = own-deploy - commit (%v), got %v", want, d)
	}
}

// Zero commit time means no lead time — the row simply skips the timer.
func TestLeadTime_ZeroCommitTime_NotAvailable(t *testing.T) {
	commits := []watcher.CommitView{{SHA: "a"}}
	g := tui.GroupCommits(commits)
	_, _, ok := g.LeadTime(0, time.Time{}, time.Unix(100, 0))
	if ok {
		t.Error("expected ok=false when commit time is zero")
	}
}
