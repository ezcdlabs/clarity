package tui_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/adapters/tui"
	"github.com/ezcdlabs/clarity/internal/core"
)

// newModel constructs a Model sized for a fixed terminal so layout-sensitive
// assertions are stable across machines. The `repo` argument is the repo
// name we expect the snapshot to carry — for tests that send a snapshot
// via Update, callers set it on the Snapshot they pass in; for tests that
// only check the loading-state View() before any snapshot, the argument
// is unused.
func newModel(_ string) tui.Model {
	return tui.New().WithSize(120, 40)
}

// withRepo is a tiny convenience for tests that want to assert against a
// rendered repo name: returns a Snapshot copy stamped with the given name.
func withRepo(snap core.Snapshot, name string) core.Snapshot {
	snap.RepoName = name
	return snap
}

func TestModel_BeforeFirstSnapshot_ShowsLoading(t *testing.T) {
	m := newModel("clarity")
	out := m.View().Content
	if !strings.Contains(strings.ToLower(out), "loading") {
		t.Errorf("expected a loading indicator before any snapshot, got:\n%s", out)
	}
	if strings.Contains(out, "no commits yet") {
		t.Errorf("should not show 'no commits yet' before first snapshot, got:\n%s", out)
	}
}

// After the first snapshot arrives — even an empty one — the loading
// placeholder is gone and the structural dividers take over instead.
func TestModel_AfterEmptySnapshot_ShowsDividersNotLoading(t *testing.T) {
	m, _ := newModel("clarity").Update(tui.ViewMsg(view(core.Snapshot{})))
	out := m.View().Content
	if strings.Contains(strings.ToLower(out), "loading") {
		t.Errorf("should no longer show loading once a snapshot has arrived, got:\n%s", out)
	}
	for _, label := range []string{"HEAD", "CI Passed", "Deployed"} {
		if !strings.Contains(out, label) {
			t.Errorf("expected divider %q after empty snapshot, got:\n%s", label, out)
		}
	}
}

func TestModel_AfterPopulatedSnapshot_RendersCommits(t *testing.T) {
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "1", Author: "alice", Subject: "first commit",
				Events: []clarityrefs.Event{{Stage: "ci", Status: "passed", Time: time.Unix(100, 0)}}},
		},
	}
	m, _ := newModel("clarity").Update(tui.ViewMsg(view(snap)))
	out := m.View().Content
	if !strings.Contains(out, "alice") {
		t.Errorf("expected commit author rendered, got:\n%s", out)
	}
	if !strings.Contains(out, "first commit") {
		t.Errorf("expected commit subject rendered, got:\n%s", out)
	}
}

// The header shows the repository name (not the branch). After step 4 the
// repo name travels on the Snapshot, so we send one through Update to put
// the name into the model before asserting the View output.
func TestModel_HeaderShowsRepoName(t *testing.T) {
	m, _ := newModel("").Update(tui.ViewMsg(view(withRepo(core.Snapshot{}, "my-cool-repo"))))
	out := m.View().Content
	if !strings.Contains(out, "my-cool-repo") {
		t.Errorf("expected repo name in header, got:\n%s", out)
	}
}

// The header shows pipeline status badges for CI and deploy.
func TestModel_HeaderShowsCIAndDeployBadges(t *testing.T) {
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "1", Author: "alice", Subject: "x", Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: time.Unix(100, 0)},
				{Stage: "deploy", Status: "passed", Time: time.Unix(150, 0)},
			}},
		},
	}
	m, _ := newModel("clarity").Update(tui.ViewMsg(view(snap)))
	out := m.View().Content
	if !strings.Contains(strings.ToLower(out), "ci") {
		t.Errorf("expected 'ci' badge in header, got:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "deploy") {
		t.Errorf("expected 'deploy' badge in header, got:\n%s", out)
	}
}

// When the body content exceeds the viewport height, the visible View() is
// clipped — and scrolling down reveals different content.
func TestModel_TallContent_ClipsThenScrolls(t *testing.T) {
	// Build a snapshot with 30 commits — far more than our small terminal.
	var commits []core.CommitView
	for i := 0; i < 30; i++ {
		commits = append(commits, core.CommitView{
			SHA:     pad("sha", i),
			Author:  pad("auth", i),
			Subject: pad("subj", i),
			Time:    time.Unix(int64(1_000_000+i), 0),
		})
	}
	snap := core.Snapshot{Commits: commits}

	// Tiny terminal — 10 rows total, leaving ~8 for the body.
	m := tui.New().WithSize(80, 10)
	m2, _ := m.Update(tui.ViewMsg(view(snap)))
	beforeScroll := m2.View().Content

	// We can't fit all 30 authors in 10 rows, so some must be missing.
	missing := 0
	for i := 0; i < 30; i++ {
		if !strings.Contains(beforeScroll, pad("auth", i)) {
			missing++
		}
	}
	if missing == 0 {
		t.Errorf("expected some commits to be clipped off-screen, but all rendered")
	}

	// Scroll down with PgDn; the visible content should change.
	m3, _ := m2.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	afterScroll := m3.View().Content
	if beforeScroll == afterScroll {
		t.Errorf("expected View() to change after PgDn scroll")
	}
}

// pad makes a unique string of the form "<prefix>-NNN" so we can search for
// individual commit identifiers without collision.
func pad(prefix string, n int) string {
	return fmt.Sprintf("%s-%03d", prefix, n)
}

// A stale View (from CachedLens) gets a small "refreshing…" indicator
// on the header line so the user knows the displayed data is from a
// cache and a fresh fetch is still in flight. The plain-text test sees
// the literal word regardless of styling — we deliberately don't pin a
// specific column to keep the test resilient to layout tuning.
func TestModel_StaleView_ShowsRefreshingIndicator(t *testing.T) {
	view := core.View{
		Snapshot: core.Snapshot{RepoName: "clarity"},
		Stale:    true,
	}
	m, _ := newModel("").Update(tui.ViewMsg(view))
	out := m.View().Content
	if !strings.Contains(strings.ToLower(out), "refreshing") {
		t.Errorf("expected 'refreshing' indicator for a stale view, got:\n%s", out)
	}
}

// The complementary case: a fresh View does NOT show the indicator —
// nothing in flight, nothing to advertise. Otherwise the indicator
// would be on-screen forever once a stale view ever arrived.
func TestModel_FreshView_NoRefreshingIndicator(t *testing.T) {
	view := core.View{
		Snapshot: core.Snapshot{RepoName: "clarity"},
		Stale:    false,
	}
	m, _ := newModel("").Update(tui.ViewMsg(view))
	out := m.View().Content
	if strings.Contains(strings.ToLower(out), "refreshing") {
		t.Errorf("expected no 'refreshing' indicator for a fresh view, got:\n%s", out)
	}
}

// The "q to quit" hint sits in the top-right of the header line.
func TestModel_QuitHint_OnHeaderLine_RightAligned(t *testing.T) {
	m := newModel("clarity")
	out := m.View().Content
	if !strings.Contains(strings.ToLower(out), "q") {
		t.Errorf("expected quit hint, got:\n%s", out)
	}
	// Quit hint must appear on the first line (the header), not below.
	firstLine := strings.SplitN(out, "\n", 2)[0]
	if !strings.Contains(strings.ToLower(firstLine), "q") {
		t.Errorf("expected quit hint on the header line, got first line:\n%q", firstLine)
	}
}
