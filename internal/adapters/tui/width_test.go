package tui_test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
)

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func visible(s string) string { return ansiRe.ReplaceAllString(s, "") }

// widest returns the widest rendered line, which is what decides whether the
// viewport can be scrolled sideways.
func widest(out string) (int, string) {
	max, worst := 0, ""
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(visible(line)); w > max {
			max, worst = w, visible(line)
		}
	}
	return max, worst
}

// TestRenderSnapshot_NeverExceedsTheTerminalWidth — a commit subject longer
// than the terminal used to push the lead time past the right edge, and the
// viewport then scrolled sideways to reach it. The lead time is the one thing
// on the row that can't be recovered by reading further, so the subject is
// what yields: it can always be read by widening the terminal.
func TestRenderSnapshot_NeverExceedsTheTerminalWidth(t *testing.T) {
	long := "refactor the entire billing subsystem and migrate every downstream consumer to the new schema"
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "a", Author: "alice", Subject: long, Time: time.Unix(100, 0),
				Events: []clarityrefs.Event{
					{Stage: "ci", Status: "passed", Time: time.Unix(200, 0)},
					{Stage: "deploy", Status: "passed", Time: time.Unix(300, 0)},
				}},
			{SHA: "b", Author: "bartholomew-longname", Subject: long, Time: time.Unix(50, 0)},
		},
	}
	view := core.DeriveView(snap, core.DefaultLeadTimeMode, nil)

	for _, width := range []int{40, 60, 80, 120} {
		out := renderSnap(view, width, time.Unix(1000, 0), 0)
		if got, line := widest(out); got > width {
			t.Errorf("at width %d a line is %d columns wide:\n%q", width, got, line)
		}
	}
}

// The lead time must survive the clipping — losing it is the bug.
func TestRenderSnapshot_KeepsTheLeadTimeWhenTheSubjectIsClipped(t *testing.T) {
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "a", Author: "alice", Subject: strings.Repeat("very long subject ", 10), Time: time.Unix(100, 0),
				Events: []clarityrefs.Event{
					{Stage: "ci", Status: "passed", Time: time.Unix(200, 0)},
					{Stage: "deploy", Status: "passed", Time: time.Unix(300, 0)},
				}},
		},
	}
	view := core.DeriveView(snap, core.DefaultLeadTimeMode, nil)
	out := visible(renderSnap(view, 50, time.Unix(1000, 0), 0))

	if !strings.Contains(out, "3m 20s") {
		t.Errorf("the lead time was lost to a long subject:\n%s", out)
	}
	if !strings.Contains(out, "…") {
		t.Errorf("the subject was not clipped:\n%s", out)
	}
}

// TestRenderSnapshot_WeekStatsSurviveNarrowTerminals — the Deployed divider
// carries the week's deploy count and average lead time. It used to drop them
// entirely as soon as the *decorative* trailing dashes didn't fit, so they
// vanished at widths where they would still have rendered.
func TestRenderSnapshot_WeekStatsSurviveNarrowTerminals(t *testing.T) {
	day := int64(86400)
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "a", Author: "al", Subject: "x", Time: time.Unix(10*day, 0),
				Events: []clarityrefs.Event{
					{Stage: "ci", Status: "passed", Time: time.Unix(10*day+60, 0)},
					{Stage: "deploy", Status: "passed", Time: time.Unix(10*day+120, 0)},
				}},
		},
	}
	view := core.DeriveView(snap, core.DefaultLeadTimeMode, nil)

	full := visible(renderSnap(view, 120, time.Unix(11*day, 0), 0))
	if !strings.Contains(full, "deploy") {
		t.Fatalf("no week stats even at 120 columns:\n%s", full)
	}

	// At every width that can fit the label plus some of the stats, some of
	// the stats must render.
	for _, width := range []int{40, 50, 60, 80} {
		out := visible(renderSnap(view, width, time.Unix(11*day, 0), 0))
		if !strings.Contains(out, "avg") {
			t.Errorf("at width %d the week stats vanished entirely:\n%s", width, out)
		}
		if got, line := widest(out); got > width {
			t.Errorf("at width %d the divider is %d columns:\n%q", width, got, line)
		}
	}
}

