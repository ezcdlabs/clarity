package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

// PlainOptions configures RenderPlain. ShowSHAs surfaces a short commit hash
// per row (off by default to mirror the TUI's commit-less layout). Limit caps
// the number of commits rendered; zero means no cap.
type PlainOptions struct {
	ShowSHAs bool
	Limit    int
}

// RenderPlain produces a static, ANSI-free snapshot of the same view the TUI
// shows: a one-line status header, then HEAD / CI Passed / Deployed sections
// matching the TUI's groupings and batch subheaders. Aimed at piped/agent
// consumers — the row vocabulary (✓ ✗ … plus section names) is greppable
// without needing key:value annotations.
func RenderPlain(repoName string, snap watcher.Snapshot, now time.Time, opts PlainOptions) string {
	commits := snap.Commits
	if opts.Limit > 0 && len(commits) > opts.Limit {
		commits = commits[:opts.Limit]
	}
	capped := watcher.Snapshot{Commits: commits}

	g := GroupCommits(capped.Commits)
	indexBySHA := make(map[string]int, len(capped.Commits))
	for i, c := range capped.Commits {
		indexBySHA[c.SHA] = i
	}

	var b strings.Builder
	b.WriteString(plainHeader(repoName, capped))
	b.WriteString("\n\n")

	writeSection := func(label string, commits []watcher.CommitView) {
		b.WriteString(label)
		b.WriteString("\n")
		for _, c := range commits {
			b.WriteString(plainRow(c, &g, indexBySHA[c.SHA], now, opts))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	writeSection("HEAD", g.Head)

	b.WriteString("CI Passed")
	b.WriteString("\n")
	for _, c := range g.CIPassed {
		b.WriteString(plainRow(c, &g, indexBySHA[c.SHA], now, opts))
		b.WriteString("\n")
	}
	for _, batch := range g.InFlight {
		b.WriteString(plainBatchSubheader(batch, now, false))
		for _, c := range batch.Commits {
			b.WriteString(plainRow(c, &g, indexBySHA[c.SHA], now, opts))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")

	statsByWeek := indexStatsByWeek(WeeklyStats(capped))
	topWeekKey, topWeekStat, hasTopWeek := firstPassedWeekStat(g.Deployed, statsByWeek)
	if hasTopWeek {
		// Merge the topmost week's summary onto the section header row so we
		// don't burn a line on a divider that's about to be followed by the
		// batch subheader for the same week.
		b.WriteString("Deployed  ·  ")
		b.WriteString(weekDividerLabel(topWeekStat))
		b.WriteString("\n")
	} else {
		b.WriteString("Deployed\n")
	}
	prevWeekKey := int64(-1)
	if hasTopWeek {
		prevWeekKey = topWeekKey
	}
	for i, batch := range g.Deployed {
		if batch.Status == "passed" {
			year, week := batch.Time.UTC().ISOWeek()
			key := weekKey(year, week)
			if key != prevWeekKey {
				if s, ok := statsByWeek[key]; ok {
					b.WriteString(weekDividerLabel(s))
					b.WriteString("\n")
				}
				prevWeekKey = key
			}
		}
		b.WriteString(plainBatchSubheader(batch, now, i == 0))
		for _, c := range batch.Commits {
			b.WriteString(plainRow(c, &g, indexBySHA[c.SHA], now, opts))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

// indexStatsByWeek builds a lookup map keyed by weekKey(year, week) so the
// renderer can find a week's stats in O(1) while walking batches.
func indexStatsByWeek(stats []WeekStat) map[int64]WeekStat {
	out := make(map[int64]WeekStat, len(stats))
	for _, s := range stats {
		out[weekKey(s.Year, s.Week)] = s
	}
	return out
}

// weekKey is the same int64 packing WeeklyStats uses internally — kept in
// one place so the renderer and the stats computation agree.
func weekKey(year, week int) int64 { return int64(year)*100 + int64(week) }

// plainHeader produces the one-line status: "<repo>  ci: <icon> <state>  deploy: <icon> <state>".
// Mirrors the TUI's header semantics: "started" and "skipped" events are
// ignored so the badge holds its colour across transient retries.
func plainHeader(repoName string, snap watcher.Snapshot) string {
	ci := plainBadge(currentStageStatus(snap.Commits, "ci"))
	deploy := plainBadge(currentStageStatus(snap.Commits, "deploy"))
	return fmt.Sprintf("%s  ci: %s  deploy: %s", repoName, ci, deploy)
}

func plainBadge(status string) string {
	switch status {
	case "passed":
		return "✓ passed"
	case "failed":
		return "✗ failed"
	default:
		return "· none"
	}
}

func plainBatchSubheader(b DeployBatch, now time.Time, isLive bool) string {
	switch b.Status {
	case "started":
		return "  … deploying\n"
	case "passed":
		ago := ""
		if !now.IsZero() && !b.Time.IsZero() {
			ago = " " + formatElapsed(now.Sub(b.Time)) + " ago"
		}
		if isLive {
			return "  deployed" + ago + "  (live)\n"
		}
		return "  deployed" + ago + "\n"
	case "failed":
		ago := ""
		if !now.IsZero() && !b.Time.IsZero() {
			ago = " " + formatElapsed(now.Sub(b.Time)) + " ago"
		}
		return "  deploy failed" + ago + "\n"
	default:
		return ""
	}
}

func plainRow(view watcher.CommitView, group *Groupings, index int, now time.Time, opts PlainOptions) string {
	icon := plainCIIcon(view.Events)
	var b strings.Builder
	b.WriteString("  ")
	b.WriteString(icon)
	if opts.ShowSHAs {
		b.WriteString("  ")
		b.WriteString(shortSHA(view.SHA))
	}
	b.WriteString("  ")
	b.WriteString(view.Author)
	b.WriteString("  ")
	b.WriteString(view.Subject)

	if group != nil {
		if d, _, ok := group.LeadTime(index, view.Time, now); ok {
			b.WriteString("  ")
			b.WriteString(formatElapsed(d))
		}
	}
	return b.String()
}

func plainCIIcon(events []clarityrefs.Event) string {
	switch ciStatus(events) {
	case "passed":
		return "✓"
	case "failed":
		return "✗"
	case "started":
		// Static glyph: the TUI's spinner can't animate in a one-shot print.
		return "…"
	default:
		return "·"
	}
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
