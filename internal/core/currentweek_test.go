package core_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
)

// deployedAt builds a commit deployed at ts.
func deployedAt(sha string, authored, deployed int64) core.CommitView {
	return core.CommitView{
		SHA: sha, Subject: sha, Author: "a", Time: time.Unix(authored, 0),
		Events: []clarityrefs.Event{
			{Stage: "ci", Status: "passed", Time: time.Unix(authored+60, 0)},
			{Stage: "deploy", Status: "passed", Time: time.Unix(deployed, 0)},
		},
	}
}

// TestCurrentWeekStat — the stats merged onto the "Deployed" header sit in the
// headline position, which reads as the current state. Merging whatever week
// happened to be newest meant a repo that last shipped a week ago showed "36
// deploys" up there, and at a glance that is this week's velocity.
func TestCurrentWeekStat(t *testing.T) {
	// 2026-09-21 is a Monday, ISO week 39. A week earlier is week 38.
	thisWeek := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	lastWeek := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		deployed time.Time
		want     bool
	}{
		{name: "deployed this week", deployed: thisWeek, want: true},
		{name: "last deployed a week ago", deployed: lastWeek, want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			snap := core.Snapshot{Commits: []core.CommitView{
				deployedAt("a", c.deployed.Unix()-3600, c.deployed.Unix()),
			}}
			view := core.DeriveView(snap, core.DefaultLeadTimeMode, nil)
			stats := core.IndexStatsByWeek(view.Flows[0].Weekly)

			_, _, ok := core.CurrentWeekStat(stats, now)
			if ok != c.want {
				t.Errorf("CurrentWeekStat present = %v, want %v", ok, c.want)
			}
		})
	}
}

// The current week is computed in UTC, like the buckets themselves, so the
// headline doesn't change depending on who is looking at it.
func TestCurrentWeekStat_UsesUTC(t *testing.T) {
	// 23:00 UTC on Sunday of week 39 is already Monday of week 40 in Sydney.
	deployed := time.Date(2026, 9, 27, 23, 0, 0, 0, time.UTC)
	snap := core.Snapshot{Commits: []core.CommitView{
		deployedAt("a", deployed.Unix()-3600, deployed.Unix()),
	}}
	view := core.DeriveView(snap, core.DefaultLeadTimeMode, nil)
	stats := core.IndexStatsByWeek(view.Flows[0].Weekly)

	sydney := time.FixedZone("AEST", 10*3600)
	if _, _, ok := core.CurrentWeekStat(stats, deployed.In(sydney)); !ok {
		t.Error("the headline week changed with the reader's timezone")
	}
}

// TestWeekDividerLabel_NoDeploys — a week with no deploys still names itself
// and says so. There is no average of zero deploys, so the label omits it
// rather than printing a placeholder: a dash where a number goes is a column
// spent saying nothing, and it invites being read as a very fast average.
func TestWeekDividerLabel_NoDeploys(t *testing.T) {
	label := core.WeekDividerLabel(core.WeekStat{Year: 2026, Week: 39})

	if !strings.Contains(label, "W2026-39") {
		t.Errorf("label does not name the week: %q", label)
	}
	if !strings.Contains(label, "0 deploys") {
		t.Errorf("label does not report zero deploys: %q", label)
	}
	if strings.Contains(label, "avg") {
		t.Errorf("label claims an average of no deploys: %q", label)
	}
}

// The current week is always reported, whether or not it has deploys — that is
// what makes two repos side by side comparable, and what stops an older week
// being mistaken for now.
func TestCurrentWeekStat_AlwaysDescribesTheCurrentWeek(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC) // ISO week 39
	lastWeek := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

	snap := core.Snapshot{Commits: []core.CommitView{
		deployedAt("a", lastWeek.Unix()-3600, lastWeek.Unix()),
	}}
	view := core.DeriveView(snap, core.DefaultLeadTimeMode, nil)
	stats := core.IndexStatsByWeek(view.Flows[0].Weekly)

	_, stat, hasDeploys := core.CurrentWeekStat(stats, now)
	if hasDeploys {
		t.Error("reported deploys for a week that had none")
	}
	if stat.Year != 2026 || stat.Week != 39 {
		t.Errorf("stat describes W%d-%02d, want W2026-39", stat.Year, stat.Week)
	}
	if stat.Deploys != 0 {
		t.Errorf("stat reports %d deploys, want 0", stat.Deploys)
	}
}

// TestCurrentWeekSummary covers the three answers the Deployed header can
// honestly give about the current week, and the one it must never give.
//
// The third state is the important one. A truncated window drops its oldest
// week, and when every deploy in the window falls in the current week that
// dropped bucket *is* the current week. Reporting "0 deploys" there would
// print a wrong number directly above the deploys it denies — turning "we
// deliberately don't know" into a confident zero, which is exactly what the
// truncation rule exists to prevent.
func TestCurrentWeekSummary(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC) // W39
	thisWeek := time.Date(2026, 9, 22, 9, 0, 0, 0, time.UTC)
	lastWeek := time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		deploys   []time.Time
		truncated bool
		want      core.WeekState
	}{
		{name: "deployed this week", deploys: []time.Time{thisWeek}, want: core.WeekReported},
		{name: "last deployed a week ago", deploys: []time.Time{lastWeek}, want: core.WeekEmpty},
		{name: "no deploys at all", want: core.WeekEmpty},
		{
			name:    "truncated, but an older week survives to be dropped",
			deploys: []time.Time{thisWeek, lastWeek}, truncated: true,
			want: core.WeekReported,
		},
		{
			name: "truncated and every deploy is this week — the bucket was dropped",
			// The oldest bucket is the current week, so its count is unknown
			// rather than zero.
			deploys: []time.Time{thisWeek, thisWeek.Add(-time.Hour)}, truncated: true,
			want: core.WeekUnknown,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var commits []core.CommitView
			for i, at := range c.deploys {
				commits = append(commits, deployedAt(string(rune('a'+i)), at.Add(-time.Hour).Unix(), at.Unix()))
			}
			snap := core.Snapshot{Commits: commits, Truncated: c.truncated}
			view := core.DeriveView(snap, core.DefaultLeadTimeMode, nil)
			flow := view.Flows[0]

			_, _, got := core.CurrentWeekSummary(flow.Groups, core.IndexStatsByWeek(flow.Weekly), now)
			if got != c.want {
				t.Errorf("CurrentWeekSummary state = %v, want %v", got, c.want)
			}
		})
	}
}

// A week whose deploys were later superseded by a failed redeploy keeps its
// measured lead times while losing its passed batches. Dropping the average
// there discards data that was actually observed.
func TestWeekDividerLabel_ZeroDeploysWithARealAverage(t *testing.T) {
	label := core.WeekDividerLabel(core.WeekStat{Year: 2026, Week: 39, AvgLead: 25 * time.Hour})
	if !strings.Contains(label, "avg") {
		t.Errorf("a measured average was discarded: %q", label)
	}

	bare := core.WeekDividerLabel(core.WeekStat{Year: 2026, Week: 39})
	if strings.Contains(bare, "avg") {
		t.Errorf("label claims an average it doesn't have: %q", bare)
	}
}
