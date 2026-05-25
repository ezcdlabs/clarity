package tui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/adapters/tui"
	"github.com/ezcdlabs/clarity/internal/core"
)

func ev(stage, status string, ts int64) clarityrefs.Event {
	return clarityrefs.Event{Stage: stage, Status: status, Time: time.Unix(ts, 0)}
}

// --- RenderRow ---------------------------------------------------------------

func TestRenderRow_ShowsAuthorAndSubject(t *testing.T) {
	view := core.CommitView{
		SHA: "abc", Author: "alice", Subject: "refactor user model",
	}
	out := tui.RenderRow(view, 80)
	if !strings.Contains(out, "alice") {
		t.Errorf("expected author in row: %q", out)
	}
	if !strings.Contains(out, "refactor user model") {
		t.Errorf("expected subject in row: %q", out)
	}
}

func TestRenderRow_PassedShowsCheckmark(t *testing.T) {
	view := core.CommitView{
		SHA: "abc", Author: "alice", Subject: "x",
		Events: []clarityrefs.Event{ev("ci", "passed", 100)},
	}
	out := tui.RenderRow(view, 80)
	if !strings.Contains(out, "✓") {
		t.Errorf("expected ✓ for passed: %q", out)
	}
}

func TestRenderRow_FailedShowsCross(t *testing.T) {
	view := core.CommitView{
		SHA: "abc", Author: "eve", Subject: "x",
		Events: []clarityrefs.Event{ev("ci", "failed", 100)},
	}
	out := tui.RenderRow(view, 80)
	if !strings.Contains(out, "✗") {
		t.Errorf("expected ✗ for failed: %q", out)
	}
}

func TestRenderRow_RunningShowsSpinner(t *testing.T) {
	view := core.CommitView{
		SHA: "abc", Author: "dave", Subject: "x",
		Events: []clarityrefs.Event{ev("ci", "started", 100)},
	}
	out := tui.RenderRow(view, 80)
	hasSpinner := false
	for _, frame := range tui.SpinnerFrames {
		if strings.Contains(out, frame) {
			hasSpinner = true
			break
		}
	}
	if !hasSpinner {
		t.Errorf("expected a spinner frame for running ci: %q", out)
	}
}


// --- RenderSnapshot ----------------------------------------------------------

func TestRenderSnapshot_OneRowPerCommit(t *testing.T) {
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "1", Author: "alice", Subject: "first"},
			{SHA: "2", Author: "bob", Subject: "second"},
			{SHA: "3", Author: "carol", Subject: "third"},
		},
	}
	out := tui.RenderSnapshot(snap, 80, time.Time{}, 0)
	for _, want := range []string{"alice", "bob", "carol", "first", "second", "third"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
}

// --- section rendering -------------------------------------------------------

// All three section dividers are persistent — even when their section has no
// commits — so the layout reads as a structural frame rather than a list of
// only-currently-active groups.
func TestRenderSnapshot_AllThreeDividersAlwaysRender(t *testing.T) {
	// Only HEAD has content; CI Passed and Deployed are both empty.
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "1", Author: "alice", Subject: "wip"},
		},
	}
	out := tui.RenderSnapshot(snap, 80, time.Time{}, 0)
	for _, label := range []string{"HEAD", "CI Passed", "Deployed"} {
		if !strings.Contains(out, label) {
			t.Errorf("expected divider %q always present, got:\n%s", label, out)
		}
	}
}

// Even with zero commits at all, the structural frame stays in place.
func TestRenderSnapshot_EmptySnapshot_StillShowsDividers(t *testing.T) {
	out := tui.RenderSnapshot(core.Snapshot{}, 80, time.Time{}, 0)
	for _, label := range []string{"HEAD", "CI Passed", "Deployed"} {
		if !strings.Contains(out, label) {
			t.Errorf("expected divider %q in empty render, got:\n%s", label, out)
		}
	}
}

// A snapshot covering all three lifecycle stages should produce all three section headers.
func TestRenderSnapshot_AllThreeSections(t *testing.T) {
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "d", Author: "dave", Subject: "broken"},
			{SHA: "c", Author: "carol", Subject: "built", Events: []clarityrefs.Event{ev("ci", "passed", 200)}},
			{SHA: "b", Author: "bob", Subject: "shipped", Events: []clarityrefs.Event{
				ev("ci", "passed", 100), ev("deploy", "passed", 150),
			}},
		},
	}
	out := tui.RenderSnapshot(snap, 80, time.Time{}, 0)
	for _, h := range []string{"HEAD", "CI Passed", "Deployed"} {
		if !strings.Contains(out, h) {
			t.Errorf("expected section header %q in output:\n%s", h, out)
		}
	}
	for _, name := range []string{"dave", "carol", "bob"} {
		if !strings.Contains(out, name) {
			t.Errorf("expected commit author %q in output:\n%s", name, out)
		}
	}
}

