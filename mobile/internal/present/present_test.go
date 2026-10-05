package present_test

import (
	"fmt"
	"strings"
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

// section returns the named lifecycle band, failing if it is missing — all
// three are supposed to be present on every flow.
func section(t *testing.T, f *v1.Flow, kind v1.SectionKind) *v1.Section {
	t.Helper()
	for _, sec := range f.Sections {
		if sec.Kind == kind {
			return sec
		}
	}
	t.Fatalf("no %v section in %v", kind, f.Sections)
	return nil
}

// everyCommit flattens a flow, for assertions about one commit wherever it
// ended up.
func everyCommit(f *v1.Flow) []*v1.Commit {
	var out []*v1.Commit
	for _, sec := range f.Sections {
		out = append(out, sec.Commits...)
		for _, b := range sec.Batches {
			out = append(out, b.Commits...)
		}
	}
	return out
}

func find(f *v1.Flow, sha string) *v1.Commit {
	for _, c := range everyCommit(f) {
		if c.Sha == sha {
			return c
		}
	}
	return nil
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

	shipped := section(t, f, v1.SectionKind_SECTION_KIND_DEPLOYED)
	if len(shipped.Batches) == 0 {
		t.Fatal("the shipped commit produced no batch")
	}
	batch := shipped.Batches[0]
	if batch.Status != v1.Status_STATUS_PASSED {
		t.Errorf("batch status = %v", batch.Status)
	}
	if batch.DeployedAgo == "" || batch.DeployedUnixSeconds == 0 {
		t.Error("a batch must carry when it landed, formatted and raw")
	}

	c := batch.Commits[0]
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

	undeployed := find(out.Flows[0], "bbbbbbbbbbbb")
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

	c := find(out.Flows[0], "cccccccccccc")
	if c == nil {
		t.Fatal("the excluded commit is missing from every section")
	}
	if c.HasLeadTime {
		t.Errorf("a commit the mode excludes reports a lead time of %q", c.LeadTime)
	}
	if c.LeadTimeSeconds != 0 {
		t.Errorf("lead time seconds = %d, want 0", c.LeadTimeSeconds)
	}
}

// TestView_StatusVocabularyIsSemantic verifies the enum carries meaning
// rather than the terminal's glyphs. ✓ and ✗ were chosen for a greyscale-safe
// palette and mean nothing on a phone.
func TestView_StatusVocabularyIsSemantic(t *testing.T) {
	snap := deployed()
	snap.Commits[0].Events = []clarityrefs.Event{{Stage: "ci", Status: "failed", Time: at(6)}}
	out := mapped(t, snap, at(10))

	c := find(out.Flows[0], "bbbbbbbbbbbb")
	if c == nil {
		t.Fatal("the failing commit is missing from every section")
	}
	if c.Ci != v1.Status_STATUS_FAILED {
		t.Errorf("ci status = %v, want FAILED", c.Ci)
	}
}

// TestView_UnknownStatusReadsAsUnreported covers a client older than the
// event that reached it. An unrecognised status must not render as broken.
func TestView_UnknownStatusReadsAsUnreported(t *testing.T) {
	snap := deployed()
	snap.Commits[0].Events = []clarityrefs.Event{{Stage: "ci", Status: "quarantined", Time: at(6)}}
	out := mapped(t, snap, at(10))

	if c := find(out.Flows[0], "bbbbbbbbbbbb"); c != nil && c.Ci == v1.Status_STATUS_FAILED {
		t.Error("an unrecognised status rendered as a failure")
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

// TestView_IsStampedWithWhenItWasBuilt covers two things at once, which is why
// it is one test and not two.
//
// A client holds the last good view across a failed refresh, so it needs to say
// how old what it is showing is. And the stamp is never zero, which is what
// keeps a view with nothing in it representable: gomobile's fromSlice turns a
// zero-length []byte into a null array, and a message with no fields set
// encodes to zero bytes.
func TestView_IsStampedWithWhenItWasBuilt(t *testing.T) {
	out := mapped(t, deployed(), at(90))

	if out.GeneratedUnixSeconds != at(90).Unix() {
		t.Errorf("stamped %d, want %d", out.GeneratedUnixSeconds, at(90).Unix())
	}
	if out.GeneratedUnixSeconds == 0 {
		t.Error("an unstamped view encodes to zero bytes when it is otherwise empty")
	}
}

// TestView_TheThreeSectionsAreAlwaysPresent pins the frame.
//
// The TUI draws HEAD, CI Passed and Deployed whether or not anything is in
// them, because they say what the lifecycle is — a repository with nothing
// shipped should read as "nothing has shipped", not as a view with a section
// missing. A client cannot reconstruct that from a list of non-empty groups.
func TestView_TheThreeSectionsAreAlwaysPresent(t *testing.T) {
	snap := core.Snapshot{RepoName: "api", Commits: []core.CommitView{
		{SHA: "aaaaaaaaaaaa", Subject: "feat: just landed", Author: "alice", Time: at(1)},
	}}
	out := mapped(t, snap, at(10))

	var kinds []v1.SectionKind
	var labels []string
	for _, s := range out.Flows[0].Sections {
		kinds = append(kinds, s.Kind)
		labels = append(labels, s.Label)
	}

	want := []v1.SectionKind{
		v1.SectionKind_SECTION_KIND_HEAD,
		v1.SectionKind_SECTION_KIND_CI_PASSED,
		v1.SectionKind_SECTION_KIND_DEPLOYED,
	}
	if len(kinds) != len(want) {
		t.Fatalf("got %d sections, want 3: %v", len(kinds), kinds)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Errorf("section %d is %v, want %v — the order is the lifecycle", i, kinds[i], want[i])
		}
	}
	// The same words the terminal writes, decided once rather than per client.
	if got := labels; got[0] != "HEAD" || got[1] != "CI Passed" || got[2] != "Deployed" {
		t.Errorf("labels = %q, want HEAD / CI Passed / Deployed", got)
	}
}

// TestView_EachDeployIsItsOwnBatch covers the thing the flat shape got wrong:
// five deploys became five sections all labelled "Deployed", so the phone
// repeated the word down the screen while the terminal says it once.
func TestView_EachDeployIsItsOwnBatch(t *testing.T) {
	snap := core.Snapshot{RepoName: "api", Commits: []core.CommitView{
		{SHA: "bbbbbbbbbbbb", Subject: "feat: second", Author: "bob", Time: at(4),
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: at(5)},
				{Stage: "deploy", Status: "passed", Time: at(6)},
			}},
		{SHA: "aaaaaaaaaaaa", Subject: "feat: first", Author: "alice", Time: at(1),
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: at(2)},
				{Stage: "deploy", Status: "passed", Time: at(3)},
			}},
	}}
	out := mapped(t, snap, at(10))

	shipped := section(t, out.Flows[0], v1.SectionKind_SECTION_KIND_DEPLOYED)
	if len(shipped.Batches) != 2 {
		t.Fatalf("two deploys produced %d batches", len(shipped.Batches))
	}
	if len(shipped.Commits) != 0 {
		t.Error("every commit in the deployed section belongs to a batch")
	}
	if shipped.Batches[0].DeployedUnixSeconds <= shipped.Batches[1].DeployedUnixSeconds {
		t.Error("batches run newest first, like the terminal")
	}
}