// The week label is three self-contained facts. Shortening it should drop
// whole facts, not cut through one: "…0-02  2 deploys" reads as damage rather
// than as a shorter label.
func TestRenderSnapshot_WeekStatsDropWholeFactsWhenShortened(t *testing.T) {
	day := int64(86400)
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "a", Author: "al", Subject: "x", Time: time.Unix(10*day, 0),
				Events: []clarityrefs.Event{
					{Stage: "ci", Status: "passed", Time: time.Unix(10*day+60, 0)},
					{Stage: "deploy", Status: "passed", Time: time.Unix(10*day+120, 0)},
				}},
		},
	}
	view := core.DeriveView(snap, core.DefaultLeadTimeMode, nil)

	for _, width := range []int{44, 48, 52, 56, 60} {
		out := visible(renderSnap(view, width, time.Unix(11*day, 0), 0))
		divider := ""
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "Deployed") {
				divider = line
			}
		}
		if divider == "" {
			t.Fatalf("no Deployed divider at width %d", width)
		}
		// A fragment of the week number ("W1970-02" cut mid-token) is the
		// failure; whole segments surviving is the success.
		if strings.Contains(divider, "…") && !strings.Contains(divider, "… ") {
			t.Errorf("at width %d the label was cut mid-token: %q", width, divider)
		}
	}
}

// TestRenderSnapshot_SweepEveryWidth is the exhaustive form of the promise.
// One over-wide line anywhere makes the whole viewport horizontally
// scrollable, so it isn't enough for rows to behave — every divider, every
// subheader and every notice has to fit too, at every width, for text that
// includes wide characters.
func TestRenderSnapshot_SweepEveryWidth(t *testing.T) {
	day := int64(86400)
	subjects := []string{
		"refactor the entire billing subsystem and migrate every consumer",
		strings.Repeat("重", 20),
		"🚀🔥✨ deploy the thing now and then some more text",
		"\x1b[31mred\x1b[0m subject with escapes in it",
		"ok",
	}

	var commits []core.CommitView
	for i, subj := range subjects {
		ts := int64(20-i*3) * day
		commits = append(commits, core.CommitView{
			SHA: string(rune('a' + i)), Author: "bartholomew-longname", Subject: subj, Time: time.Unix(ts, 0),
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: time.Unix(ts+60, 0)},
				{Stage: "deploy", Status: "passed", Time: time.Unix(ts+120, 0)},
			},
		})
	}
	snap := core.Snapshot{RepoName: "clarity", Commits: commits, Truncated: true, Limit: 100}
	view := core.DeriveView(snap, core.DefaultLeadTimeMode, nil)

	for width := 1; width <= 200; width++ {
		out := renderSnap(view, width, time.Unix(30*day, 0), 0)
		if got, line := widest(out); got > width {
			t.Fatalf("at width %d a line is %d columns:\n%q", width, got, line)
		}
	}
}

// width 0 means "size not known yet" — the first frame, before Bubble Tea
// delivers a WindowSizeMsg. Nothing may be clipped away on that frame.
func TestRenderSnapshot_UnknownWidthClipsNothing(t *testing.T) {
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "a", Author: "alice", Subject: "a reasonably long commit subject here", Time: time.Unix(100, 0),
				Events: []clarityrefs.Event{
					{Stage: "ci", Status: "passed", Time: time.Unix(200, 0)},
					{Stage: "deploy", Status: "passed", Time: time.Unix(300, 0)},
				}},
		},
	}
	view := core.DeriveView(snap, core.DefaultLeadTimeMode, nil)
	out := visible(renderSnap(view, 0, time.Unix(1000, 0), 0))

	if !strings.Contains(out, "a reasonably long commit subject here") {
		t.Errorf("the subject was clipped at unknown width:\n%s", out)
	}
	if strings.Contains(out, "…") {
		t.Errorf("something was clipped at unknown width:\n%s", out)
	}
}

