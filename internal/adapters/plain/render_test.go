package plain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/adapters/plain"
	"github.com/ezcdlabs/clarity/internal/core"
)

const fakeSHA1 = "abc1234567890abc1234567890abc1234567890a"
const fakeSHA2 = "def4567890abcdef4567890abcdef4567890abcd"

func TestRenderPlain_EmptySnapshot_ShowsHeaderAndSections(t *testing.T) {
	out := plain.RenderSnapshot("clarity", core.Snapshot{}, time.Unix(0, 0), plain.Options{})
	for _, want := range []string{"clarity", "HEAD", "CI Passed", "Deployed"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestRenderPlain_HeaderShowsCIAndDeployBadges(t *testing.T) {
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "x", Events: []clarityrefs.Event{
			{Stage: "ci", Status: "passed", Time: time.Unix(100, 0)},
			{Stage: "deploy", Status: "failed", Time: time.Unix(200, 0)},
		}},
	}}
	out := plain.RenderSnapshot("clarity", snap, time.Unix(300, 0), plain.Options{})
	first := strings.SplitN(out, "\n", 2)[0]
	if !strings.Contains(first, "ci") {
		t.Errorf("expected 'ci' in header line, got: %q", first)
	}
	if !strings.Contains(first, "deploy") {
		t.Errorf("expected 'deploy' in header line, got: %q", first)
	}
	if !strings.Contains(first, "passed") {
		t.Errorf("expected 'passed' in header line, got: %q", first)
	}
	if !strings.Contains(first, "failed") {
		t.Errorf("expected 'failed' in header line, got: %q", first)
	}
}

func TestRenderPlain_PassedCommit_ShowsTickAndAuthor(t *testing.T) {
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "fix things",
			Events: []clarityrefs.Event{{Stage: "ci", Status: "passed", Time: time.Unix(100, 0)}}},
	}}
	out := plain.RenderSnapshot("clarity", snap, time.Unix(200, 0), plain.Options{})
	if !strings.Contains(out, "✓") {
		t.Errorf("expected ✓ icon for passed commit, got:\n%s", out)
	}
	if !strings.Contains(out, "alice") {
		t.Errorf("expected author 'alice' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "fix things") {
		t.Errorf("expected subject 'fix things' in output, got:\n%s", out)
	}
}

func TestRenderPlain_FailedCommit_ShowsCross(t *testing.T) {
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "broke ci",
			Events: []clarityrefs.Event{{Stage: "ci", Status: "failed", Time: time.Unix(100, 0)}}},
	}}
	out := plain.RenderSnapshot("clarity", snap, time.Unix(200, 0), plain.Options{})
	if !strings.Contains(out, "✗") {
		t.Errorf("expected ✗ icon for failed commit, got:\n%s", out)
	}
}

func TestRenderPlain_StartedCommit_ShowsStaticEllipsis(t *testing.T) {
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "in progress",
			Events: []clarityrefs.Event{{Stage: "ci", Status: "started", Time: time.Unix(100, 0)}}},
	}}
	out := plain.RenderSnapshot("clarity", snap, time.Unix(200, 0), plain.Options{})
	// The TUI spinner animates; plain mode renders a static glyph so the
	// row is still visually distinct from passed/failed without depending
	// on terminal animation.
	if !strings.Contains(out, "…") {
		t.Errorf("expected … glyph for in-progress commit, got:\n%s", out)
	}
}

func TestRenderPlain_InFlightDeploy_ShowsDeployingSubheader(t *testing.T) {
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "deploying",
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: time.Unix(100, 0)},
				{Stage: "deploy", Status: "started", Time: time.Unix(150, 0)},
			}},
	}}
	out := plain.RenderSnapshot("clarity", snap, time.Unix(200, 0), plain.Options{})
	if !strings.Contains(out, "deploying") {
		t.Errorf("expected 'deploying' subheader for in-flight batch, got:\n%s", out)
	}
}

func TestRenderPlain_DeployedBatch_ShowsAgoAndLive(t *testing.T) {
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "shipped",
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: time.Unix(100, 0)},
				{Stage: "deploy", Status: "passed", Time: time.Unix(200, 0)},
			}},
	}}
	// 200 seconds after deploy_passed.
	out := plain.RenderSnapshot("clarity", snap, time.Unix(400, 0), plain.Options{})
	if !strings.Contains(out, "deployed") {
		t.Errorf("expected 'deployed' subheader, got:\n%s", out)
	}
	if !strings.Contains(out, "live") {
		t.Errorf("expected 'live' marker on the latest deploy batch, got:\n%s", out)
	}
}

func TestRenderPlain_ShowSHAs_OptIn(t *testing.T) {
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "x",
			Events: []clarityrefs.Event{{Stage: "ci", Status: "passed", Time: time.Unix(100, 0)}}},
	}}
	short := fakeSHA1[:7]

	defaultOut := plain.RenderSnapshot("clarity", snap, time.Unix(200, 0), plain.Options{})
	if strings.Contains(defaultOut, short) {
		t.Errorf("default output should not contain short SHA, got:\n%s", defaultOut)
	}

	withSHAs := plain.RenderSnapshot("clarity", snap, time.Unix(200, 0), plain.Options{ShowSHAs: true})
	if !strings.Contains(withSHAs, short) {
		t.Errorf("--show-shas output should contain short SHA %q, got:\n%s", short, withSHAs)
	}
}