// TestView_TheNewestPassingBatchIsLive distinguishes the present from history.
// The TUI escalates it to "live on production" because it is the one batch
// that answers "what is running right now".
func TestView_TheNewestPassingBatchIsLive(t *testing.T) {
	snap := core.Snapshot{RepoName: "api", Commits: []core.CommitView{
		{SHA: "bbbbbbbbbbbb", Subject: "feat: second", Time: at(4),
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: at(5)},
				{Stage: "deploy", Status: "passed", Time: at(6)},
			}},
		{SHA: "aaaaaaaaaaaa", Subject: "feat: first", Time: at(1),
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: at(2)},
				{Stage: "deploy", Status: "passed", Time: at(3)},
			}},
	}}
	out := mapped(t, snap, at(10))
	batches := section(t, out.Flows[0], v1.SectionKind_SECTION_KIND_DEPLOYED).Batches

	if !batches[0].Live {
		t.Error("the newest passing deploy is what is on production")
	}
	if batches[1].Live {
		t.Error("exactly one batch is live; the rest are settled history")
	}
	if batches[0].Label != "live on production · deployed" {
		t.Errorf("live label = %q", batches[0].Label)
	}
	if batches[1].Label != "deployed" {
		t.Errorf("older label = %q", batches[1].Label)
	}
	// The time is not baked into the label: a client places it separately and
	// keeps it ticking.
	if strings.Contains(batches[0].Label, "ago") {
		t.Error("the label carries the time, so it cannot tick")
	}
}

