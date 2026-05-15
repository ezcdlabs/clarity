package tui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/tui"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

const fakeSHA1 = "abc1234567890abc1234567890abc1234567890a"
const fakeSHA2 = "def4567890abcdef4567890abcdef4567890abcd"

func TestRenderPlain_EmptySnapshot_ShowsHeaderAndSections(t *testing.T) {
	out := tui.RenderPlain("clarity", watcher.Snapshot{}, time.Unix(0, 0), tui.PlainOptions{})
	for _, want := range []string{"clarity", "HEAD", "CI Passed", "Deployed"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestRenderPlain_HeaderShowsCIAndDeployBadges(t *testing.T) {
	snap := watcher.Snapshot{Commits: []watcher.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "x", Events: []clarityrefs.Event{
			{Stage: "ci", Status: "passed", Time: time.Unix(100, 0)},
			{Stage: "deploy", Status: "failed", Time: time.Unix(200, 0)},
		}},
	}}
	out := tui.RenderPlain("clarity", snap, time.Unix(300, 0), tui.PlainOptions{})
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
	snap := watcher.Snapshot{Commits: []watcher.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "fix things",
			Events: []clarityrefs.Event{{Stage: "ci", Status: "passed", Time: time.Unix(100, 0)}}},
	}}
	out := tui.RenderPlain("clarity", snap, time.Unix(200, 0), tui.PlainOptions{})
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
	snap := watcher.Snapshot{Commits: []watcher.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "broke ci",
			Events: []clarityrefs.Event{{Stage: "ci", Status: "failed", Time: time.Unix(100, 0)}}},
	}}
	out := tui.RenderPlain("clarity", snap, time.Unix(200, 0), tui.PlainOptions{})
	if !strings.Contains(out, "✗") {
		t.Errorf("expected ✗ icon for failed commit, got:\n%s", out)
	}
}

func TestRenderPlain_StartedCommit_ShowsStaticEllipsis(t *testing.T) {
	snap := watcher.Snapshot{Commits: []watcher.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "in progress",
			Events: []clarityrefs.Event{{Stage: "ci", Status: "started", Time: time.Unix(100, 0)}}},
	}}
	out := tui.RenderPlain("clarity", snap, time.Unix(200, 0), tui.PlainOptions{})
	// The TUI spinner animates; plain mode renders a static glyph so the
	// row is still visually distinct from passed/failed without depending
	// on terminal animation.
	if !strings.Contains(out, "…") {
		t.Errorf("expected … glyph for in-progress commit, got:\n%s", out)
	}
}

func TestRenderPlain_InFlightDeploy_ShowsDeployingSubheader(t *testing.T) {
	snap := watcher.Snapshot{Commits: []watcher.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "deploying",
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: time.Unix(100, 0)},
				{Stage: "deploy", Status: "started", Time: time.Unix(150, 0)},
			}},
	}}
	out := tui.RenderPlain("clarity", snap, time.Unix(200, 0), tui.PlainOptions{})
	if !strings.Contains(out, "deploying") {
		t.Errorf("expected 'deploying' subheader for in-flight batch, got:\n%s", out)
	}
}

func TestRenderPlain_DeployedBatch_ShowsAgoAndLive(t *testing.T) {
	snap := watcher.Snapshot{Commits: []watcher.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "shipped",
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: time.Unix(100, 0)},
				{Stage: "deploy", Status: "passed", Time: time.Unix(200, 0)},
			}},
	}}
	// 200 seconds after deploy_passed.
	out := tui.RenderPlain("clarity", snap, time.Unix(400, 0), tui.PlainOptions{})
	if !strings.Contains(out, "deployed") {
		t.Errorf("expected 'deployed' subheader, got:\n%s", out)
	}
	if !strings.Contains(out, "live") {
		t.Errorf("expected 'live' marker on the latest deploy batch, got:\n%s", out)
	}
}

func TestRenderPlain_ShowSHAs_OptIn(t *testing.T) {
	snap := watcher.Snapshot{Commits: []watcher.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "x",
			Events: []clarityrefs.Event{{Stage: "ci", Status: "passed", Time: time.Unix(100, 0)}}},
	}}
	short := fakeSHA1[:7]

	defaultOut := tui.RenderPlain("clarity", snap, time.Unix(200, 0), tui.PlainOptions{})
	if strings.Contains(defaultOut, short) {
		t.Errorf("default output should not contain short SHA, got:\n%s", defaultOut)
	}

	withSHAs := tui.RenderPlain("clarity", snap, time.Unix(200, 0), tui.PlainOptions{ShowSHAs: true})
	if !strings.Contains(withSHAs, short) {
		t.Errorf("--show-shas output should contain short SHA %q, got:\n%s", short, withSHAs)
	}
}

func TestRenderPlain_Limit_TruncatesCommits(t *testing.T) {
	snap := watcher.Snapshot{Commits: []watcher.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "newer",
			Events: []clarityrefs.Event{{Stage: "ci", Status: "passed", Time: time.Unix(100, 0)}}},
		{SHA: fakeSHA2, Author: "bob", Subject: "older",
			Events: []clarityrefs.Event{{Stage: "ci", Status: "passed", Time: time.Unix(50, 0)}}},
	}}
	out := tui.RenderPlain("clarity", snap, time.Unix(200, 0), tui.PlainOptions{Limit: 1})
	if !strings.Contains(out, "newer") {
		t.Errorf("limit=1 should keep newer commit, got:\n%s", out)
	}
	if strings.Contains(out, "older") {
		t.Errorf("limit=1 should drop older commit, got:\n%s", out)
	}
}

func TestRenderPlain_NoColor(t *testing.T) {
	// Plain mode is for non-TTY consumers (pipes, agents). It must not embed
	// ANSI escape sequences — the row icons (✓ ✗ …) are UTF-8 glyphs only.
	snap := watcher.Snapshot{Commits: []watcher.CommitView{
		{SHA: fakeSHA1, Author: "alice", Subject: "x",
			Events: []clarityrefs.Event{{Stage: "ci", Status: "passed", Time: time.Unix(100, 0)}}},
	}}
	out := tui.RenderPlain("clarity", snap, time.Unix(200, 0), tui.PlainOptions{})
	if strings.Contains(out, "\x1b[") {
		t.Errorf("plain mode must not embed ANSI escapes, got:\n%q", out)
	}
}
