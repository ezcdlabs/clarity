package core

import (
	"fmt"
	"sort"
	"time"
)

// WeekStat is one ISO week's DORA-shaped throughput summary.
//
// The bucket key is the deploy date — DORA's standard aggregation — so a
// commit authored three weeks ago and deployed today contributes to *this*
// week's lead time. "Deploys" counts distinct DeployBatch entries that
// reached production in the week, not individual commit deploy:passed
// events: a 10-commit batch is one deploy, not ten. AvgLead averages each
// commit's individual (batch.Time - commit.Time) across every commit in
// those batches.
type WeekStat struct {
	Year    int // ISO year — can differ from calendar year at week boundaries.
	Week    int // ISO week (1–53).
	Deploys int
	AvgLead time.Duration
}

// WeeklyStats computes per-ISO-week throughput from snap. Results are
// sorted newest-week-first (matching the snapshot's commit order).
//
// Lead time uses the SAME per-commit data the per-row renderer displays:
// every commit's Groupings.DeployedAtIndex (own deploy:passed time, or the
// time of the newest newer passed deploy via fix-forward inheritance) minus
// its commit time. Commits with no inherited deploy time (i.e. newer than
// every passed deploy in the snapshot — they sit above the Deployed
// section) don't contribute. This makes the weekly avg equivalent to "the
// average of every frozen lead time visible in the Deployed section for
// commits whose deploy week falls in this bucket" — including commits in
// batches whose deploy:passed event was never recorded but which inherit
// from a newer batch.
//
// Deploys counts distinct DeployBatch entries with Status == "passed". A
// started-or-failed batch is not a deploy.
func WeeklyStats(snap Snapshot) []WeekStat {
	return WeeklyStatsMode(snap, DefaultLeadTimeMode)
}

// WeeklyStatsMode is WeeklyStats under an explicit LeadTimeMode. Which
// commits contribute a lead time, and what it is measured from, come from the
// same precomputed starts the per-row renderer reads — so the average is
// always "the mean of the lead times visible in the Deployed section", under
// any mode.
func WeeklyStatsMode(snap Snapshot, mode LeadTimeMode) []WeekStat {
	g := GroupCommitsMode(snap.Commits, mode)

	type bucket struct {
		deploys       int
		totalLeadNs   int64
		leadCommitCnt int
	}
	byWeek := map[int64]*bucket{}
	var keys []int64

	getOrCreate := func(key int64) *bucket {
		bk, ok := byWeek[key]
		if !ok {
			bk = &bucket{}
			byWeek[key] = bk
			keys = append(keys, key)
		}
		return bk
	}

	for i := range snap.Commits {
		deployedAt := g.DeployedAtIndex(i)
		start := g.leadStartAt(i)
		// Zero start = the mode excludes this commit. A deploy at or before
		// the start = a non-positive interval, excluded for the same reason
		// Groupings.LeadTime refuses to render one.
		if deployedAt.IsZero() || start.IsZero() || !deployedAt.After(start) {
			continue
		}
		year, week := deployedAt.UTC().ISOWeek()
		bk := getOrCreate(int64(year)*100 + int64(week))
		bk.totalLeadNs += int64(deployedAt.Sub(start))
		bk.leadCommitCnt++
	}

	for _, b := range g.Deployed {
		if b.Status != "passed" {
			continue
		}
		year, week := b.Time.UTC().ISOWeek()
		bk := getOrCreate(int64(year)*100 + int64(week))
		bk.deploys++
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i] > keys[j] })

	out := make([]WeekStat, 0, len(keys))
	for _, k := range keys {
		year := int(k / 100)
		week := int(k % 100)
		bk := byWeek[k]
		var avg time.Duration
		if bk.leadCommitCnt > 0 {
			avg = time.Duration(bk.totalLeadNs / int64(bk.leadCommitCnt))
		}
		out = append(out, WeekStat{
			Year:    year,
			Week:    week,
			Deploys: bk.deploys,
			AvgLead: avg,
		})
	}
	return out
}

// WeekKey packs an ISO (year, week) pair into a single int64 for map keys.
// Mirrors the packing WeeklyStats uses internally so renderers and stats
// computation agree on the key shape.
func WeekKey(year, week int) int64 { return int64(year)*100 + int64(week) }

// IndexStatsByWeek builds a WeekKey → WeekStat lookup so renderers can find
// a week's stats in O(1) while walking deploy batches.
func IndexStatsByWeek(stats []WeekStat) map[int64]WeekStat {
	out := make(map[int64]WeekStat, len(stats))
	for _, s := range stats {
		out[WeekKey(s.Year, s.Week)] = s
	}
	return out
}

// FirstPassedWeekStat finds the topmost (newest) passed deploy batch and
// returns its week key, the matching WeekStat, and whether a match was
// found. Renderers use this so the Deployed section header can absorb the
// topmost week's stats into its own divider row instead of emitting a
// separate one immediately below it.
func FirstPassedWeekStat(batches []DeployBatch, statsByWeek map[int64]WeekStat) (int64, WeekStat, bool) {
	for _, batch := range batches {
		if batch.Status != "passed" {
			continue
		}
		year, week := batch.Time.UTC().ISOWeek()
		key := WeekKey(year, week)
		if s, ok := statsByWeek[key]; ok {
			return key, s, true
		}
		return 0, WeekStat{}, false
	}
	return 0, WeekStat{}, false
}

// WeekDividerLabel formats a WeekStat as the inline text for a divider
// ("W<year>-<NN>  N deploys  Xh Ym avg"). Shared by both renderers so the
// format stays in one place — the TUI styles around it, the plain renderer
// emits it bare.
func WeekDividerLabel(s WeekStat) string {
	deploysLabel := "deploys"
	if s.Deploys == 1 {
		deploysLabel = "deploy"
	}
	return fmt.Sprintf("W%d-%02d  %d %s  %s avg",
		s.Year, s.Week, s.Deploys, deploysLabel, FormatElapsed(s.AvgLead))
}
