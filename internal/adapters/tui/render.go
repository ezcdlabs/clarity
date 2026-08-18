// Package tui renders core Views to the terminal. The package is split
// into pure render functions (this file) and a thin Bubble Tea program
// (program.go) so the visuals are unit-testable without spinning up a TTY.
//
// Pure data derivation (grouping, weekly stats, stage collapse, etc.)
// lives in internal/core; this package only turns derived data into bytes.
package tui

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
)

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
	colorRed   = lipgloss.Color("1")
	colorGreen = lipgloss.Color("2")
	colorGray  = lipgloss.Color("8")
	colorBlue  = lipgloss.Color("12")
	// colorYellowLight / colorYellowDark are the two raw ANSI yellows the CI
	// Passed divider accent picks between. They're plain ANSI basic colors
	// (not wrapped) so lipgloss emits an SGR escape that the terminal themes
	// — wrapping them in an image/color.Color adapter forces a truecolor RGB
	// encoding and locks the yellow to a fixed shade across every theme.
	// colorYellow() picks one of them at call time, lazily detecting bg.
	colorYellowLight = lipgloss.Color("3")
	colorYellowDark  = lipgloss.Color("11")
)

// detectDarkBackground caches whether the terminal has a dark background.
// Detection runs at most once per process, lazily on first call — never at
// package init — so non-TUI subcommands that transitively import this
// package don't leak OSC 11 escape sequences at startup. The TUI invokes
// this once from newProgram before bubbletea claims stdin, so the response
// can be read cleanly. HasDarkBackground defaults to true on error or
// non-TTY contexts, which is safe in tests and pipelines.
var (
	hasDarkBg     bool
	hasDarkBgOnce sync.Once
)

func detectDarkBackground() bool {
	hasDarkBgOnce.Do(func() {
		hasDarkBg = lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	})
	return hasDarkBg
}

