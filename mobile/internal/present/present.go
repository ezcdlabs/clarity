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
	out := &v1.Flow{
		Name:       f.Name,
		Deploy:     status(f.Deploy),
		Undeclared: f.Undeclared,
	}
	g := f.Groups

	if len(g.Head) > 0 {
		out.Groups = append(out.Groups, &v1.Group{
			Kind:    v1.GroupKind_GROUP_KIND_HEAD,
			Label:   "HEAD",
			Commits: commits(g.Head, &g, index, now),
		})
	}
	if len(g.CIPassed) > 0 {
		out.Groups = append(out.Groups, &v1.Group{
			Kind:    v1.GroupKind_GROUP_KIND_CI_PASSED,
			Label:   "CI Passed",
			Commits: commits(g.CIPassed, &g, index, now),
		})
	}
	// One group per deploy attempt, rather than one section holding all of
	// them: each carries its own status and time, and a client showing them
	// as one list would have nowhere to put either.
	for _, b := range g.InFlight {
		out.Groups = append(out.Groups, batch(b, v1.GroupKind_GROUP_KIND_IN_FLIGHT, &g, index, now))
	}
	for _, b := range g.Deployed {
		out.Groups = append(out.Groups, batch(b, v1.GroupKind_GROUP_KIND_DEPLOYED, &g, index, now))
	}
	return out
}

func batch(b core.DeployBatch, kind v1.GroupKind, g *core.Groupings, index map[string]int, now time.Time) *v1.Group {
	label := "Deployed"
	if kind == v1.GroupKind_GROUP_KIND_IN_FLIGHT {
		label = "Deploying"
	}
	out := &v1.Group{
		Kind:    kind,
		Label:   label,
		Status:  status(b.Status),
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
		Deploy:              status(core.OverallStatus(c.Events)),
	}
	if i, ok := index[c.SHA]; ok && g != nil {
		if d, frozen, has := g.LeadTime(i, now); has {
			out.HasLeadTime = true
			out.LeadTimeSeconds = int64(d / time.Second)
			out.LeadTime = core.FormatElapsed(d)
			// Live is the inverse of frozen: the deploy that stops this
			// commit's clock has not landed, so a client may keep it ticking.
			out.LeadTimeLive = !frozen
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
