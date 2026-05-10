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

// The palette is deliberately minimal: gray is the neutral foreground for
// everything routine; red is the only "something is broken" colour; green is
// reserved for the *summary* badges in the header where a binary "is the
// pipeline green?" answer earns its colour; yellow and blue are lifecycle
// accents on the section dividers (see renderSectionDivider).
//
// Per-row status icons carry their meaning via shape (✓ / ✗ / spinner / ·),
// not hue — colour reinforces only the cases where the user needs to notice.
var (
	colorRed    = lipgloss.Color("1")
	colorGreen  = lipgloss.Color("2")
	colorGray   = lipgloss.Color("8")
	colorBlue   = lipgloss.Color("12")
	colorYellow = lipgloss.AdaptiveColor{Light: "3", Dark: "11"} // CI Passed divider accent
)

// SpinnerFrames is the same braille animation pushq uses for its spinner —
// works in any terminal that can render those code points.
var SpinnerFrames = []string{"⠴", "⠦", "⠧", "⠇", "⠏", "⠋", "⠙", "⠹", "⠸", "⠼"}

// spinnerFrame returns the frame at the given animation index (wraps around).
func spinnerFrame(idx int) string {
	if len(SpinnerFrames) == 0 {
		return ""
	}
	return SpinnerFrames[((idx%len(SpinnerFrames))+len(SpinnerFrames))%len(SpinnerFrames)]
}

// RenderRow renders one commit row using only that commit's own events —
// no fix-forward awareness, no timer. Used for standalone tests; section
// rendering goes through renderRowInGroup.
func RenderRow(view watcher.CommitView, width int) string {
	return renderRowInGroup(view, nil, 0, width, time.Time{}, 0)
}

