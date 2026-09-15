package core

import (
	"fmt"
	"strings"

	"github.com/ezcdlabs/clarity/clarityrefs"
)

// Flow is one deployable thing the repo ships: a display name plus the set of
// deploy-event targets whose events belong to it.
//
// Targets is a set rather than a single string because a flow's identity can
// outlive the name its pipeline reports. A web backend mid-rename reports both
// "" and "web"; folding them into one flow keeps its history continuous
// without rewriting a single event. It is emphatically not for aggregating
// systems that are concurrently live — merging ios and android would make
// "deployed" mean "one of them deployed" and average two unrelated pipelines
// into one lead time.
//
// The empty string is the untargeted deploy: a flow in its own right, not one
// shared across the others. A repo that never reports a target has exactly one
// flow and renders exactly as it did before targets existed.
type Flow struct {
	Name    string
	Targets []string
}

// DefaultFlowName is what the untargeted deploy is called when nothing has
// been declared and it is the only flow there is. It matches the header label
// a single-flow repo has always shown.
const DefaultFlowName = "deploy"

// UntargetedFlowName labels the untargeted deploy in the one case where
// DefaultFlowName is unavailable: a pipeline reporting a target literally
// named "deploy".
const UntargetedFlowName = "(untargeted)"

// Claims reports whether an event carrying this target belongs to the flow.
func (f Flow) Claims(target string) bool {
	for _, t := range f.Targets {
		if t == target {
			return true
		}
	}
	return false
}

// FlowView is one flow's fully-derived state: its own lifecycle grouping, its
// own DORA throughput, and its own deploy status. Derived once per flow so a
// Renderer can switch between them without re-deriving anything — the rule
// that a renderer consumes its View and never rebuilds from Snapshot applies
// per flow too.
type FlowView struct {
	Flow
	// Groups is this flow's HEAD / CI Passed / Deployed buckets. The
	// boundary between them is set by this flow's deploys alone, so the same
	// commit can sit in different sections in different flows — which is the
	// whole point: a commit that shipped to web has genuinely not shipped to
	// ios.
	Groups Groupings
	// Weekly is this flow's throughput. Per-flow because averaging a
	// two-minute web deploy with a multi-day store review produces a number
	// that describes neither.
	Weekly []WeekStat
	// Deploy is the resolved deploy status for this flow: "" / "passed" /
	// "failed", by the same newest-commit-that-resolved-it rule the header
	// has always used.
	Deploy string
	// Undeclared marks a flow that appeared in the events but is in no
	// .ezcd.json declaration — a typo, or a target someone added without
	// updating config. Surfaced rather than dropped, because a silently
	// discarded deploy is worse than an unexpected tab.
	Undeclared bool
}

// ResolveFlows decides which flows exist for a snapshot.
//
// Declared flows always appear, in declaration order, whether or not they
// carry any events — a declared flow with nothing reported is a failed
// expectation and that is worth seeing. Targets present in the events but
// claimed by no declared flow are appended, sorted, and marked Undeclared.
//
// With nothing declared, flows are discovered from the events alone: the
// untargeted deploy first (when anything is untargeted), then each target
// sorted for a stable order. A repo with no deploy events at all still gets
// the single default flow, so the header has something to render.
func ResolveFlows(commits []CommitView, declared []Flow) []FlowView {
	claimed := map[string]bool{}
	for _, f := range declared {
		for _, t := range f.Targets {
			claimed[t] = true
		}
	}

	// Distinct deploy targets actually present in the window. CI events are
	// ignored: CI never carries a target, so it can never mint a flow.
	seen := map[string]bool{}
	for _, c := range commits {
		for _, e := range c.Events {
			if e.Stage == "deploy" {
				seen[e.Target] = true
			}
		}
	}

	out := make([]FlowView, 0, len(declared)+len(seen))
	for _, f := range declared {
		out = append(out, FlowView{Flow: f})
	}

	extras := make([]string, 0, len(seen))
	for target := range seen {
		if !claimed[target] {
			extras = append(extras, target)
		}
	}
	sortStrings(extras)

	// Names already spoken for. A flow is addressed by name — in the strip,
	// in plain output, and by anything grepping it — so two flows sharing one
	// are indistinguishable to all three. Declared names are reserved first
	// because they are explicit intent; a discovered flow is the one that
	// gives way, being the unexpected arrival.
	used := make(map[string]bool, len(declared)+len(extras))
	for _, f := range declared {
		used[foldName(f.Name)] = true
	}

	// extras is sorted, and "" sorts before every non-empty string, so the
	// untargeted flow already leads. A repo mid-migration therefore never
	// watches its established deploys drop below a newly-named target.
	for _, target := range extras {
		preferred := []string{target}
		if target == "" {
			// The untargeted deploy has no name of its own, so it borrows the
			// label the header has always shown it under.
			preferred = []string{DefaultFlowName, UntargetedFlowName}
		}
		name := uniqueName(preferred, used)
		used[foldName(name)] = true
		out = append(out, FlowView{
			Flow:       Flow{Name: name, Targets: []string{target}},
			Undeclared: len(declared) > 0,
		})
	}

	// A repo with no declarations and no deploy events still needs somewhere
	// to render its (empty) deploy status.
	if len(out) == 0 {
		out = append(out, FlowView{Flow: Flow{Name: DefaultFlowName, Targets: []string{""}}})
	}

	return out
}

// uniqueName returns the first preferred label that is still free, falling
// back to numbered variants of the last one.
//
// Collisions arrive from three directions, all reachable from a plausible
// config: a pipeline reporting a target literally called "deploy", a flow
// declared under that name, and a flow declared under a name that some other
// target also happens to use. A duplicate label is worse than an ugly one, so
// something always gives way.
func uniqueName(preferred []string, used map[string]bool) string {
	for _, candidate := range preferred {
		if !used[foldName(candidate)] {
			return candidate
		}
	}
	base := preferred[len(preferred)-1]
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d", base, n)
		if !used[foldName(candidate)] {
			return candidate
		}
	}
}

// foldName is the key names are compared under. Case-insensitive, because
// --deploy matches that way: two flows differing only in case would be one
// name to the user selecting between them.
func foldName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// sortStrings is a tiny insertion sort so core stays dependency-free; the
// slices are at most a handful of flow names.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// commitsForFlow returns the commits with their events narrowed to the ones
// this flow owns: every CI event (CI is repo-wide and shared by every flow)
// plus only the deploy events whose target this flow claims.
func commitsForFlow(commits []CommitView, f Flow) []CommitView {
	out := make([]CommitView, len(commits))
	for i, c := range commits {
		kept := make([]clarityrefs.Event, 0, len(c.Events))
		for _, e := range c.Events {
			if e.Stage == "deploy" && !f.Claims(e.Target) {
				continue
			}
			kept = append(kept, e)
		}
		c.Events = kept
		out[i] = c
	}
	return out
}