// While a deploy is in flight the Deployed batch subheader says "deploying…".
func TestRenderSnapshot_DeployingSubheader(t *testing.T) {
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "b", Author: "bob", Subject: "shipping", Events: []clarityrefs.Event{
				ev("ci", "passed", 100), ev("deploy", "started", 200),
			}},
			{SHA: "a", Author: "alice", Subject: "live", Events: []clarityrefs.Event{
				ev("ci", "passed", 50), ev("deploy", "passed", 75),
			}},
		},
	}
	out := tui.RenderSnapshot(snap, 80, time.Time{}, 0)
	if !strings.Contains(strings.ToLower(out), "deploying") {
		t.Errorf("expected 'deploying' subheader inside Deployed, got:\n%s", out)
	}
}

// A passed deploy and a started deploy should produce two distinct subheaders.
// Week dividers in the Deployed section show ISO-week throughput summaries
// (W<year>-<week>, deploy count, average lead time). They appear once per
// week above that week's group of batches.
func TestRenderSnapshot_WeekDivider_AppearsAboveDeployedBatch(t *testing.T) {
	c := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC).Unix()
	d := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC).Unix()
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: "a", Author: "alice", Subject: "shipped",
			Time:   time.Unix(c, 0),
			Events: []clarityrefs.Event{ev("ci", "passed", c+60), ev("deploy", "passed", d)}},
	}}
	out := tui.RenderSnapshot(snap, 80, time.Unix(d+3600, 0), 0)
	if !strings.Contains(out, "W2026-02") {
		t.Errorf("expected W2026-02 week divider in output, got:\n%s", out)
	}
	if !strings.Contains(out, "1 deploy") {
		t.Errorf("expected '1 deploy' on divider, got:\n%s", out)
	}
}

// The topmost week's stats live on the same divider line as the "Deployed"
// section header — saves a row and keeps the eye on the section start.
// Older weeks below still get their own standalone dividers.
func TestRenderSnapshot_WeekDivider_MergedIntoDeployedHeaderForTopWeek(t *testing.T) {
	c1 := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC).Unix()
	d1 := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC).Unix() // week 2
	c2 := time.Date(2026, 1, 12, 9, 0, 0, 0, time.UTC).Unix()
	d2 := time.Date(2026, 1, 12, 10, 0, 0, 0, time.UTC).Unix() // week 3
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: "b", Author: "alice", Subject: "newer",
			Time:   time.Unix(c2, 0),
			Events: []clarityrefs.Event{ev("ci", "passed", c2+60), ev("deploy", "passed", d2)}},
		{SHA: "a", Author: "alice", Subject: "older",
			Time:   time.Unix(c1, 0),
			Events: []clarityrefs.Event{ev("ci", "passed", c1+60), ev("deploy", "passed", d1)}},
	}}
	out := tui.RenderSnapshot(snap, 120, time.Unix(d2+3600, 0), 0)

	// Find the line that contains "Deployed" — it must ALSO contain the
	// newest week's W-label so the two have been merged onto one row.
	var deployedLine string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Deployed") {
			deployedLine = line
			break
		}
	}
	if deployedLine == "" {
		t.Fatalf("no Deployed line found in output:\n%s", out)
	}
	if !strings.Contains(deployedLine, "W2026-03") {
		t.Errorf("expected newest week (W2026-03) merged into Deployed header line, got: %q", deployedLine)
	}

	// The OLDER week still gets its own standalone divider line.
	if !strings.Contains(out, "W2026-02") {
		t.Errorf("expected W2026-02 standalone divider for older week, got:\n%s", out)
	}
	// And that older divider must NOT be on the Deployed header line.
	if strings.Contains(deployedLine, "W2026-02") {
		t.Errorf("older week should not appear on the Deployed header line, got: %q", deployedLine)
	}
}

func TestRenderSnapshot_WeekDivider_NotShownForEmptyDeployed(t *testing.T) {
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: "a", Author: "alice", Subject: "ci only",
			Events: []clarityrefs.Event{ev("ci", "passed", 100)}},
	}}
	out := tui.RenderSnapshot(snap, 80, time.Unix(200, 0), 0)
	if strings.Contains(out, "W20") {
		t.Errorf("expected no week divider when nothing has been deployed, got:\n%s", out)
	}
}

func TestRenderSnapshot_TwoBatches_TwoSubheaders(t *testing.T) {
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "c", Author: "carol", Subject: "shipping", Events: []clarityrefs.Event{
				ev("ci", "passed", 300), ev("deploy", "started", 350),
			}},
			{SHA: "a", Author: "alice", Subject: "live", Events: []clarityrefs.Event{
				ev("ci", "passed", 50), ev("deploy", "passed", 75),
			}},
		},
	}
	out := tui.RenderSnapshot(snap, 80, time.Unix(400, 0), 0)
	if !strings.Contains(strings.ToLower(out), "deploying") {
		t.Errorf("expected 'deploying' for newest batch, got:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "deployed") {
		t.Errorf("expected 'deployed' for older batch, got:\n%s", out)
	}
}