func TestRenderPlain_Limit_TruncatesCommits(t *testing.T) {
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "newer",
			Events: []clarityrefs.Event{{Stage: "ci", Status: "passed", Time: time.Unix(100, 0)}}},
		{SHA: fakeSHA2, Author: "bob", Subject: "older",
			Events: []clarityrefs.Event{{Stage: "ci", Status: "passed", Time: time.Unix(50, 0)}}},
	}}
	out := plain.RenderSnapshot("clarity", snap, time.Unix(200, 0), plain.Options{Limit: 1})
	if !strings.Contains(out, "newer") {
		t.Errorf("limit=1 should keep newer commit, got:\n%s", out)
	}
	if strings.Contains(out, "older") {
		t.Errorf("limit=1 should drop older commit, got:\n%s", out)
	}
}

func TestRenderPlain_WeekDivider_AppearsAboveFirstBatchOfWeek(t *testing.T) {
	// One deploy in ISO week 2 of 2026. Plain output should include a
	// "W2026-02" divider line with deploy count and avg lead time.
	c := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	d := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "ship",
			Time:   c,
			Events: []clarityrefs.Event{{Stage: "deploy", Status: "passed", Time: d}}},
	}}
	out := plain.RenderSnapshot("clarity", snap, d.Add(time.Hour), plain.Options{})
	if !strings.Contains(out, "W2026-02") {
		t.Errorf("expected W2026-02 week divider, got:\n%s", out)
	}
	if !strings.Contains(out, "1 deploy") {
		t.Errorf("expected '1 deploy' on divider, got:\n%s", out)
	}
}

func TestRenderPlain_WeekDivider_TwoWeeks_TwoDividers(t *testing.T) {
	// Two deploys in different ISO weeks.
	c1 := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	d1 := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC) // week 2
	c2 := time.Date(2026, 1, 12, 9, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 1, 12, 10, 0, 0, 0, time.UTC) // week 3
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: fakeSHA2, Author: "alice", Subject: "newer", Time: c2,
			Events: []clarityrefs.Event{{Stage: "deploy", Status: "passed", Time: d2}}},
		{SHA: fakeSHA1, Author: "alice", Subject: "older", Time: c1,
			Events: []clarityrefs.Event{{Stage: "deploy", Status: "passed", Time: d1}}},
	}}
	out := plain.RenderSnapshot("clarity", snap, d2.Add(time.Hour), plain.Options{})
	if !strings.Contains(out, "W2026-03") {
		t.Errorf("expected W2026-03 divider, got:\n%s", out)
	}
	if !strings.Contains(out, "W2026-02") {
		t.Errorf("expected W2026-02 divider, got:\n%s", out)
	}
	// Order: newer week appears above older week (matches the snapshot's
	// newest-first commit order).
	idxNewer := strings.Index(out, "W2026-03")
	idxOlder := strings.Index(out, "W2026-02")
	if idxNewer == -1 || idxOlder == -1 || idxNewer > idxOlder {
		t.Errorf("expected W2026-03 to appear above W2026-02, got positions %d vs %d", idxNewer, idxOlder)
	}
}

func TestRenderPlain_WeekDivider_MergedIntoDeployedHeader(t *testing.T) {
	// The topmost week's stats share the Deployed section header line —
	// saves a row and parallels the TUI's merged divider.
	c := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	d := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "ship",
			Time:   c,
			Events: []clarityrefs.Event{{Stage: "deploy", Status: "passed", Time: d}}},
	}}
	out := plain.RenderSnapshot("clarity", snap, d.Add(time.Hour), plain.Options{})
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Deployed") {
			if !strings.Contains(line, "W2026-02") {
				t.Errorf("expected merged Deployed + W2026-02 header line, got: %q", line)
			}
			return
		}
	}
	t.Errorf("no Deployed header line found, got:\n%s", out)
}

func TestRenderPlain_WeekDivider_OnlyForDeployedSection(t *testing.T) {
	// A commit with only CI events (no deploy) must not produce a week
	// divider — week dividers are a Deployed-section feature.
	c := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "ci only", Time: c,
			Events: []clarityrefs.Event{{Stage: "ci", Status: "passed", Time: c.Add(time.Minute)}}},
	}}
	out := plain.RenderSnapshot("clarity", snap, c.Add(time.Hour), plain.Options{})
	if strings.Contains(out, "W2026") {
		t.Errorf("expected no week divider when there are no deploys, got:\n%s", out)
	}
}

func TestRenderPlain_NoColor(t *testing.T) {
	// Plain mode is for non-TTY consumers (pipes, agents). It must not embed
	// ANSI escape sequences — the row icons (✓ ✗ …) are UTF-8 glyphs only.
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "x",
			Events: []clarityrefs.Event{{Stage: "ci", Status: "passed", Time: time.Unix(100, 0)}}},
	}}
	out := plain.RenderSnapshot("clarity", snap, time.Unix(200, 0), plain.Options{})
	if strings.Contains(out, "\x1b[") {
		t.Errorf("plain mode must not embed ANSI escapes, got:\n%q", out)
	}
}