// colorYellow returns the adaptive yellow for the CI Passed divider. We
// return the raw lipgloss.Color (an ansi.BasicColor underneath) rather than
// wrapping in an image/color.Color adapter so lipgloss emits a themable SGR
// escape — wrapping forces truecolor RGB and locks the color to one shade.
func colorYellow() color.Color {
	if detectDarkBackground() {
		return colorYellowDark
	}
	return colorYellowLight
}

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
func RenderRow(view core.CommitView, width int) string {
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
func RenderSnapshot(view core.View, width int, now time.Time, spinnerIdx int) string {
	// Grouping, lead times and weekly stats all come from the View. Deriving
	// them here instead would silently ignore the caller's configuration —
	// which is exactly how `clarity.leadTime` first shipped doing nothing.
	snap := view.Snapshot
	g := view.Groups
	indexBySHA := make(map[string]int, len(snap.Commits))
	for i, c := range snap.Commits {
		indexBySHA[c.SHA] = i
	}

	var b strings.Builder
	writeFlat := func(title string, color color.Color, commits []core.CommitView) {
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
	b.WriteString(renderSectionDivider("CI Passed", colorYellow(), width))
	for _, c := range g.CIPassed {
		b.WriteString(renderRowInGroup(c, &g, indexBySHA[c.SHA], width, now, spinnerIdx))
		b.WriteString("\n")
	}
	for _, batch := range g.InFlight {
		// In-flight batches are never "live on production" — they're still
		// deploying or stuck-failed. isLive only applies to passed batches.
		b.WriteString(renderBatchSubheader(batch, now, spinnerIdx, false))
		for _, c := range batch.Commits {
			b.WriteString(renderRowInGroup(c, &g, indexBySHA[c.SHA], width, now, spinnerIdx))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")

	// Deployed: blue lifecycle accent. Only completed (deploy:passed) batches.
	// The first batch (newest passing deploy) is THE currently-live state in
	// production; subsequent batches are settled history. The Deployed header
	// also carries the TOPMOST week's DORA summary on its right side (saving
	// a row) — only older weeks need standalone week dividers below.
	statsByWeek := core.IndexStatsByWeek(view.Weekly)
	topWeekKey, topWeekStat, hasTopWeek := core.FirstPassedWeekStat(g.Deployed, statsByWeek)
	if hasTopWeek {
		b.WriteString(renderSectionDividerWithRight("Deployed", colorBlue, core.WeekDividerLabel(topWeekStat), width))
	} else {
		b.WriteString(renderSectionDivider("Deployed", colorBlue, width))
	}
	prevWeekKey := int64(-1)
	if hasTopWeek {
		prevWeekKey = topWeekKey
	}
	for i, batch := range g.Deployed {
		if batch.Status == "passed" {
			year, week := batch.Time.UTC().ISOWeek()
			key := core.WeekKey(year, week)
			if key != prevWeekKey {
				if s, ok := statsByWeek[key]; ok {
					b.WriteString(renderWeekDivider(s, width))
				}
				prevWeekKey = key
			}
		}
		b.WriteString(renderBatchSubheader(batch, now, spinnerIdx, i == 0))
		for _, c := range batch.Commits {
			b.WriteString(renderRowInGroup(c, &g, indexBySHA[c.SHA], width, now, spinnerIdx))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

// renderWeekDivider is the less-prominent sibling of renderSectionDivider:
// gray dashes (no lifecycle tint, no bold) with the week's DORA summary
// inlined on the RIGHT side, separating groups of batches in different ISO
// weeks. The right alignment + lighter weight keeps the eye on the per-batch
// "deployed Xh ago" subheaders while the weekly aggregate stays available
// peripherally.
func renderWeekDivider(s core.WeekStat, width int) string {
	dashStyle := lipgloss.NewStyle().Foreground(colorGray)
	labelStyle := lipgloss.NewStyle().Foreground(colorGray).Italic(true)
	label := labelStyle.Render(core.WeekDividerLabel(s))

	const trailing = 4
	tail := dashStyle.Render(strings.Repeat("─", trailing))
	// 2 spaces frame the label on each side.
	used := lipgloss.Width(label) + trailing + 2
	if width <= used {
		return label + " " + tail + "\n"
	}
	leading := dashStyle.Render(strings.Repeat("─", width-used))
	return leading + " " + label + " " + tail + "\n"
}

// renderSectionDividerWithRight is renderSectionDivider with an extra right-
// aligned secondary label (italic gray). Used by the Deployed section header
// to absorb the topmost week's DORA summary onto the same row — left part is
// the bold section title, dashes fill the middle, right part is the week
// stats. Falls back to the plain section divider when the terminal isn't
// wide enough to fit both labels.
func renderSectionDividerWithRight(label string, leftColor color.Color, rightLabel string, width int) string {
	dashStyle := lipgloss.NewStyle().Foreground(colorGray)
	rightStyle := lipgloss.NewStyle().Foreground(colorGray).Italic(true)

	leading := dashStyle.Render(strings.Repeat("─", rowAuthorColumn))
	spacedLabel := label + " "
	labelStyle := lipgloss.NewStyle().Bold(true)
	if leftColor != nil {
		labelStyle = labelStyle.Foreground(leftColor)
	}
	boldLabel := labelStyle.Render(spacedLabel)

	const trailingDashes = 4
	rightRendered := rightStyle.Render(rightLabel)
	leftUsed := rowAuthorColumn + lipgloss.Width(spacedLabel)
	// 1 space before rightRendered + 1 space before trailing dashes:
	rightUsed := 1 + lipgloss.Width(rightRendered) + 1 + trailingDashes
	if width <= leftUsed+rightUsed {
		// Not enough room for both labels — degrade to the plain section divider.
		return renderSectionDivider(label, leftColor, width)
	}
	middle := dashStyle.Render(strings.Repeat("─", width-leftUsed-rightUsed))
	tail := dashStyle.Render(strings.Repeat("─", trailingDashes))
	return leading + boldLabel + middle + " " + rightRendered + " " + tail + "\n"
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
func renderSectionDivider(label string, color color.Color, width int) string {
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
// batches. When isLive is true (the topmost passed batch — i.e. what's
// currently running in production), the passed subheader is escalated to
// bold and prefixed with "live on production ·" so the reader can tell
// at a glance which batch is the present state vs. settled history.
func renderBatchSubheader(b core.DeployBatch, now time.Time, spinnerIdx int, isLive bool) string {
	style := lipgloss.NewStyle().Foreground(colorGray).Italic(true)
	switch b.Status {
	case "started":
		spin := lipgloss.NewStyle().Foreground(colorGray).Render(spinnerFrame(spinnerIdx))
		return "  " + spin + " " + style.Render("deploying…") + "\n"
	case "passed":
		ago := ""
		if !now.IsZero() && !b.Time.IsZero() {
			ago = " " + core.FormatElapsed(now.Sub(b.Time)) + " ago"
		}
		if isLive {
			// The currently-live batch is the present state, not a past
			// event — bold blue with the "live on production" anchor.
			return lipgloss.NewStyle().Foreground(colorBlue).Bold(true).
				Render("  live on production · deployed"+ago) + "\n"
		}
		// Older deployed batches are settled history: italic blue.
		return lipgloss.NewStyle().Foreground(colorBlue).Italic(true).
			Render("  deployed"+ago) + "\n"
	case "failed":
		ago := ""
		if !now.IsZero() && !b.Time.IsZero() {
			ago = " " + core.FormatElapsed(now.Sub(b.Time)) + " ago"
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
func renderRowInGroup(view core.CommitView, group *core.Groupings, index int, width int, now time.Time, spinnerIdx int) string {
	icon := ciIcon(view.Events, group, index, spinnerIdx)
	author := lipgloss.NewStyle().Foreground(colorGray).Render(view.Author)
	subject := view.Subject
	left := fmt.Sprintf("  %s  %s  %s", icon, author, subject)

	timer := ""
	if group != nil {
		if d, frozen, ok := group.LeadTime(index, now); ok {
			// Gray while ticking; blue once the deploy that pushed this
			// commit to production fires, so the lead-time "blooms" blue
			// exactly when it freezes — matching the Deployed section's
			// brand colour.
			color := colorGray
			if frozen {
				color = colorBlue
			}
			timer = lipgloss.NewStyle().Foreground(color).Render(core.FormatElapsed(d))
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
func ciIcon(events []clarityrefs.Event, group *core.Groupings, index int, spinnerIdx int) string {
	status := core.CIStatus(events)
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
