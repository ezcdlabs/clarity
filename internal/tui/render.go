// Package tui renders watcher snapshots to the terminal. The package is split
// into pure render functions (this file) and a thin Bubble Tea program
// (program.go) so the visuals are unit-testable without spinning up a TTY.
package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

// StageStatus is the most recent status for one stage on one commit.
type StageStatus struct {
	Stage  string
	Status string
	Time   timeRef
}

// timeRef is a minimal time wrapper so callers don't need to import time
// just to construct test data — they can build it via CollapseStages.
type timeRef struct{ unix int64 }

// CollapseStages returns the latest status per stage from the given event
// stream, in the order each stage's latest event was observed (chronological).
// This is the "render-time status collapse" described in README.md.
func CollapseStages(events []clarityrefs.Event) []StageStatus {
	latestByStage := map[string]clarityrefs.Event{}
	firstSeen := map[string]int64{}
	for _, e := range events {
		ts := e.Time.Unix()
		if cur, ok := latestByStage[e.Stage]; !ok || e.Time.After(cur.Time) {
			latestByStage[e.Stage] = e
		}
		if _, ok := firstSeen[e.Stage]; !ok {
			firstSeen[e.Stage] = ts
		}
	}

	out := make([]StageStatus, 0, len(latestByStage))
	for stage, e := range latestByStage {
		out = append(out, StageStatus{
			Stage:  stage,
			Status: e.Status,
			Time:   timeRef{unix: e.Time.Unix()},
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return firstSeen[out[i].Stage] < firstSeen[out[j].Stage]
	})
	return out
}

// OverallStatus reduces a commit's events to one of:
//   - "none"    no events
//   - "passed"  every stage's latest is passed or skipped
//   - "failed"  any stage's latest is failed
//   - "running" any stage's latest is started (and none failed)
//
// "failed" wins over "running"; "running" wins over "passed".
func OverallStatus(events []clarityrefs.Event) string {
	if len(events) == 0 {
		return "none"
	}
	stages := CollapseStages(events)
	hasRunning := false
	for _, s := range stages {
		switch s.Status {
		case "failed":
			return "failed"
		case "started":
			hasRunning = true
		}
	}
	if hasRunning {
		return "running"
	}
	return "passed"
}

// --- rendering ---------------------------------------------------------------

var (
	colorGreen = lipgloss.Color("2")
	colorRed   = lipgloss.Color("1")
	colorAmber = lipgloss.Color("3")
	colorGray  = lipgloss.Color("8")
	colorBlue  = lipgloss.Color("12")
)

// RenderRow renders one commit row using only that commit's own events —
// no fix-forward awareness, no timer. Used for standalone tests; section
// rendering goes through renderRowInGroup.
func RenderRow(view watcher.CommitView, width int) string {
	return renderRowInGroup(view, nil, 0, width, time.Time{})
}

// RenderSnapshot renders the snapshot grouped by lifecycle stage:
// NeedsCI → NextDeploy → Deployed. Empty sections are hidden so the
// active drama is concentrated. Stale stage icons (where a newer commit
// has already succeeded on the same stage) render in muted gray. A
// right-aligned lead-time timer ticks on each row in blue while the commit
// is still in flight, and freezes in gray once it reaches production.
//
// now drives the live half of the timer; pass time.Time{} for tests that
// don't care about timer values (timers won't render for commits with no
// Time set anyway).
func RenderSnapshot(snap watcher.Snapshot, width int, now time.Time) string {
	if len(snap.Commits) == 0 {
		return "  (no commits yet)\n"
	}

	g := GroupCommits(snap.Commits)
	indexBySHA := make(map[string]int, len(snap.Commits))
	for i, c := range snap.Commits {
		indexBySHA[c.SHA] = i
	}

	var b strings.Builder
	writeSection := func(title string, commits []watcher.CommitView) {
		if len(commits) == 0 {
			return
		}
		b.WriteString(renderSectionHeader(title))
		for _, c := range commits {
			b.WriteString(renderRowInGroup(c, &g, indexBySHA[c.SHA], width, now))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	writeSection("On main · needs CI", g.NeedsCI)

	nextTitle := "Next deploy"
	if g.Deploying {
		nextTitle = "Next deploy · deploying…"
	}
	writeSection(nextTitle, g.NextDeploy)

	writeSection("Deployed to production", g.Deployed)

	return b.String()
}

func renderSectionHeader(title string) string {
	return lipgloss.NewStyle().Bold(true).Render(title) + "\n"
}

// renderRowInGroup renders one commit row, optionally with fix-forward
// awareness (group != nil) and an elapsed lead-time timer. When width > 0
// the timer is right-aligned to that column.
func renderRowInGroup(view watcher.CommitView, group *Groupings, index int, width int, now time.Time) string {
	icon := iconFor(OverallStatus(view.Events))
	author := lipgloss.NewStyle().Foreground(colorGray).Render(view.Author)
	subject := view.Subject
	stages := renderStagesInGroup(view.Events, group, index)
	left := fmt.Sprintf("  %s  %s  %s  %s", icon, author, subject, stages)

	timer := ""
	if group != nil {
		if d, frozen, ok := group.LeadTime(index, view.Time, now); ok {
			color := colorBlue
			if frozen {
				color = colorGray
			}
			timer = lipgloss.NewStyle().Foreground(color).Render(formatElapsed(d))
		}
	}

	if timer == "" {
		return left
	}
	if width <= 0 {
		return left + "  " + timer
	}
	pad := width - lipgloss.Width(left) - lipgloss.Width(timer)
	if pad < 2 {
		pad = 2
	}
	return left + strings.Repeat(" ", pad) + timer
}

func iconFor(status string) string {
	switch status {
	case "passed":
		return lipgloss.NewStyle().Foreground(colorGreen).Render("✓")
	case "failed":
		return lipgloss.NewStyle().Foreground(colorRed).Render("✗")
	case "running":
		return lipgloss.NewStyle().Foreground(colorAmber).Render("⧗")
	default:
		return lipgloss.NewStyle().Foreground(colorGray).Render("·")
	}
}

func renderStagesInGroup(events []clarityrefs.Event, group *Groupings, index int) string {
	if len(events) == 0 {
		return lipgloss.NewStyle().Foreground(colorGray).Render("(no events)")
	}
	stages := CollapseStages(events)
	parts := make([]string, 0, len(stages))
	for _, s := range stages {
		stale := group != nil && group.IsStaleStage(index, s.Stage)
		parts = append(parts, formatStage(s, stale))
	}
	return strings.Join(parts, " ")
}

func formatStage(s StageStatus, stale bool) string {
	if stale {
		// stale: keep the original word/icon but mute the colour so it reads
		// as settled history rather than active state.
		return lipgloss.NewStyle().Foreground(colorGray).Render(stageLabel(s))
	}
	switch s.Status {
	case "passed":
		return lipgloss.NewStyle().Foreground(colorGreen).Render(s.Stage)
	case "failed":
		return lipgloss.NewStyle().Foreground(colorRed).Render(s.Stage + " failed")
	case "started":
		return lipgloss.NewStyle().Foreground(colorAmber).Render(s.Stage + "…")
	case "skipped":
		return lipgloss.NewStyle().Foreground(colorGray).Render(s.Stage + " skipped")
	default:
		return lipgloss.NewStyle().Foreground(colorGray).Render(s.Stage + " " + s.Status)
	}
}

// stageLabel is the uncoloured label for a stage status — used by the stale
// branch of formatStage to keep the wording stable while only colour changes.
func stageLabel(s StageStatus) string {
	switch s.Status {
	case "failed":
		return s.Stage + " failed"
	case "started":
		return s.Stage + "…"
	case "skipped":
		return s.Stage + " skipped"
	case "passed":
		return s.Stage
	default:
		return s.Stage + " " + s.Status
	}
}
