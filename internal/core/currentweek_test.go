package core_test

import (
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
