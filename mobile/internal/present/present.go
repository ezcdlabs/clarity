// Package present turns a derived core.View into the presentation model the
// mobile apps render.
//
// This is the layer that makes the decisions: which status a commit is in,
// how a duration reads, what a section is called, which commits belong to
// which deploy. What crosses the boundary is the answers.
//
// What deliberately does not cross is geometry. The terminal renderer
// quantises everything to character cells and most of its logic exists only
// because of that — reserving a cell for a clamp arrow, forcing a one-cell
// minimum bar, shedding tick labels that would otherwise collide. None of it
// means anything at pixel resolution, and a client that inherited it would be
// carrying a terminal's constraints onto a phone.
package present

import (
	"fmt"
	"time"

	"github.com/ezcdlabs/clarity/internal/core"
	v1 "github.com/ezcdlabs/clarity/proto/gen/go/clarityv1"
)

// shortSHALen matches the width the TUI abbreviates to, so a commit is
// recognisable as the same one across clients.
const shortSHALen = 8

// View maps a derived view to the wire model. now is passed in rather than
// read, so a lead time still running is measured against a clock the caller
// controls — the same reason the renderers take one.
func View(view core.View, now time.Time) *v1.View {
	out := &v1.View{
		RepoName:  view.Snapshot.RepoName,
		Ci:        status(view.Header.CI),
		Deploy:    status(view.Header.Deploy),
		Truncated: view.Snapshot.Truncated,
		Limit:     int32(view.Snapshot.Limit),
		// Always set. A client holds this view across a failed refresh, so it
		// needs to know how old it is — and a field that is never zero is what
		// keeps an otherwise-empty view representable across the FFI boundary.
		GeneratedUnixSeconds: now.Unix(),
	}

	index := indexBySHA(view.Snapshot.Commits)
	for _, f := range view.Flows {
		out.Flows = append(out.Flows, flow(f, index, now))
	}
	return out
}

// indexBySHA maps a commit back to its position in the snapshot, which is
// what the per-commit lead time is keyed on.
func indexBySHA(commits []core.CommitView) map[string]int {
	out := make(map[string]int, len(commits))
	for i, c := range commits {
		out[c.SHA] = i
	}
	return out
}

func flow(f core.FlowView, index map[string]int, now time.Time) *v1.Flow {
	g := f.Groups

	// All three sections, always, in lifecycle order. The terminal draws them
	// whether or not they hold anything, because they are the frame the view is
	// read against — an empty Deployed says "nothing has shipped", which is an
	// answer. A client handed only the non-empty ones could not tell that from
	// a view that forgot to include it.
	head := &v1.Section{
		Kind:    v1.SectionKind_SECTION_KIND_HEAD,
		Label:   "HEAD",
		Commits: commits(g.Head, &g, index, now),
	}
	// CI Passed is the one section with both: commits queued for the next
	// deploy, then any attempt that has not landed. An unfinished deploy stays
	// above the line, because a client that showed it under Deployed would be
	// claiming it shipped.
	green := &v1.Section{
		Kind:    v1.SectionKind_SECTION_KIND_CI_PASSED,
		Label:   "CI Passed",
		Commits: commits(g.CIPassed, &g, index, now),
	}
	for _, b := range g.InFlight {
		green.Batches = append(green.Batches, batch(b, false, &g, index, now))
	}

	shipped := &v1.Section{
		Kind:  v1.SectionKind_SECTION_KIND_DEPLOYED,
		Label: "Deployed",
	}
	live := true
	for _, b := range g.Deployed {
		// Exactly one batch is what is running now: the newest that passed.
		// Everything below it is settled history, and saying so is the
		// difference between "this is production" and "this happened".
		isLive := live && b.Status == "passed"
		if isLive {
			live = false
		}
		shipped.Batches = append(shipped.Batches, batch(b, isLive, &g, index, now))
	}

	return &v1.Flow{
		Name:       f.Name,
		Deploy:     status(f.Deploy),
		Undeclared: f.Undeclared,
		Sections:   []*v1.Section{head, green, shipped},
	}
}

// batchLabel is the subheader the TUI writes above a deploy. The time is
// deliberately not part of it: a client places that separately so it can keep
// ticking, which a baked-in string cannot.
func batchLabel(status string, live bool) string {
	switch status {
	case "started":
		return "deploying…"
	case "passed":
		if live {
			return "live on production · deployed"
		}
		return "deployed"
	case "failed":
		return "deploy failed"
	default:
		return ""
	}
}

func batch(b core.DeployBatch, live bool, g *core.Groupings, index map[string]int, now time.Time) *v1.Batch {
	out := &v1.Batch{
		Status:  status(b.Status),
		Label:   batchLabel(b.Status, live),
		Live:    live,
		Commits: commits(b.Commits, g, index, now),
	}
	if !b.Time.IsZero() {
		out.DeployedUnixSeconds = b.Time.Unix()
		out.DeployedAgo = fmt.Sprintf("%s ago", core.FormatElapsed(now.Sub(b.Time)))
	}
	return out
}

func commits(views []core.CommitView, g *core.Groupings, index map[string]int, now time.Time) []*v1.Commit {
	out := make([]*v1.Commit, 0, len(views))
	for _, c := range views {
		out = append(out, commit(c, g, index, now))
	}
	return out
}

func commit(c core.CommitView, g *core.Groupings, index map[string]int, now time.Time) *v1.Commit {
	out := &v1.Commit{
		Sha:                 c.SHA,
		ShortSha:            shortSHA(c.SHA),
		Subject:             c.Subject,
		Author:              c.Author,
		AuthoredUnixSeconds: c.Time.Unix(),
		Age:                 core.FormatElapsed(now.Sub(c.Time)),
		Ci:                  status(core.CIStatus(c.Events)),
	}
	// No deploy status: whether this commit shipped is said by the section and
	// the batch it sits in, which is why the terminal has only ever drawn one
	// icon per row.
	if i, ok := index[c.SHA]; ok && g != nil {
		out.CiStale = g.IsStaleStage(i, "ci")
		if d, frozen, has := g.LeadTime(i, now); has {
			out.HasLeadTime = true
			out.LeadTimeSeconds = int64(d / time.Second)
			out.LeadTime = core.FormatElapsed(d)
			// Live is the inverse of frozen: the deploy that stops this
			// commit's clock has not landed, so a client may keep it ticking.
			out.LeadTimeLive = !frozen
			if !frozen {
				// Where the clock started, so a client can keep the timer
				// honest without asking for the whole view again every second.
				out.LeadTimeAnchorUnixSeconds = now.Add(-d).Unix()
			}
		}
	}
	return out
}

func shortSHA(sha string) string {
	if len(sha) <= shortSHALen {
		return sha
	}
	return sha[:shortSHALen]
}

// status maps the string vocabulary the events use onto the enum. An
// unrecognised value becomes NONE rather than UNSPECIFIED: a build that
// predates a status should render the commit as unreported, not as broken.
func status(s string) v1.Status {
	switch s {
	case "passed":
		return v1.Status_STATUS_PASSED
	case "failed":
		return v1.Status_STATUS_FAILED
	case "started":
		return v1.Status_STATUS_STARTED
	case "skipped":
		return v1.Status_STATUS_SKIPPED
	default:
		return v1.Status_STATUS_NONE
	}
}
