// Package plain implements the plain-text Renderer adapter: a one-shot,
// ANSI-free dump of the lens's current View. Aimed at piped agent / shell-
// script consumers; the bubble-tea TUI lives in internal/adapters/tui.
package plain

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
)

// Compile-time check: the Renderer adapter satisfies the core.Renderer
// port. Drift on the port signature surfaces here at build time.
var _ core.Renderer = (*Renderer)(nil)

// Renderer is the core.Renderer adapter for plain-text output. It consumes
// one View from the channel, prints the rendered text to stdout, and
// returns — matching the doc's "plain mode is one-shot" expectation.
type Renderer struct {
	opts  Options
	nowFn func() time.Time
}

// NewRenderer constructs a plain Renderer with the given options.
func NewRenderer(opts Options) *Renderer {
	return &Renderer{opts: opts}
}

// WithClock returns a copy of r whose "now" timestamp is provided by fn
// instead of the real clock.
func (r *Renderer) WithClock(fn func() time.Time) *Renderer {
	cp := *r
	cp.nowFn = fn
	return &cp
}

// Render reads exactly one View from views, formats it, and writes it to
// stdout. Returns when the View has been written, or when ctx is cancelled,
// or when views closes without emitting (treated as an error — plain mode
// expects a single snapshot to arrive).
func (r *Renderer) Render(ctx context.Context, views <-chan core.View) error {
	select {
	case v, ok := <-views:
		if !ok {
			return fmt.Errorf("plain: source closed before emitting a view")
		}
		now := time.Now()
		if r.nowFn != nil {
			now = r.nowFn()
		}
		_, err := fmt.Print(RenderSnapshot(v.Snapshot.RepoName, v, now, r.opts))
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Options configures RenderSnapshot. ShowSHAs surfaces a short commit hash
// per row (off by default to mirror the TUI's commit-less layout). Limit caps
// the number of commits rendered; zero means no cap.
type Options struct {
	ShowSHAs bool
	Limit    int
}

// RenderSnapshot produces a static, ANSI-free snapshot of the same view the TUI
// shows: a one-line status header, then HEAD / CI Passed / Deployed sections
// matching the TUI's groupings and batch subheaders. Aimed at piped/agent
// consumers — the row vocabulary (✓ ✗ … plus section names) is greppable
// without needing key:value annotations.
func RenderSnapshot(repoName string, view core.View, now time.Time, opts Options) string {
	// Grouping, lead times and weekly stats all come from the View. Deriving
	// them here instead would silently ignore the caller's configuration —
	// which is exactly how `clarity.leadTime` first shipped doing nothing.
	snap := view.Snapshot
	g := view.Groups
	indexBySHA := make(map[string]int, len(snap.Commits))
	for i, c := range snap.Commits {
		indexBySHA[c.SHA] = i
	}

	// Limit now filters the rows rendered rather than truncating the data
	// before grouping. Commits arrive newest-first, so this keeps the same
	// "N newest" selection — but a display limit no longer changes which
	// section a commit lands in or what the weekly average says.
	included := func(sha string) bool {
		return opts.Limit <= 0 || indexBySHA[sha] < opts.Limit
	}

	var b strings.Builder
	b.WriteString(plainHeader(repoName, snap))
	b.WriteString("\n\n")

	writeSection := func(label string, commits []core.CommitView) {
		b.WriteString(label)
		b.WriteString("\n")
		for _, c := range commits {
			if !included(c.SHA) {
				continue
			}
			b.WriteString(plainRow(c, &g, indexBySHA[c.SHA], now, opts))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	writeSection("HEAD", g.Head)

	b.WriteString("CI Passed")
	b.WriteString("\n")
	for _, c := range g.CIPassed {
		if !included(c.SHA) {
			continue
		}
		b.WriteString(plainRow(c, &g, indexBySHA[c.SHA], now, opts))
		b.WriteString("\n")
	}
	for _, batch := range g.InFlight {
		b.WriteString(plainBatchSubheader(batch, now, false))
		for _, c := range batch.Commits {
			if !included(c.SHA) {
				continue
			}
			b.WriteString(plainRow(c, &g, indexBySHA[c.SHA], now, opts))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")

	statsByWeek := core.IndexStatsByWeek(view.Weekly)
	topWeekKey, topWeekStat, hasTopWeek := core.FirstPassedWeekStat(g.Deployed, statsByWeek)
	if hasTopWeek {
		// Merge the topmost week's summary onto the section header row so we
		// don't burn a line on a divider that's about to be followed by the
		// batch subheader for the same week.
		b.WriteString("Deployed  ·  ")
		b.WriteString(core.WeekDividerLabel(topWeekStat))
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
			key := core.WeekKey(year, week)
			if key != prevWeekKey {
				if s, ok := statsByWeek[key]; ok {
					b.WriteString(core.WeekDividerLabel(s))
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

	// Same note the TUI closes with, bare. Piped output has no scrollbar to
	// hint that the list was cut, so it matters at least as much here.
	if view.Snapshot.Truncated {
		b.WriteString(core.LimitNoticeLabel(view.Snapshot.Limit))
		b.WriteString("\n")
	}

	return b.String()
}

// plainHeader produces the one-line status: "<repo>  ci: <icon> <state>  deploy: <icon> <state>".
// Mirrors the TUI's header semantics: "started" and "skipped" events are
// ignored so the badge holds its colour across transient retries.
func plainHeader(repoName string, snap core.Snapshot) string {
	ci := plainBadge(core.CurrentStageStatus(snap.Commits, "ci"))
	deploy := plainBadge(core.CurrentStageStatus(snap.Commits, "deploy"))
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

func plainBatchSubheader(b core.DeployBatch, now time.Time, isLive bool) string {
	switch b.Status {
	case "started":
		return "  … deploying\n"
	case "passed":
		ago := ""
		if !now.IsZero() && !b.Time.IsZero() {
			ago = " " + core.FormatElapsed(now.Sub(b.Time)) + " ago"
		}
		if isLive {
			return "  deployed" + ago + "  (live)\n"
		}
		return "  deployed" + ago + "\n"
	case "failed":
		ago := ""
		if !now.IsZero() && !b.Time.IsZero() {
			ago = " " + core.FormatElapsed(now.Sub(b.Time)) + " ago"
		}
		return "  deploy failed" + ago + "\n"
	default:
		return ""
	}
}

func plainRow(view core.CommitView, group *core.Groupings, index int, now time.Time, opts Options) string {
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
		if d, _, ok := group.LeadTime(index, now); ok {
			b.WriteString("  ")
			b.WriteString(core.FormatElapsed(d))
		}
	}
	return b.String()
}

func plainCIIcon(events []clarityrefs.Event) string {
	switch core.CIStatus(events) {
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
