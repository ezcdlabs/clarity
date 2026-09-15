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

// Opts returns the options this renderer was built with, so a caller can
// assert its own wiring reached it.
func (r *Renderer) Opts() Options { return r.opts }

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
		if err := SelectFlow(v, r.opts.Flow); err != nil {
			return err
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
	// Flow narrows the output to a single deploy flow, from --deploy. Empty
	// renders every flow. An unmatched value is an error rather than a
	// fallback to everything: a script or agent that quietly reported on the
	// wrong subsystem is worse than one that failed.
	Flow string
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

	// --deploy narrows to one flow. The header then names only that flow, so
	// piped output describes exactly what was asked for.
	flows := view.Flows
	if opts.Flow != "" {
		i, ok := core.MatchFlow(flows, opts.Flow)
		if !ok {
			return ""
		}
		flows = flows[i : i+1]
	}

	// Whether flows are named is a property of the repo, not of how many are
	// currently on screen: --deploy=ios must still say which flow it is
	// showing, or piped output can't be attributed.
	named := len(view.Flows) > 1

	var b strings.Builder
	b.WriteString(plainHeader(repoName, view.Header, flows, named))
	b.WriteString("\n\n")

	// One block per deploy flow. A repo with a single flow gets no block
	// header at all, so its output is byte-identical to what it rendered
	// before targets existed; only a repo that actually has several pays for
	// the extra structure. Each block reads from its own FlowView, never from
	// view.Groups — the whole point is that one flow's deploys must not move
	// another's lifecycle boundary.
	for fi, flow := range flows {
		if named {
			if fi > 0 {
				b.WriteString("\n")
			}
			b.WriteString("deploy: ")
			b.WriteString(flow.Name)
			if flow.Undeclared {
				b.WriteString("  (undeclared)")
			}
			b.WriteString("\n\n")
		}
		b.WriteString(renderFlowBlock(flow, view, indexBySHA, included, now, opts))
	}

	// Same note the TUI closes with, bare. Piped output has no scrollbar to
	// hint that the list was cut, so it matters at least as much here.
	if view.Snapshot.Truncated {
		b.WriteString(core.LimitNoticeLabel(view.Snapshot.Limit))
		b.WriteString("\n")
	}

	return b.String()
}

// renderFlowBlock renders one flow's HEAD / CI Passed / Deployed sections.
func renderFlowBlock(
	flow core.FlowView,
	view core.View,
	indexBySHA map[string]int,
	included func(string) bool,
	now time.Time,
	opts Options,
) string {
	g := flow.Groups
	var b strings.Builder

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

	statsByWeek := core.IndexStatsByWeek(flow.Weekly)
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
			// The Deployed section obeys --limit like every other section.
			// Without this a capped render printed the whole deploy history
			// and then closed with a notice claiming the limit was why the
			// list ended.
			if !included(c.SHA) {
				continue
			}
			b.WriteString(plainRow(c, &g, indexBySHA[c.SHA], now, opts))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

// plainHeader produces the one-line status: "<repo>  ci: <icon> <state>  deploy: <icon> <state>".
// The statuses arrive already resolved on View.Header — see "The header
// badges" in README.md for which event each one speaks for.
func plainHeader(repoName string, h core.HeaderStatus, flows []core.FlowView, named bool) string {
	ci := plainBadge(h.CI)

	// One flow and nothing to name: the repo-wide deploy badge, exactly as
	// before targets existed. Otherwise one badge per flow, named, because a
	// single summary badge would have to pick one flow's answer and call it
	// the repo's.
	if !named {
		return fmt.Sprintf("%s  ci: %s  deploy: %s", repoName, ci, plainBadge(h.Deploy))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s  ci: %s  deploy:", repoName, ci)
	for _, f := range flows {
		fmt.Fprintf(&b, "  %s: %s", f.Name, plainBadge(f.Deploy))
	}
	return b.String()
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

// SelectFlow reports whether a --deploy value names a flow in this view, and
// returns an error naming the alternatives when it doesn't.
func SelectFlow(view core.View, query string) error {
	if query == "" {
		return nil
	}
	if _, ok := core.MatchFlow(view.Flows, query); ok {
		return nil
	}
	return fmt.Errorf("no deploy flow named %q — this repo has: %s",
		query, strings.Join(core.FlowNames(view.Flows), ", "))
}
