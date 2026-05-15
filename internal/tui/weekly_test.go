package tui_test

import (
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/tui"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

// utc is a shorthand for constructing UTC test times — ISO week boundaries
// depend on UTC, so the tests must too.
func utc(year, month, day, hour int) time.Time {
	return time.Date(year, time.Month(month), day, hour, 0, 0, 0, time.UTC)
}

// commit constructs a CommitView with a single deploy:passed event at deployAt.
// The CI event isn't needed for WeeklyStats — only the deploy time is bucketed.
func commit(sha string, commitAt, deployAt time.Time) watcher.CommitView {
	return watcher.CommitView{
		SHA:  sha,
		Time: commitAt,
		Events: []clarityrefs.Event{
			{Stage: "deploy", Status: "passed", Time: deployAt},
		},
	}
}

func TestWeeklyStats_EmptySnapshot(t *testing.T) {
	got := tui.WeeklyStats(watcher.Snapshot{})
	if len(got) != 0 {
		t.Errorf("expected no stats for empty snapshot, got %v", got)
	}
}

func TestWeeklyStats_SingleDeploy_OneCommit(t *testing.T) {
	// 2026-01-05 (Mon) → 2026-01-08 (Thu) — both ISO week 2 of 2026.
	c := utc(2026, 1, 5, 9)
	d := utc(2026, 1, 8, 10)
	snap := watcher.Snapshot{Commits: []watcher.CommitView{commit("abc", c, d)}}

	got := tui.WeeklyStats(snap)
	if len(got) != 1 {
		t.Fatalf("expected 1 week stat, got %d: %+v", len(got), got)
	}
	w := got[0]
	if w.Year != 2026 || w.Week != 2 {
		t.Errorf("expected 2026 week 2, got %d week %d", w.Year, w.Week)
	}
	if w.Deploys != 1 {
		t.Errorf("expected 1 deploy, got %d", w.Deploys)
	}
	if w.AvgLead != d.Sub(c) {
		t.Errorf("expected lead %v, got %v", d.Sub(c), w.AvgLead)
	}
}

func TestWeeklyStats_MultipleDeploys_SameWeek_AveragesLeadTime(t *testing.T) {
	// Two separate deploys in the same week — same week bucket, deploy count
	// counts batches (2), lead time averages across both commits.
	c1 := utc(2026, 1, 5, 9)
	d1 := utc(2026, 1, 5, 10)  // 1h lead
	c2 := utc(2026, 1, 6, 9)
	d2 := utc(2026, 1, 6, 12)  // 3h lead
	snap := watcher.Snapshot{Commits: []watcher.CommitView{
		commit("c2", c2, d2), // newest first in snapshot order
		commit("c1", c1, d1),
	}}

	got := tui.WeeklyStats(snap)
	if len(got) != 1 {
		t.Fatalf("expected 1 week, got %d", len(got))
	}
	w := got[0]
	if w.Deploys != 2 {
		t.Errorf("expected 2 deploys, got %d", w.Deploys)
	}
	wantAvg := 2 * time.Hour
	if w.AvgLead != wantAvg {
		t.Errorf("expected avg lead %v, got %v", wantAvg, w.AvgLead)
	}
}

func TestWeeklyStats_BatchWithMultipleCommits_OneDeploy_AvgAcrossCommits(t *testing.T) {
	// In clarity's model a "batch deploy" is represented by the NEWEST commit
	// of the batch holding the deploy:passed event; older commits in the
	// batch have no deploy events of their own — computeBatches walks
	// newest-first and joins event-less older commits to the current batch.
	// So: one explicit deploy at time d, two commits in the batch.
	cNewer := utc(2026, 1, 5, 10) // 2h lead
	cOlder := utc(2026, 1, 5, 9)  // 3h lead
	d := utc(2026, 1, 5, 12)
	snap := watcher.Snapshot{Commits: []watcher.CommitView{
		commit("newer", cNewer, d),
		{SHA: "older", Time: cOlder}, // no deploy event — rides along
	}}

	got := tui.WeeklyStats(snap)
	if len(got) != 1 {
		t.Fatalf("expected 1 week, got %d", len(got))
	}
	w := got[0]
	if w.Deploys != 1 {
		t.Errorf("expected 1 deploy batch (not 2 commits), got %d", w.Deploys)
	}
	wantAvg := 150 * time.Minute
	if w.AvgLead != wantAvg {
		t.Errorf("expected avg lead %v, got %v", wantAvg, w.AvgLead)
	}
}

func TestWeeklyStats_AcrossWeeks_ProducesMultipleBuckets(t *testing.T) {
	// Two deploys in different ISO weeks of 2026.
	// 2026-01-08 = week 2, 2026-01-15 = week 3.
	c1 := utc(2026, 1, 5, 9)
	d1 := utc(2026, 1, 8, 10)
	c2 := utc(2026, 1, 12, 9)
	d2 := utc(2026, 1, 15, 10)
	snap := watcher.Snapshot{Commits: []watcher.CommitView{
		commit("c2", c2, d2),
		commit("c1", c1, d1),
	}}

	got := tui.WeeklyStats(snap)
	if len(got) != 2 {
		t.Fatalf("expected 2 weeks, got %d: %+v", len(got), got)
	}
	// Newest-first: week 3 then week 2.
	if got[0].Week != 3 || got[1].Week != 2 {
		t.Errorf("expected newest-first ordering (3, 2), got (%d, %d)", got[0].Week, got[1].Week)
	}
	for _, w := range got {
		if w.Deploys != 1 {
			t.Errorf("expected each week to have 1 deploy, got %d for week %d", w.Deploys, w.Week)
		}
	}
}

func TestWeeklyStats_OlderBatchWithoutRecordedDeploy_StillContributesToAvg(t *testing.T) {
	// Real-world case: an OLDER batch's deploy:passed event was never
	// recorded (data gap). Its commits sit in the Deployed section below a
	// newer passed deploy, inherit that deploy's time via fix-forward, so
	// their lead times are FROZEN and visible. They should count toward the
	// same week's average. "Deploys" count stays strict at 1 — only the
	// actually-recorded passed batch is a deploy.
	cNewer := utc(2026, 1, 5, 9)
	dNewer := utc(2026, 1, 5, 10)   // newest passed deploy, week 2
	cOlder := utc(2026, 1, 1, 0)    // 4 days earlier than dNewer
	snap := watcher.Snapshot{Commits: []watcher.CommitView{
		commit("newer", cNewer, dNewer),
		// Older: deploy:started exists but no recorded terminal. Inherits
		// dNewer via fix-forward, so its frozen lead time is dNewer-cOlder.
		{SHA: "older", Time: cOlder, Events: []clarityrefs.Event{
			{Stage: "ci", Status: "passed", Time: cOlder.Add(time.Hour)},
			{Stage: "deploy", Status: "started", Time: cOlder.Add(2 * time.Hour)},
		}},
	}}

	got := tui.WeeklyStats(snap)
	if len(got) != 1 {
		t.Fatalf("expected 1 week (older commit inherits newer's deploy week), got %d: %+v", len(got), got)
	}
	w := got[0]
	if w.Deploys != 1 {
		t.Errorf("expected Deploys=1 (started batch is not a recorded deploy), got %d", w.Deploys)
	}
	wantAvg := (dNewer.Sub(cNewer) + dNewer.Sub(cOlder)) / 2
	if w.AvgLead != wantAvg {
		t.Errorf("expected avg lead %v (includes inherited frozen lead), got %v", wantAvg, w.AvgLead)
	}
}

func TestWeeklyStats_ISOWeekBoundary(t *testing.T) {
	// 2026-01-04 is Sunday, still ISO week 1 of 2026 (Mon-start).
	// 2026-01-05 is Monday, ISO week 2 of 2026.
	// Verify a deploy on each side falls in the right bucket.
	c1 := utc(2026, 1, 1, 9)
	d1 := utc(2026, 1, 4, 23) // Sunday of week 1
	c2 := utc(2026, 1, 5, 9)
	d2 := utc(2026, 1, 5, 10) // Monday of week 2
	snap := watcher.Snapshot{Commits: []watcher.CommitView{
		commit("c2", c2, d2),
		commit("c1", c1, d1),
	}}

	got := tui.WeeklyStats(snap)
	if len(got) != 2 {
		t.Fatalf("expected 2 weeks across the boundary, got %d", len(got))
	}
	if got[0].Week != 2 || got[1].Week != 1 {
		t.Errorf("expected weeks (2, 1) newest-first, got (%d, %d)", got[0].Week, got[1].Week)
	}
}