// TestView_AnInFlightDeploySitsInCIPassed covers where a deploy that has not
// landed belongs. Above the line, with the commits still waiting — not in
// Deployed, which would claim it shipped.
func TestView_AnInFlightDeploySitsInCIPassed(t *testing.T) {
	snap := core.Snapshot{RepoName: "api", Commits: []core.CommitView{
		{SHA: "aaaaaaaaaaaa", Subject: "feat: going out", Time: at(1),
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: at(2)},
				{Stage: "deploy", Status: "started", Time: at(3)},
			}},
	}}
	out := mapped(t, snap, at(10))

	green := section(t, out.Flows[0], v1.SectionKind_SECTION_KIND_CI_PASSED)
	if len(green.Batches) != 1 {
		t.Fatalf("the in-flight deploy produced %d batches in CI Passed", len(green.Batches))
	}
	if green.Batches[0].Label != "deploying…" {
		t.Errorf("in-flight label = %q", green.Batches[0].Label)
	}
	if green.Batches[0].Live {
		t.Error("a deploy that has not landed is not what is on production")
	}
	if len(section(t, out.Flows[0], v1.SectionKind_SECTION_KIND_DEPLOYED).Batches) != 0 {
		t.Error("an unfinished deploy must not appear as shipped")
	}
}

// TestView_AFailedDeploySaysSo keeps the third subheader the TUI writes.
func TestView_AFailedDeploySaysSo(t *testing.T) {
	snap := core.Snapshot{RepoName: "api", Commits: []core.CommitView{
		{SHA: "aaaaaaaaaaaa", Subject: "feat: broke", Time: at(1),
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: at(2)},
				{Stage: "deploy", Status: "failed", Time: at(3)},
			}},
	}}
	out := mapped(t, snap, at(10))

	green := section(t, out.Flows[0], v1.SectionKind_SECTION_KIND_CI_PASSED)
	if len(green.Batches) != 1 {
		t.Fatalf("expected the stuck batch, got %d", len(green.Batches))
	}
	if green.Batches[0].Label != "deploy failed" {
		t.Errorf("failed label = %q", green.Batches[0].Label)
	}
	if green.Batches[0].Status != v1.Status_STATUS_FAILED {
		t.Errorf("failed batch status = %v", green.Batches[0].Status)
	}
}

// TestView_SupersededCIIsMarkedStale carries the TUI's muting rule across.
// A build that failed three commits ago and has since gone green is history,
// not an alarm, and the client cannot work that out from one commit.
func TestView_SupersededCIIsMarkedStale(t *testing.T) {
	snap := core.Snapshot{RepoName: "api", Commits: []core.CommitView{
		{SHA: "bbbbbbbbbbbb", Subject: "fix: green again", Time: at(5),
			Events: []clarityrefs.Event{{Stage: "ci", Status: "passed", Time: at(6)}}},
		{SHA: "aaaaaaaaaaaa", Subject: "feat: broke the build", Time: at(1),
			Events: []clarityrefs.Event{{Stage: "ci", Status: "failed", Time: at(2)}}},
	}}
	out := mapped(t, snap, at(10))

	older := find(out.Flows[0], "aaaaaaaaaaaa")
	newer := find(out.Flows[0], "bbbbbbbbbbbb")
	if older == nil || newer == nil {
		t.Fatal("a commit went missing")
	}
	if !older.CiStale {
		t.Error("a failure a newer commit has already superseded is stale")
	}
	if newer.CiStale {
		t.Error("the newest result is never stale")
	}
}

// TestView_ALiveLeadTimeCarriesItsAnchor is what lets a client tick.
//
// Without the anchor the only way to keep a running timer honest is to ask for
// the whole view again every second, which on a phone means walking the commit
// graph every second.
func TestView_ALiveLeadTimeCarriesItsAnchor(t *testing.T) {
	out := mapped(t, deployed(), at(10))

	live := find(out.Flows[0], "bbbbbbbbbbbb")
	if live == nil || !live.LeadTimeLive {
		t.Fatal("expected the un-shipped commit to have a running clock")
	}
	// Anchored where the clock started, so now - anchor is the elapsed time.
	if want := at(10).Add(-time.Duration(live.LeadTimeSeconds) * time.Second).Unix(); live.LeadTimeAnchorUnixSeconds != want {
		t.Errorf("anchor = %d, want %d", live.LeadTimeAnchorUnixSeconds, want)
	}

	frozen := find(out.Flows[0], "aaaaaaaaaaaa")
	if frozen == nil || frozen.LeadTimeLive {
		t.Fatal("expected the shipped commit to have a stopped clock")
	}
	if frozen.LeadTimeAnchorUnixSeconds != 0 {
		t.Error("a stopped clock has nothing to tick from, and an anchor would invite one")
	}
}

// midWeek is a fixed Wednesday, so "this week" and "nine days ago" are always
// different ISO weeks.
//
// time.Now() was used here and it made these tests depend on the day they ran:
// written on a Friday they passed, and on the following Monday nine days back
// landed inside the same ISO week and the divider vanished. A test about weeks
// cannot be allowed to ask which week it is.
func midWeek() time.Time {
	return time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
}

