package tui

import (
	"sort"
	"time"

	"github.com/ezcdlabs/clarity/internal/watcher"
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

// WeeklyStats computes per-ISO-week throughput from snap's deploy batches.
// Results are sorted newest-week-first (matching the snapshot's commit order).
// In-flight and failed batches are excluded — only deploy:passed batches
// represent "this shipped" for DORA throughput purposes.
func WeeklyStats(snap watcher.Snapshot) []WeekStat {
	g := GroupCommits(snap.Commits)

	type bucket struct {
		deploys       int
		totalLeadNs   int64
		leadCommitCnt int
	}
	byWeek := map[int64]*bucket{}
	var keys []int64

	for _, b := range g.Deployed {
		if b.Status != "passed" {
			continue
		}
		year, week := b.Time.UTC().ISOWeek()
		// ISO year and week each fit comfortably in 32 bits; combining them
		// into one int64 key gives a stable, sortable identifier without
		// allocating per-bucket struct keys.
		key := int64(year)*100 + int64(week)
		bk, ok := byWeek[key]
		if !ok {
			bk = &bucket{}
			byWeek[key] = bk
			keys = append(keys, key)
		}
		bk.deploys++
		for _, c := range b.Commits {
			if c.Time.IsZero() {
				continue
			}
			bk.totalLeadNs += int64(b.Time.Sub(c.Time))
			bk.leadCommitCnt++
		}
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