// The topmost passed batch in Deployed gets a "live on production" prefix
// to distinguish "what's running right now" from settled deploy history.
// Older batches keep the plain "deployed Xm ago" label.
func TestRenderSnapshot_TopDeployedBatch_IsMarkedLive(t *testing.T) {
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "b", Author: "bob", Subject: "fresh ship", Events: []clarityrefs.Event{
				ev("ci", "passed", 300), ev("deploy", "passed", 350),
			}},
			{SHA: "a", Author: "alice", Subject: "older ship", Events: []clarityrefs.Event{
				ev("ci", "passed", 50), ev("deploy", "passed", 75),
			}},
		},
	}
	out := tui.RenderSnapshot(snap, 80, time.Unix(400, 0), 0)

	if !strings.Contains(out, "live on production") {
		t.Errorf("expected 'live on production' on the topmost batch, got:\n%s", out)
	}
	// The label should appear exactly once — only the freshest batch is live.
	if n := strings.Count(out, "live on production"); n != 1 {
		t.Errorf("expected exactly 1 'live on production' marker, got %d:\n%s", n, out)
	}
}

// The row format no longer trails with explicit "ci" / "deploy" labels —
// the icon and section convey that.
func TestRenderSnapshot_DoesNotTrailWithStageLabels(t *testing.T) {
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "a", Author: "alice", Subject: "x", Events: []clarityrefs.Event{
				ev("ci", "passed", 100), ev("deploy", "passed", 150),
			}},
		},
	}
	out := tui.RenderSnapshot(snap, 80, time.Time{}, 0)
	// Each row should not contain the verbatim words "ci" or "deploy"
	// after the subject. We check rows containing the author rather than
	// the whole output (subheaders may legitimately mention "deploy*").
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "alice") {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "ci") || strings.Contains(lower, "deploy") {
			t.Errorf("commit row should not mention 'ci'/'deploy', got: %q", line)
		}
	}
}

// A live (not-yet-deployed) commit shows its lead-time timer ticking against now.
func TestRenderSnapshot_LiveCommit_RendersTickingLeadTime(t *testing.T) {
	commitTime := time.Unix(1000, 0)
	now := time.Unix(1090, 0) // 1m 30s later
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "a", Author: "alice", Subject: "wip", Time: commitTime,
				Events: []clarityrefs.Event{ev("ci", "started", 50)}},
		},
	}
	out := tui.RenderSnapshot(snap, 80, now, 0)
	if !strings.Contains(out, "1m 30s") {
		t.Errorf("expected '1m 30s' lead time in output, got:\n%s", out)
	}
}

// A deployed commit shows its lead time frozen at the deploy moment, not now.
func TestRenderSnapshot_DeployedCommit_FrozenLeadTime(t *testing.T) {
	commitTime := time.Unix(1000, 0)
	deployTime := time.Unix(1300, 0) // exactly 5m after commit
	now := time.Unix(9999, 0)        // long after — must NOT influence frozen value
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "a", Author: "alice", Subject: "shipped", Time: commitTime,
				Events: []clarityrefs.Event{
					ev("ci", "passed", 100),
					{Stage: "deploy", Status: "passed", Time: deployTime},
				}},
		},
	}
	out := tui.RenderSnapshot(snap, 80, now, 0)
	if !strings.Contains(out, "5m 00s") {
		t.Errorf("expected '5m 00s' frozen lead time, got:\n%s", out)
	}
}

// Sections appear top-to-bottom in lifecycle order: HEAD → CI Passed → Deployed.
func TestRenderSnapshot_SectionsInLifecycleOrder(t *testing.T) {
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "d", Author: "dave", Subject: "broken"},
			{SHA: "c", Author: "carol", Subject: "built", Events: []clarityrefs.Event{ev("ci", "passed", 200)}},
			{SHA: "b", Author: "bob", Subject: "shipped", Events: []clarityrefs.Event{
				ev("ci", "passed", 100), ev("deploy", "passed", 150),
			}},
		},
	}
	out := tui.RenderSnapshot(snap, 80, time.Time{}, 0)
	pHead := strings.Index(out, "HEAD")
	pCI := strings.Index(out, "CI Passed")
	pDeployed := strings.Index(out, "Deployed")
	if !(pHead < pCI && pCI < pDeployed) {
		t.Errorf("expected sections in order HEAD → CI Passed → Deployed, got positions %d, %d, %d in:\n%s",
			pHead, pCI, pDeployed, out)
	}
}