// Below about 21 columns the section label leaves no room for the week stats
// at all, and the decorative trailing rule is what has to go first. Nothing
// else in the suite reaches that width.
func TestRenderSnapshot_VeryNarrowShedsDecorationFirst(t *testing.T) {
	day := int64(86400)
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "a", Author: "al", Subject: "x", Time: time.Unix(10*day, 0),
				Events: []clarityrefs.Event{
					{Stage: "ci", Status: "passed", Time: time.Unix(10*day+60, 0)},
					{Stage: "deploy", Status: "passed", Time: time.Unix(10*day+120, 0)},
				}},
		},
	}
	view := core.DeriveView(snap, core.DefaultLeadTimeMode, nil)

	for _, width := range []int{16, 18, 20, 22} {
		out := visible(renderSnap(view, width, time.Unix(11*day, 0), 0))
		if got, line := widest(out); got > width {
			t.Errorf("at width %d a line is %d columns: %q", width, got, line)
		}
		if !strings.Contains(out, "Deployed") {
			t.Errorf("at width %d the section label was lost:\n%s", width, out)
		}
	}
}

// TestRenderSnapshot_LeadTimeSurvivesALongAuthor — fitting inside the terminal
// isn't enough on its own: the row has to give up the right things. A long
// author name must clip before the lead time does, because the lead time is
// what the right-hand column exists for.
func TestRenderSnapshot_LeadTimeSurvivesALongAuthor(t *testing.T) {
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "a", Author: "bartholomew-longname-mclongfacington", Subject: "x", Time: time.Unix(100, 0),
				Events: []clarityrefs.Event{
					{Stage: "ci", Status: "passed", Time: time.Unix(200, 0)},
					{Stage: "deploy", Status: "passed", Time: time.Unix(300, 0)},
				}},
		},
	}
	view := core.DeriveView(snap, core.DefaultLeadTimeMode, nil)

	for _, width := range []int{20, 30, 40, 49} {
		out := visible(renderSnap(view, width, time.Unix(1000, 0), 0))
		if !strings.Contains(out, "3m 20s") {
			t.Errorf("at width %d the lead time was clipped instead of the author:\n%s", width, out)
		}
	}
}

// The week stats show whole facts or nothing. A fragment like "…s avg" spends
// columns on something that conveys nothing, so below the width where the
// average fits intact the label is dropped and the rule takes the space back.
func TestRenderSnapshot_WeekStatsAreWholeFactsOrAbsent(t *testing.T) {
	day := int64(86400)
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "a", Author: "al", Subject: "x", Time: time.Unix(10*day, 0),
				Events: []clarityrefs.Event{
					{Stage: "ci", Status: "passed", Time: time.Unix(10*day+60, 0)},
					{Stage: "deploy", Status: "passed", Time: time.Unix(10*day+120, 0)},
				}},
		},
	}
	view := core.DeriveView(snap, core.DefaultLeadTimeMode, nil)

	for width := 16; width <= 80; width++ {
		out := visible(renderSnap(view, width, time.Unix(11*day, 0), 0))
		for _, line := range strings.Split(out, "\n") {
			if !strings.Contains(line, "Deployed") {
				continue
			}
			if strings.Contains(line, "…") {
				t.Errorf("at width %d the week label was rendered as a fragment: %q", width, line)
			}
			// Whenever a non-zero count shows, so does the average. Zero
			// deploys legitimately has none — there is no average of nothing.
			if strings.Contains(line, "deploys") &&
				!strings.Contains(line, "0 deploys") &&
				!strings.Contains(line, "avg") {
				t.Errorf("at width %d the count survived but the average didn't: %q", width, line)
			}
		}
	}
}

// Batch subheaders must render in full before the terminal size is known —
// width 0 means "not yet", not "no room".
func TestRenderSnapshot_UnknownWidthKeepsSubheaders(t *testing.T) {
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{SHA: "a", Author: "alice", Subject: "x", Time: time.Unix(100, 0),
				Events: []clarityrefs.Event{
					{Stage: "ci", Status: "passed", Time: time.Unix(200, 0)},
					{Stage: "deploy", Status: "passed", Time: time.Unix(300, 0)},
				}},
		},
	}
	view := core.DeriveView(snap, core.DefaultLeadTimeMode, nil)
	out := visible(renderSnap(view, 0, time.Unix(1000, 0), 0))

	if !strings.Contains(out, "live on production") {
		t.Errorf("the batch subheader was clipped away at unknown width:\n%s", out)
	}
}
