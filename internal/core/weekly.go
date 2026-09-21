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
	return weeklyStats(snap, GroupCommitsMode(snap.Commits, mode))
}

// WeeklyStatsForFlow is WeeklyStatsMode with candidacy applied, so a flow's
// throughput average is built only from commits that flow actually ships.
func WeeklyStatsForFlow(snap Snapshot, mode LeadTimeMode, f Flow) []WeekStat {
	return weeklyStats(snap, GroupCommitsForFlow(snap.Commits, mode, f))
}

func weeklyStats(snap Snapshot, g Groupings) []WeekStat {

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

	// A truncated window cuts through its oldest week, so that bucket is
	// missing however many deploys fell below the limit. Drop it rather
	// than report a count that is wrong in a way the reader cannot see —
	// both renderers already treat a missing bucket as "render no divider".
	// Newer buckets are kept: they can only be short by a commit authored
	// before the cut but deployed inside the window, which is the rare
	// long-lead case, and --limit 0 is the answer to that.
	if snap.Truncated && len(keys) > 0 {
		keys = keys[:len(keys)-1]
	}

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

// WeekDividerLabel formats a WeekStat as the inline text for a divider
// ("W<year>-<NN>  N deploys  Xh Ym avg"). Shared by both renderers so the
// format stays in one place — the TUI styles around it, the plain renderer
// emits it bare.
func WeekDividerLabel(s WeekStat) string {
	deploysLabel := "deploys"
	if s.Deploys == 1 {
		deploysLabel = "deploy"
	}
	// Nothing measured means no average to report. A placeholder in that slot
	// is a column spent saying nothing, and "0s avg" is worse than nothing —
	// it reads as an extraordinarily fast week rather than an empty one.
	//
	// The test is the average, not the count: a week can lose its passed
	// batches to a later failed redeploy while keeping the lead times it
	// actually measured, and discarding those would throw away real data.
	//
	// A single space, unusually, so the label is one unit. The divider sheds
	// whole facts from the left when narrow, and a shed week number would
	// leave a bare "0 deploys" sitting above another week's rows, reading as
	// a statement about the section rather than about this week.
	if s.AvgLead == 0 {
		return fmt.Sprintf("W%d-%02d %d %s", s.Year, s.Week, s.Deploys, deploysLabel)
	}
	return fmt.Sprintf("W%d-%02d  %d %s  %s avg",
		s.Year, s.Week, s.Deploys, deploysLabel, FormatElapsed(s.AvgLead))
}

// LimitNoticeLabel formats the note that closes a truncated commit list
// ("--limit 100 reached · raise it, or --limit 0 for all commits"). Shared
// by both renderers so the wording stays in one place, the same way
// WeekDividerLabel is.
//
// It names the active limit because that is the number the reader has to
// change, and it names --limit 0 because "raise it" alone leaves them
// guessing at how high. Without this line the bottom of a cut list is
// indistinguishable from the start of the repository — which is the reading
// that turns a deploy below the cut into a deploy that never happened.
func LimitNoticeLabel(limit int) string {
	return fmt.Sprintf("--limit %d reached · raise it, or --limit 0 for all commits", limit)
}

// CurrentWeekStat returns the stats for the ISO week containing now, if that
// week has any deploys.
//
// It replaces "whatever week is newest" for the Deployed section's header,
// because that header sits in the headline position and reads as the current
// state. A repo whose last deploy was a week ago showed last week's totals
// there — "36 deploys" where a glance takes it for this week's velocity. When
// the current week is empty the header carries no stats at all and the week
// below gets its own divider, which says plainly that it is not this week.
//
// The week is computed in UTC, like the buckets themselves, so the headline
// doesn't depend on who is reading it.
func CurrentWeekStat(statsByWeek map[int64]WeekStat, now time.Time) (int64, WeekStat, bool) {
	year, week := now.UTC().ISOWeek()
	key := WeekKey(year, week)
	if s, ok := statsByWeek[key]; ok {
		return key, s, true
	}
	// The current week is reported whether or not it has deploys: a header
	// that names this week and says zero is what makes two repos comparable at
	// a glance, and it leaves no headline slot for an older week to occupy.
	return key, WeekStat{Year: year, Week: week}, false
}

// WeekState is what can honestly be said about a week's throughput.
type WeekState int

const (
	// WeekReported: the week has a counted bucket.
	WeekReported WeekState = iota
	// WeekEmpty: the week is in the window and nothing deployed in it.
	WeekEmpty
	// WeekUnknown: the week's bucket was dropped by truncation, so its count
	// is unknown rather than zero. Saying "0 deploys" here would print a wrong
	// number above the very deploys it denies.
	WeekUnknown
)

// CurrentWeekSummary describes the current week for the Deployed header.
//
// A truncated window drops its oldest week, because a count cut through by the
// limit understates in a way the reader cannot see. When every deploy in the
// window falls in the current week, that dropped bucket *is* the current week
// — so an absent bucket means "unknown", not "none", whenever the section
// still shows deploys from it.
func CurrentWeekSummary(g Groupings, statsByWeek map[int64]WeekStat, now time.Time) (int64, WeekStat, WeekState) {
	key, stat, reported := CurrentWeekStat(statsByWeek, now)
	switch {
	case reported:
		return key, stat, WeekReported
	case currentWeekHasBatch(g, key):
		return key, stat, WeekUnknown
	default:
		return key, stat, WeekEmpty
	}
}

// CurrentWeekHasDeploys reports whether the Deployed section will render a
// batch belonging to the current week directly below its header.
func CurrentWeekHasDeploys(g Groupings, now time.Time) bool {
	year, week := now.UTC().ISOWeek()
	return currentWeekHasBatch(g, WeekKey(year, week))
}

func currentWeekHasBatch(g Groupings, key int64) bool {
	for _, b := range g.Deployed {
		if b.Status != "passed" {
			continue
		}
		year, week := b.Time.UTC().ISOWeek()
		if WeekKey(year, week) == key {
			return true
		}
	}
	return false
}
