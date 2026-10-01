package present_test

import (
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
	"github.com/ezcdlabs/clarity/mobile/internal/present"
	v1 "github.com/ezcdlabs/clarity/proto/gen/go/clarityv1"
)

func at(h int) time.Time { return time.Date(2026, 1, 8, h, 0, 0, 0, time.UTC) }

// deployed builds a snapshot with one commit that shipped and one that has
// only passed CI, which between them cover every section the view has.
func deployed() core.Snapshot {
	return core.Snapshot{
		RepoName: "api",
		Commits: []core.CommitView{
			{SHA: "bbbbbbbbbbbb", Subject: "feat: newer", Author: "bob", Time: at(5),
				Events: []clarityrefs.Event{{Stage: "ci", Status: "passed", Time: at(6)}}},
			{SHA: "aaaaaaaaaaaa", Subject: "feat: older", Author: "alice", Time: at(1),
				Events: []clarityrefs.Event{
					{Stage: "ci", Status: "passed", Time: at(2)},
					{Stage: "deploy", Status: "passed", Time: at(3)},
				}},
		},
	}
}

func mapped(t *testing.T, snap core.Snapshot, now time.Time) *v1.View {
	t.Helper()
	return present.View(core.DeriveView(snap, core.DefaultLeadTimeMode, nil), now)
}

// TestView_CarriesTheDecisionsNotTheGeometry is the contract of this
// boundary: everything that decides what is true is resolved before it
// crosses, and nothing about how wide or which glyph does.
func TestView_CarriesTheDecisionsNotTheGeometry(t *testing.T) {
	out := mapped(t, deployed(), at(10))

	if out.RepoName != "api" {
		t.Errorf("repo name = %q", out.RepoName)
	}
	if len(out.Flows) != 1 {
		t.Fatalf("a repo with no targets should still have exactly one flow, got %d", len(out.Flows))
	}
	f := out.Flows[0]
	if len(f.Groups) == 0 {
		t.Fatal("no groups")
	}

	var deployedGroup *v1.Group
	for _, g := range f.Groups {
		if g.Kind == v1.GroupKind_GROUP_KIND_DEPLOYED {
			deployedGroup = g
		}
	}
	if deployedGroup == nil {
		t.Fatal("the shipped commit produced no deployed group")
	}
	if deployedGroup.Status != v1.Status_STATUS_PASSED {
		t.Errorf("deployed group status = %v", deployedGroup.Status)
	}
	if deployedGroup.DeployedAgo == "" || deployedGroup.DeployedUnixSeconds == 0 {
		t.Error("a deploy group must carry when it landed, formatted and raw")
	}

	c := deployedGroup.Commits[0]
	if c.ShortSha != "aaaaaaaa" {
		t.Errorf("short sha = %q, want 8 characters", c.ShortSha)
	}
	if c.Sha != "aaaaaaaaaaaa" {
		t.Errorf("the full sha must survive for anything that needs to address the commit")
	}
	// Lead time is two hours: authored at 01:00, deployed at 03:00.
	if !c.HasLeadTime {
		t.Fatal("a shipped commit has a lead time")
	}
	if c.LeadTimeSeconds != int64(2*time.Hour/time.Second) {
		t.Errorf("lead time = %ds, want %ds", c.LeadTimeSeconds, int64(2*time.Hour/time.Second))
	}
	if c.LeadTime == "" {
		t.Error("the preformatted lead time is what a client displays; it must not be empty")
	}
	if c.LeadTimeLive {
		t.Error("a commit whose deploy has landed has a stopped clock")
	}
}

// TestView_LiveLeadTimeIsMarkedLive covers a commit that has landed but not
// shipped. Its clock is still running, which the TUI shows as a ticking grey
// timer — so the value is real and a client needs to know it will change.
func TestView_LiveLeadTimeIsMarkedLive(t *testing.T) {
	out := mapped(t, deployed(), at(10))

	var undeployed *v1.Commit
	for _, g := range out.Flows[0].Groups {
		if g.Kind == v1.GroupKind_GROUP_KIND_DEPLOYED {
			continue
		}
		for _, c := range g.Commits {
			if c.Sha == "bbbbbbbbbbbb" {
				undeployed = c
			}
		}
	}
	if undeployed == nil {
		t.Fatal("expected the un-shipped commit above the deploy line")
	}
	if !undeployed.HasLeadTime {
		t.Fatal("a commit waiting to ship has a running lead time, not none")
	}
	if !undeployed.LeadTimeLive {
		t.Error("the clock has not stopped; a client that renders this as final " +
			"shows a number that silently goes stale")
	}
	if undeployed.LeadTime == "" {
		t.Error("a running lead time still has a display value")
	}
}

