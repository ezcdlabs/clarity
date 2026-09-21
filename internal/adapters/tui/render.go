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
	"github.com/charmbracelet/x/ansi"
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
	hasDarkBg bool
	termBg    color.Color
	termOnce  sync.Once
)

func detectDarkBackground() bool {
	detectTerminal()
	return hasDarkBg
}

// detectBackgroundColor returns the terminal's actual background colour, or
// nil when it cannot be determined — piped output, no TTY, or a terminal that
// ignores the OSC 11 query.
//
// The deploy strip needs the value itself, not just the light/dark verdict: an
// elevated surface has to be derived from the real background to pick up a
// theme's own tint, and no colour from the ANSI text palette can do that.
func detectBackgroundColor() color.Color {
	detectTerminal()
	return termBg
}

// isDarkColor reports whether a colour reads as dark, by HSL lightness — the
// same test lipgloss applies internally, reimplemented because it isn't
// exported. Colours are alpha-premultiplied 16-bit, which is fine here since a
// terminal background is opaque.
func isDarkColor(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	hi := max(r, max(g, b))
	lo := min(r, min(g, b))
	return float64(hi+lo)/2/65535.0 < 0.5
}

// detectTerminal performs the single OSC 11 round-trip both callers need.
//
// One query, not two: each costs a full terminal round-trip at startup, and
// two could disagree if only one of them got an answer. The light/dark verdict
// is derived from the colour rather than asked for separately.
func detectTerminal() {
	termOnce.Do(func() {
		bg, err := lipgloss.BackgroundColor(os.Stdin, os.Stdout)
		if err != nil {
			// No answer: assume dark, which is what HasDarkBackground does and
			// what is safe in tests and pipelines.
			hasDarkBg = true
			return
		}
		termBg = bg
		hasDarkBg = isDarkColor(bg)
	})
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
func RenderSnapshot(view core.View, flow core.FlowView, width int, now time.Time, spinnerIdx int) string {
	// Grouping, lead times and weekly stats all come from the View. Deriving
	// them here instead would silently ignore the caller's configuration —
	// which is exactly how `clarity.leadTime` first shipped doing nothing.
	//
	// The grouping comes from the selected flow rather than the whole repo,
	// because one flow's deploys must not move another's lifecycle boundary:
	// a commit that shipped to web genuinely has not shipped to ios.
	snap := view.Snapshot
	g := flow.Groups
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
		b.WriteString(renderBatchSubheader(batch, now, spinnerIdx, false, width))
		for _, c := range batch.Commits {
			b.WriteString(renderRowInGroup(c, &g, indexBySHA[c.SHA], width, now, spinnerIdx))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")

	// Deployed: blue lifecycle accent. Only completed (deploy:passed) batches.
	// The first batch (newest passing deploy) is THE currently-live state in
	// production; subsequent batches are settled history. The Deployed header
	// carries the CURRENT week's DORA summary on its right side (saving a
	// row); every other week gets a standalone divider below.
	statsByWeek := core.IndexStatsByWeek(flow.Weekly)
	topWeekKey, topWeekStat, weekState := core.CurrentWeekSummary(g, statsByWeek, now)
	if weekState == core.WeekUnknown {
		// The limit cut through this week, so its count is unknown rather than
		// zero. Saying nothing is the honest answer; the limit notice at the
		// bottom says why.
		b.WriteString(renderSectionDivider("Deployed", colorBlue, width))
	} else {
		b.WriteString(renderSectionDividerWithRight("Deployed", colorBlue, core.WeekDividerLabel(topWeekStat), width))
	}
	if !core.CurrentWeekHasDeploys(g, now) {
		// No batch from this week follows, so the section opens empty — and
		// reads that way, like HEAD and CI Passed above it. Without the blank
		// line the next week's divider butts against this header and the eye
		// binds its totals to it, which is the misreading this exists to stop.
		b.WriteString("\n")
	}

	// A week is marked once. Batches are ordered by commit, not by deploy
	// time, so a redeploy of an older commit puts a week out of sequence and
	// tracking only the previous key would mark it again further down — and
	// the current week, already on the header row, would be marked a second
	// time below it.
	weekShown := map[int64]bool{topWeekKey: true}
	for i, batch := range g.Deployed {
		if batch.Status == "passed" {
			year, week := batch.Time.UTC().ISOWeek()
			key := core.WeekKey(year, week)
			if !weekShown[key] {
				if s, ok := statsByWeek[key]; ok {
					b.WriteString(renderWeekDivider(s, width))
				}
				weekShown[key] = true
			}
		}
		b.WriteString(renderBatchSubheader(batch, now, spinnerIdx, i == 0, width))
		for _, c := range batch.Commits {
			b.WriteString(renderRowInGroup(c, &g, indexBySHA[c.SHA], width, now, spinnerIdx))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if view.Snapshot.Truncated {
		b.WriteString(renderLimitNotice(view.Snapshot.Limit, width))
	}

	return b.String()
}

// renderLimitNotice closes a truncated list with the reason it ended. It
// borrows renderWeekDivider's grammar — gray dashes, italic gray label,
// right-aligned — because it is the same kind of thing: peripheral context
// about the rows above it, not a row in its own right. Someone scrolling for
// a specific commit should be able to ignore it; someone who has hit the
// bottom and not found what they came for should find their answer in it.
func renderLimitNotice(limit int, width int) string {
	dashStyle := lipgloss.NewStyle().Foreground(colorGray)
	labelStyle := lipgloss.NewStyle().Foreground(colorGray).Italic(true)
	// Clipped from the right, unlike the week stats: what matters here is that
	// a limit was reached and which one, so the head of the sentence is the
	// part worth keeping.
	return rightAlignedRule(core.LimitNoticeLabel(limit), labelStyle, dashStyle, width, ClipRight)
}

// rightAlignedRule draws "───── label ────" with the label pushed right.
//
// When it doesn't fit, the decoration goes before the words: the four trailing
// dashes first, then the label clips. Emitting the label whole regardless —
// which is what both callers used to do — produces a line wider than the
// terminal, and one of those makes the entire viewport scroll sideways.
func rightAlignedRule(
	text string,
	labelStyle, dashStyle lipgloss.Style,
	width int,
	clip func(string, int) string,
) string {
	const maxTrailing = 4
	if width <= 0 {
		return labelStyle.Render(text) + " " + dashStyle.Render(strings.Repeat("─", maxTrailing)) + "\n"
	}

	budget := width - 2 // the spaces framing the label
	if budget <= 0 {
		return labelStyle.Render(clip(text, width)) + "\n"
	}

	trailing := maxTrailing
	if trailing > budget-1 {
		trailing = max(0, budget-1)
	}
	shown := clip(text, budget-trailing)
	if shown == "" {
		return dashStyle.Render(strings.Repeat("─", width)) + "\n"
	}

	leading := width - 2 - lipgloss.Width(shown) - trailing
	if leading < 0 {
		leading = 0
	}
	return dashStyle.Render(strings.Repeat("─", leading)) + " " +
		labelStyle.Render(shown) + " " +
		dashStyle.Render(strings.Repeat("─", trailing)) + "\n"
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
	return rightAlignedRule(core.WeekDividerLabel(s), labelStyle, dashStyle, width, ClipLeft)
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

	leftUsed := rowAuthorColumn + lipgloss.Width(spacedLabel)

	// What the week stats and their trailing rule may occupy, after the two
	// spaces that separate them from the section label and from the edge.
	budget := width - leftUsed - 2
	if budget <= 0 {
		// Genuinely no room: the section label wins, because "Deployed" is
		// what the rows below it are grouped under.
		return renderSectionDivider(label, leftColor, width)
	}

	// The stats shed whole facts from the left — the week number before the
	// counts, the counts before the average — so the number worth reading is
	// the last to survive. They used to be dropped the moment the *decorative*
	// trailing rule stopped fitting, which counted decoration as mandatory and
	// lost them at widths where they would still have rendered.
	const trailing = 4
	shown := ClipLeft(rightLabel, budget-trailing)
	if shown == "" {
		return renderSectionDivider(label, leftColor, width)
	}

	middle := width - leftUsed - 2 - lipgloss.Width(shown) - trailing
	if middle < 0 {
		middle = 0
	}
	return leading + boldLabel +
		dashStyle.Render(strings.Repeat("─", middle)) + " " +
		rightStyle.Render(shown) + " " +
		dashStyle.Render(strings.Repeat("─", trailing)) + "\n"
}

// ClipLeft shortens text from its start, keeping the tail.
//
// The week label reads "W2026-38  4 deploys  6d 4h avg" — three self-contained
// facts separated by double spaces — so whole facts are dropped before any
// characters are, giving "4 deploys  6d 4h avg" and then "6d 4h avg". Cutting
// mid-token instead produces fragments like "…0-02  4 deploys", which reads as
// damage rather than as a shorter label. Character clipping is the last resort
// for a final segment that still doesn't fit.
//
// The cut is measured in display columns over grapheme clusters, never by rune
// index. A CJK label is two columns per rune, so index arithmetic against a
// column budget both overflows it and can run off the end of the string.
func ClipLeft(text string, max int) string {
	if max <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= max {
		return text
	}

	segments := strings.Split(text, "  ")
	for i := 1; i < len(segments); i++ {
		candidate := strings.Join(segments[i:], "  ")
		if lipgloss.Width(candidate) <= max {
			return candidate
		}
	}

	// Not even the last fact fits. Returning a fragment of it — "…s avg", or
	// a bare "…" — spends columns on something that says nothing, so the
	// caller is told there is no room and drops the label entirely.
	return ""
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
	if width <= 0 {
		return leading + boldLabel + "\n"
	}
	if width <= used {
		// Narrower than the label itself: the leading rule goes first, then
		// the label clips. A section header wider than the terminal is one
		// over-wide line, and one is enough to make the viewport scroll.
		lead := max(0, min(rowAuthorColumn, width-lipgloss.Width(spacedLabel)))
		return dashStyle.Render(strings.Repeat("─", lead)) +
			labelStyle.Render(ClipRight(spacedLabel, width-lead)) + "\n"
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
// renderBatchSubheader labels a deploy batch. width clips the label rather
// than letting it run past the right edge, since a subheader wide enough to
// overflow makes the whole viewport horizontally scrollable.
func renderBatchSubheader(b core.DeployBatch, now time.Time, spinnerIdx int, isLive bool, width int) string {
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
				Render(fitLine("  live on production · deployed"+ago, width)) + "\n"
		}
		// Older deployed batches are settled history: italic blue.
		return lipgloss.NewStyle().Foreground(colorBlue).Italic(true).
			Render(fitLine("  deployed"+ago, width)) + "\n"
	case "failed":
		ago := ""
		if !now.IsZero() && !b.Time.IsZero() {
			ago = " " + core.FormatElapsed(now.Sub(b.Time)) + " ago"
		}
		return lipgloss.NewStyle().Foreground(colorRed).Italic(true).
			Render(fitLine("  deploy failed"+ago, width)) + "\n"
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

	if width <= 0 {
		if timer == "" {
			return left
		}
		return left + "  " + timer
	}

	// The subject is what yields when the row won't fit. A clipped subject can
	// still be read by widening the terminal; a lead time pushed past the
	// right edge cannot be read at all, and it used to take the viewport with
	// it — the whole view became horizontally scrollable to reach a column
	// that was supposed to be pinned to the edge.
	gap := 0
	if timer != "" {
		gap = 2 // the minimum space before the timer
	}
	fixed := lipgloss.Width(fmt.Sprintf("  %s  %s  ", icon, author))
	room := width - fixed - lipgloss.Width(timer) - gap
	if room < 0 {
		// Even the author doesn't fit. It clips too, rather than letting the
		// row run past the edge: the timer is pinned to the right and a row
		// wider than the terminal makes the whole viewport scroll.
		authorRoom := width - lipgloss.Width(fmt.Sprintf("  %s    ", icon)) - lipgloss.Width(timer) - gap
		author = lipgloss.NewStyle().Foreground(colorGray).Render(ClipRight(view.Author, authorRoom))
		fixed = lipgloss.Width(fmt.Sprintf("  %s  %s  ", icon, author))
		room = width - fixed - lipgloss.Width(timer) - gap
	}
	if lipgloss.Width(subject) > room {
		subject = ClipRight(subject, room)
	}
	left = fmt.Sprintf("  %s  %s  %s", icon, author, subject)
	if lipgloss.Width(left) > width {
		// Nothing left to give: clip the assembled row.
		left = ClipRight(left, width)
	}

	if timer == "" {
		return ClipRight(left, width)
	}
	pad := width - lipgloss.Width(left) - lipgloss.Width(timer)
	if pad < 1 {
		pad = 1
	}
	// A final clamp on the assembled row. Below roughly ten columns even the
	// lead time alone is wider than the terminal, so there is nothing left to
	// protect — but the row still must not be wider than the screen, because
	// a single over-wide line is what makes the viewport pan sideways.
	return ClipRight(left+strings.Repeat(" ", pad)+timer, width)
}

// fitLine clips a whole line to the terminal width. width <= 0 means unknown,
// which leaves the text alone.
func fitLine(text string, width int) string {
	if width <= 0 || lipgloss.Width(text) <= width {
		return text
	}
	return ClipRight(text, width)
}

// ClipRight shortens text to fit, marking the cut so a truncated subject is
// never mistaken for a short one.
//
// Commit subjects are arbitrary user text — CJK at two columns per rune,
// emoji, ZWJ sequences, combining marks, and escape sequences, since nothing
// sanitises what `git log %s` returns. The cut is therefore made over grapheme
// clusters in display columns: slicing by rune index against a column budget
// overflows it for wide characters, runs past the end of the string, and can
// sever an escape sequence so its colour bleeds across the rest of the row.
func ClipRight(text string, max int) string {
	if max <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= max {
		return text
	}
	if max == 1 {
		return "…"
	}
	return ansi.Truncate(text, max-1, "…")
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
