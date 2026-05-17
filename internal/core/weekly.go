package core

import (
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
	g := GroupCommits(snap.Commits)

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

	for i, c := range snap.Commits {
		deployedAt := g.DeployedAtIndex(i)
		if deployedAt.IsZero() || c.Time.IsZero() {
			continue
		}
		year, week := deployedAt.UTC().ISOWeek()
		bk := getOrCreate(int64(year)*100 + int64(week))
		bk.totalLeadNs += int64(deployedAt.Sub(c.Time))
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
