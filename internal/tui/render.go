// Package tui renders watcher snapshots to the terminal. The package is split
// into pure render functions (this file) and a thin Bubble Tea program
// (program.go) so the visuals are unit-testable without spinning up a TTY.
package tui

import (
	"fmt"
	"sort"
	"strings"

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
)

// RenderRow renders one commit row: <icon>  <author>  <subject>  <stages>
func RenderRow(view watcher.CommitView, width int) string {
	icon := iconFor(OverallStatus(view.Events))
	author := lipgloss.NewStyle().Foreground(colorGray).Render(view.Author)
	subject := view.Subject
	stages := renderStages(view.Events)
	return fmt.Sprintf("  %s  %s  %s  %s", icon, author, subject, stages)
}

// RenderSnapshot renders all commits in the snapshot, newest-first (already
// sorted by the watcher), one row per commit.
func RenderSnapshot(snap watcher.Snapshot, width int) string {
	if len(snap.Commits) == 0 {
		return "  (no commits yet)\n"
	}
	var b strings.Builder
	for _, c := range snap.Commits {
		b.WriteString(RenderRow(c, width))
		b.WriteString("\n")
	}
	return b.String()
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

func renderStages(events []clarityrefs.Event) string {
	if len(events) == 0 {
		return lipgloss.NewStyle().Foreground(colorGray).Render("(no events)")
	}
	stages := CollapseStages(events)
	parts := make([]string, 0, len(stages))
	for _, s := range stages {
		parts = append(parts, formatStage(s))
	}
	return strings.Join(parts, " ")
}

func formatStage(s StageStatus) string {
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