// TestView_AbsentLeadTimeIsNotZero pins the distinction a bare number cannot
// carry. Under the reported mode a commit with no pipeline events contributes
// nothing, and "no lead time" must not arrive as a lead time of zero.
func TestView_AbsentLeadTimeIsNotZero(t *testing.T) {
	snap := core.Snapshot{RepoName: "api", Commits: []core.CommitView{
		{SHA: "cccccccccccc", Subject: "feat: swept along", Author: "carol", Time: at(1)},
		{SHA: "aaaaaaaaaaaa", Subject: "feat: reported", Author: "alice", Time: at(1),
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: at(2)},
				{Stage: "deploy", Status: "passed", Time: at(3)},
			}},
	}}
	view := core.DeriveView(snap, core.LeadReported, nil)
	out := present.View(view, at(10))

	for _, g := range out.Flows[0].Groups {
		for _, c := range g.Commits {
			if c.Sha != "cccccccccccc" {
				continue
			}
			if c.HasLeadTime {
				t.Errorf("a commit the mode excludes reports a lead time of %q", c.LeadTime)
			}
			if c.LeadTimeSeconds != 0 {
				t.Errorf("lead time seconds = %d, want 0", c.LeadTimeSeconds)
			}
			return
		}
	}
	t.Fatal("the excluded commit is missing from every group")
}

// TestView_StatusVocabularyIsSemantic verifies the enum carries meaning
// rather than the terminal's glyphs. ✓ and ✗ were chosen for a greyscale-safe
// palette and mean nothing on a phone.
func TestView_StatusVocabularyIsSemantic(t *testing.T) {
	snap := deployed()
	snap.Commits[0].Events = []clarityrefs.Event{{Stage: "ci", Status: "failed", Time: at(6)}}
	out := mapped(t, snap, at(10))

	var found bool
	for _, g := range out.Flows[0].Groups {
		for _, c := range g.Commits {
			if c.Sha == "bbbbbbbbbbbb" {
				found = true
				if c.Ci != v1.Status_STATUS_FAILED {
					t.Errorf("ci status = %v, want FAILED", c.Ci)
				}
			}
		}
	}
	if !found {
		t.Fatal("the failing commit is missing from every group")
	}
}

// TestView_UnknownStatusReadsAsUnreported covers a client older than the
// event that reached it. An unrecognised status must not render as broken.
func TestView_UnknownStatusReadsAsUnreported(t *testing.T) {
	snap := deployed()
	snap.Commits[0].Events = []clarityrefs.Event{{Stage: "ci", Status: "quarantined", Time: at(6)}}
	out := mapped(t, snap, at(10))

	for _, g := range out.Flows[0].Groups {
		for _, c := range g.Commits {
			if c.Sha == "bbbbbbbbbbbb" && c.Ci == v1.Status_STATUS_FAILED {
				t.Error("an unrecognised status rendered as a failure")
			}
		}
	}
}

// TestView_TargetsBecomeFlows covers the multi-target case the tab bar is for.
func TestView_TargetsBecomeFlows(t *testing.T) {
	snap := core.Snapshot{RepoName: "api", Commits: []core.CommitView{
		{SHA: "a", Subject: "x", Time: at(1), Events: []clarityrefs.Event{
			{Stage: "deploy", Status: "passed", Time: at(2), Target: "web"}}},
		{SHA: "b", Subject: "y", Time: at(1), Events: []clarityrefs.Event{
			{Stage: "deploy", Status: "failed", Time: at(2), Target: "ios"}}},
	}}
	out := mapped(t, snap, at(10))

	names := map[string]v1.Status{}
	for _, f := range out.Flows {
		names[f.Name] = f.Deploy
	}
	if len(names) < 2 {
		t.Fatalf("expected a flow per target, got %v", names)
	}
	if names["ios"] != v1.Status_STATUS_FAILED {
		t.Errorf("ios deploy status = %v, want FAILED", names["ios"])
	}
	if names["web"] != v1.Status_STATUS_PASSED {
		t.Errorf("web deploy status = %v, want PASSED", names["web"])
	}
}

// TestView_TruncationCrosses verifies a partial window says so. An aggregate
// built from one and read as complete is wrong in a way no client can see.
func TestView_TruncationCrosses(t *testing.T) {
	snap := deployed()
	snap.Truncated = true
	snap.Limit = 50
	out := mapped(t, snap, at(10))

	if !out.Truncated || out.Limit != 50 {
		t.Errorf("truncated=%v limit=%d, want true/50", out.Truncated, out.Limit)
	}
}