// weeksApart builds two deploys in different ISO weeks, so the divider between
// them has something to divide.
func weeksApart(now time.Time) core.Snapshot {
	thisWeek := now.Add(-2 * time.Hour)
	lastWeek := now.AddDate(0, 0, -9)
	return core.Snapshot{RepoName: "api", Commits: []core.CommitView{
		{SHA: "bbbbbbbbbbbb", Subject: "feat: recent", Time: thisWeek.Add(-time.Hour),
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: thisWeek.Add(-30 * time.Minute)},
				{Stage: "deploy", Status: "passed", Time: thisWeek},
			}},
		{SHA: "aaaaaaaaaaaa", Subject: "feat: older", Time: lastWeek.Add(-time.Hour),
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: lastWeek.Add(-30 * time.Minute)},
				{Stage: "deploy", Status: "passed", Time: lastWeek},
			}},
	}}
}

// TestView_ThisWeeksThroughputRidesOnTheSection mirrors where the terminal puts
// it: on the right of the Deployed rule, saving a row, because this week is the
// one a reader is asking about.
func TestView_ThisWeeksThroughputRidesOnTheSection(t *testing.T) {
	now := midWeek()
	out := mapped(t, weeksApart(now), now)

	shipped := section(t, out.Flows[0], v1.SectionKind_SECTION_KIND_DEPLOYED)
	if shipped.Summary == "" {
		t.Fatal("the Deployed section carries no throughput summary")
	}
	year, week := now.UTC().ISOWeek()
	if want := fmt.Sprintf("W%d-%02d", year, week); !strings.HasPrefix(shipped.Summary, want) {
		t.Errorf("summary = %q, want it to name %s", shipped.Summary, want)
	}
	if !strings.Contains(shipped.Summary, "deploy") {
		t.Errorf("summary = %q, want a deploy count", shipped.Summary)
	}
}

// TestView_OlderWeeksAreNamedAboveTheirFirstBatch covers the divider. Which
// batch carries it is a decision — batches run by commit, not by deploy time —
// so a client cannot work it out from position.
func TestView_OlderWeeksAreNamedAboveTheirFirstBatch(t *testing.T) {
	now := midWeek()
	out := mapped(t, weeksApart(now), now)

	batches := section(t, out.Flows[0], v1.SectionKind_SECTION_KIND_DEPLOYED).Batches
	if len(batches) != 2 {
		t.Fatalf("expected two batches, got %d", len(batches))
	}
	// The current week is already named on the section; repeating it above the
	// first batch would say the same thing twice.
	if batches[0].WeekLabel != "" {
		t.Errorf("this week is named twice: %q", batches[0].WeekLabel)
	}
	if batches[1].WeekLabel == "" {
		t.Fatal("the older week has no divider, so its rows read as this week's")
	}
	lastYear, lastWeek := now.AddDate(0, 0, -9).UTC().ISOWeek()
	if want := fmt.Sprintf("W%d-%02d", lastYear, lastWeek); !strings.HasPrefix(batches[1].WeekLabel, want) {
		t.Errorf("divider = %q, want it to name %s", batches[1].WeekLabel, want)
	}
}

// TestView_AWeekIsNamedOnlyOnce guards the repeat the TUI's weekShown map
// exists to prevent.
func TestView_AWeekIsNamedOnlyOnce(t *testing.T) {
	now := midWeek()
	lastWeek := now.AddDate(0, 0, -9)
	snap := core.Snapshot{RepoName: "api", Commits: []core.CommitView{
		{SHA: "bbbbbbbbbbbb", Subject: "feat: second", Time: lastWeek.Add(-time.Hour),
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: lastWeek.Add(-30 * time.Minute)},
				{Stage: "deploy", Status: "passed", Time: lastWeek.Add(time.Hour)},
			}},
		{SHA: "aaaaaaaaaaaa", Subject: "feat: first", Time: lastWeek.Add(-2 * time.Hour),
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: lastWeek.Add(-90 * time.Minute)},
				{Stage: "deploy", Status: "passed", Time: lastWeek},
			}},
	}}
	out := mapped(t, snap, now)

	batches := section(t, out.Flows[0], v1.SectionKind_SECTION_KIND_DEPLOYED).Batches
	named := 0
	for _, b := range batches {
		if b.WeekLabel != "" {
			named++
		}
	}
	if named != 1 {
		t.Errorf("two batches from one week produced %d dividers", named)
	}
}