// RenderSnapshot renders the snapshot grouped by lifecycle stage:
// HEAD → CI Passed → Deployed (with per-batch subheaders). All three section
// dividers are persistent, even when their section has no commits — the
// dividers act as a structural frame for the lifecycle, not a list of
// only-currently-active groups. Stale build icons (where a newer commit has
// already passed) render in muted gray. A right-aligned lead-time timer ticks
// on each row in blue while the commit is still in flight, and freezes in
// gray once it reaches production.
//
// now drives the live half of the timer; pass time.Time{} for tests that
// don't care about timer values (timers won't render for commits with no
// Time set anyway).
func RenderSnapshot(snap watcher.Snapshot, width int, now time.Time, spinnerIdx int) string {
	g := GroupCommits(snap.Commits)
	indexBySHA := make(map[string]int, len(snap.Commits))
	for i, c := range snap.Commits {
		indexBySHA[c.SHA] = i
	}

	var b strings.Builder
	writeFlat := func(title string, color lipgloss.TerminalColor, commits []watcher.CommitView) {
		b.WriteString(renderSectionDivider(title, color, width))
		for _, c := range commits {
			b.WriteString(renderRowInGroup(c, &g, indexBySHA[c.SHA], width, now, spinnerIdx))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// HEAD: no lifecycle tint — just-landed commits are visually "neutral".
	writeFlat("HEAD", nil, g.Head)

	// CI Passed: yellow lifecycle accent. Idle commits at the top, then
	// in-flight deploy batches (deploying… or stuck-failed) at the bottom.
	b.WriteString(renderSectionDivider("CI Passed", colorYellow, width))
	for _, c := range g.CIPassed {
		b.WriteString(renderRowInGroup(c, &g, indexBySHA[c.SHA], width, now, spinnerIdx))
		b.WriteString("\n")
	}
	for _, batch := range g.InFlight {
		b.WriteString(renderBatchSubheader(batch, now, spinnerIdx))
		for _, c := range batch.Commits {
			b.WriteString(renderRowInGroup(c, &g, indexBySHA[c.SHA], width, now, spinnerIdx))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")

	// Deployed: blue lifecycle accent. Only completed (deploy:passed) batches.
	b.WriteString(renderSectionDivider("Deployed", colorBlue, width))
	for _, batch := range g.Deployed {
		b.WriteString(renderBatchSubheader(batch, now, spinnerIdx))
		for _, c := range batch.Commits {
			b.WriteString(renderRowInGroup(c, &g, indexBySHA[c.SHA], width, now, spinnerIdx))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

// rowAuthorColumn is the visible column at which renderRowInGroup places the
// author name (after "  <icon>  "). Section dividers indent their label to
// the same column so that label and author texts share a left edge.
const rowAuthorColumn = 5

// renderSectionDivider produces a horizontal-rule-with-inline-label line:
// gray dashes leading up to the author column, then the bold label, then a
// space and gray dashes filling to the terminal width. The label sits at
// the same left edge as the rows beneath it. When color is non-nil the
// label is tinted (used for the lifecycle accents — yellow on CI Passed,
// blue on Deployed); HEAD passes nil so the label stays in the default
// foreground for a "neutral / just landed" feel that works on both light
// and dark terminals without an AdaptiveColor dance.
func renderSectionDivider(label string, color lipgloss.TerminalColor, width int) string {
	dashStyle := lipgloss.NewStyle().Foreground(colorGray)
	leading := dashStyle.Render(strings.Repeat("─", rowAuthorColumn))
	spacedLabel := label + " "
	labelStyle := lipgloss.NewStyle().Bold(true)
	if color != nil {
		labelStyle = labelStyle.Foreground(color)
	}
	boldLabel := labelStyle.Render(spacedLabel)

	used := rowAuthorColumn + lipgloss.Width(spacedLabel)
	if width <= used {
		return leading + boldLabel + "\n"
	}
	return leading + boldLabel + dashStyle.Render(strings.Repeat("─", width-used)) + "\n"
}

// renderBatchSubheader produces a one-line subheader inside the Deployed
// section: "deploying…" (with spinner) for in-flight batches, "deployed Xm
// ago" for completed batches, or "deploy failed Xm ago" for stuck-failed
// batches.
func renderBatchSubheader(b DeployBatch, now time.Time, spinnerIdx int) string {
	style := lipgloss.NewStyle().Foreground(colorGray).Italic(true)
	switch b.Status {
	case "started":
		spin := lipgloss.NewStyle().Foreground(colorGray).Render(spinnerFrame(spinnerIdx))
		return "  " + spin + " " + style.Render("deploying…") + "\n"
	case "passed":
		// Tinted blue to echo the Deployed section's lifecycle accent —
		// the "this batch is settled, in production" subheader carries the
		// same colour as the section divider above it.
		ago := ""
		if !now.IsZero() && !b.Time.IsZero() {
			ago = " " + formatElapsed(now.Sub(b.Time)) + " ago"
		}
		return lipgloss.NewStyle().Foreground(colorBlue).Italic(true).
			Render("  deployed"+ago) + "\n"
	case "failed":
		ago := ""
		if !now.IsZero() && !b.Time.IsZero() {
			ago = " " + formatElapsed(now.Sub(b.Time)) + " ago"
		}
		return lipgloss.NewStyle().Foreground(colorRed).Italic(true).
			Render("  deploy failed"+ago) + "\n"
	default:
		return ""
	}
}

// renderRowInGroup renders one commit row: build-status icon (or spinner
// while the build is in flight), author, subject, and a right-aligned
// lead-time timer. The deploy status is implied by the section the row
// sits in, so it isn't rendered explicitly. When width > 0 the timer is
// right-aligned to that column.
func renderRowInGroup(view watcher.CommitView, group *Groupings, index int, width int, now time.Time, spinnerIdx int) string {
	icon := ciIcon(view.Events, group, index, spinnerIdx)
	author := lipgloss.NewStyle().Foreground(colorGray).Render(view.Author)
	subject := view.Subject
	left := fmt.Sprintf("  %s  %s  %s", icon, author, subject)

	timer := ""
	if group != nil {
		if d, frozen, ok := group.LeadTime(index, view.Time, now); ok {
			// Gray while ticking; blue once the deploy that pushed this
			// commit to production fires, so the lead-time "blooms" blue
			// exactly when it freezes — matching the Deployed section's
			// brand colour.
			color := colorGray
			if frozen {
				color = colorBlue
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

// ciIcon returns the icon representing the commit's build/CI status. The
// palette deliberately reserves red for *broken* state — a failed build
// that hasn't been fix-forwarded yet — so red carries genuine signal value
// rather than annotating routine output. Passed/started/idle all render in
// neutral gray; their meaning is carried by the icon shape (✓ / spinner /
// ·) and the section the row sits in.
func ciIcon(events []clarityrefs.Event, group *Groupings, index int, spinnerIdx int) string {
	status := ciStatus(events)
	stale := group != nil && group.IsStaleStage(index, "ci")

	color := colorGray
	glyph := "·"
	switch status {
	case "passed":
		glyph = "✓"
	case "failed":
		glyph = "✗"
		if !stale {
			color = colorRed
		}
	case "started":
		glyph = spinnerFrame(spinnerIdx)
	case "skipped":
		glyph = "·"
	}
	return lipgloss.NewStyle().Foreground(color).Render(glyph)
}

// ciStatus returns the latest build event's status, or "" if there are none.
func ciStatus(events []clarityrefs.Event) string {
	var latest clarityrefs.Event
	found := false
	for _, e := range events {
		if e.Stage != "ci" {
			continue
		}
		if !found || e.Time.After(latest.Time) {
			latest = e
			found = true
		}
	}
	if !found {
		return ""
	}
	return latest.Status
}
