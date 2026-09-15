package core_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
)

func dep(target string, ts int64) clarityrefs.Event {
	return clarityrefs.Event{Stage: "deploy", Status: "passed", Time: time.Unix(ts, 0), Target: target}
}
func ciEv(ts int64) clarityrefs.Event {
	return clarityrefs.Event{Stage: "ci", Status: "passed", Time: time.Unix(ts, 0)}
}
func flowCommit(sha string, events ...clarityrefs.Event) core.CommitView {
	return core.CommitView{SHA: sha, Subject: sha, Author: "a", Time: time.Unix(100, 0), Events: events}
}

// TestResolveFlows covers how the flow set is decided: declared flows win and
// keep their order, undeclared targets found in events still surface (a
// silently dropped deploy is the worst outcome available), and a repo that
// declares nothing gets flows discovered from its events.
func TestResolveFlows(t *testing.T) {
	cases := []struct {
		name     string
		declared []core.Flow
		commits  []core.CommitView
		want     []string // flow names, in order
	}{
		{
			name:    "no declarations and no targets is one default flow",
			commits: []core.CommitView{flowCommit("a", ciEv(1), dep("", 2))},
			want:    []string{"deploy"},
		},
		{
			name:    "no events at all still yields the default flow",
			commits: []core.CommitView{flowCommit("a")},
			want:    []string{"deploy"},
		},
		{
			name: "undeclared targets are discovered, default first then sorted",
			commits: []core.CommitView{
				flowCommit("a", dep("ios", 2)),
				flowCommit("b", dep("", 1)),
				flowCommit("c", dep("android", 3)),
			},
			want: []string{"deploy", "android", "ios"},
		},
		{
			name: "discovery omits the default flow when nothing is untargeted",
			commits: []core.CommitView{
				flowCommit("a", dep("ios", 2)),
				flowCommit("b", dep("web", 1)),
			},
			want: []string{"ios", "web"},
		},
		{
			name:     "declared flows keep declaration order, not alphabetical",
			declared: []core.Flow{{Name: "web", Targets: []string{"", "web"}}, {Name: "ios", Targets: []string{"ios"}}},
			commits:  []core.CommitView{flowCommit("a", dep("ios", 2)), flowCommit("b", dep("", 1))},
			want:     []string{"web", "ios"},
		},
		{
			name:     "a declared flow with no events still appears",
			declared: []core.Flow{{Name: "web", Targets: []string{""}}, {Name: "android", Targets: []string{"android"}}},
			commits:  []core.CommitView{flowCommit("a", dep("", 1))},
			want:     []string{"web", "android"},
		},
		{
			name:     "an undeclared target appears after the declared flows",
			declared: []core.Flow{{Name: "web", Targets: []string{"", "web"}}},
			commits:  []core.CommitView{flowCommit("a", dep("", 1)), flowCommit("b", dep("ios", 2))},
			want:     []string{"web", "ios"},
		},
		{
			name:     "a target claimed by a declared flow is not also discovered",
			declared: []core.Flow{{Name: "web", Targets: []string{"", "web"}}},
			commits:  []core.CommitView{flowCommit("a", dep("web", 1)), flowCommit("b", dep("", 2))},
			want:     []string{"web"},
		},
		{
			name:     "ci events never create a flow",
			declared: nil,
			commits:  []core.CommitView{flowCommit("a", ciEv(1)), flowCommit("b", ciEv(2))},
			want:     []string{"deploy"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := core.ResolveFlows(c.commits, c.declared)
			if len(got) != len(c.want) {
				t.Fatalf("ResolveFlows = %v, want %v", names(got), c.want)
			}
			for i := range got {
				if got[i].Name != c.want[i] {
					t.Fatalf("ResolveFlows = %v, want %v", names(got), c.want)
				}
			}
		})
	}
}

// TestResolveFlows_MarksUndeclared keeps the expectation-vs-actual distinction
// visible: a target nobody declared is rendered, but it is rendered as a
// surprise rather than as a peer of the declared flows.
func TestResolveFlows_MarksUndeclared(t *testing.T) {
	declared := []core.Flow{{Name: "web", Targets: []string{"", "web"}}}
	commits := []core.CommitView{flowCommit("a", dep("", 1)), flowCommit("b", dep("ios", 2))}

	got := core.ResolveFlows(commits, declared)
	if len(got) != 2 {
		t.Fatalf("want 2 flows, got %v", names(got))
	}
	if got[0].Undeclared {
		t.Error("declared flow web marked undeclared")
	}
	if !got[1].Undeclared {
		t.Error("undeclared target ios not marked as such")
	}
}

// TestDeriveView_FlowsGroupIndependently is the core of the feature: one
// flow's deploys must not move another flow's lifecycle boundary. The same
// commit sits in different sections depending on which flow is asked.
func TestDeriveView_FlowsGroupIndependently(t *testing.T) {
	snap := core.Snapshot{
		Commits: []core.CommitView{
			flowCommit("newest", ciEv(10), dep("", 20)), // shipped to the default flow only
			flowCommit("older", ciEv(1), dep("ios", 2)), // shipped to ios only
		},
	}
	declared := []core.Flow{
		{Name: "web", Targets: []string{""}},
		{Name: "ios", Targets: []string{"ios"}},
	}

	view := core.DeriveView(snap, core.DefaultLeadTimeMode, declared)

	if len(view.Flows) != 2 {
		t.Fatalf("want 2 flow views, got %d", len(view.Flows))
	}

	web, ios := view.Flows[0], view.Flows[1]

	if !deployedContains(web, "newest") {
		t.Error("web: newest commit should be deployed — it carries an untargeted deploy")
	}
	if deployedContains(ios, "newest") {
		t.Error("ios: newest commit must not be deployed — no ios deploy has shipped it")
	}
	if !deployedContains(ios, "older") {
		t.Error("ios: the ios-targeted commit should be deployed")
	}
}

// TestDeriveView_CIIsSharedAcrossFlows pins the rule that keeps this from
// becoming a per-subsystem status board: CI is repo-wide, so every flow sees
// every CI event — including a flow that claims no untargeted deploys and so
// shares none of the commit's deploy events.
//
// The assertion has to reach into each flow's own grouping. Checking
// View.Header.CI would prove nothing: it is computed from the unfiltered
// commits and cannot be affected by flow filtering at all.
func TestDeriveView_CIIsSharedAcrossFlows(t *testing.T) {
	snap := core.Snapshot{
		Commits: []core.CommitView{
			// Passed CI, deployed nowhere: it can only be grouped as CI-passed,
			// and only if its ci event survived the flow's filtering.
			flowCommit("green", ciEv(10)),
			flowCommit("shipped", ciEv(1), dep("ios", 2), dep("", 3)),
		},
	}
	declared := []core.Flow{
		{Name: "web", Targets: []string{""}},
		{Name: "ios", Targets: []string{"ios"}},
	}

	view := core.DeriveView(snap, core.DefaultLeadTimeMode, declared)

	for _, f := range view.Flows {
		if !ciPassedContains(f, "green") {
			t.Errorf("flow %s: commit with a passing ci event is not in CI Passed — "+
				"ci events must reach every flow, not just the one claiming the deploy", f.Name)
		}
	}
}

// TestDeriveView_FlowDeployStatusIsPerFlow pins FlowView.Deploy, which is what
// the header strip renders per flow. One flow failing must not paint another.
func TestDeriveView_FlowDeployStatusIsPerFlow(t *testing.T) {
	snap := core.Snapshot{
		Commits: []core.CommitView{
			flowCommit("head",
				clarityrefs.Event{Stage: "deploy", Status: "failed", Time: time.Unix(20, 0), Target: "ios"},
				clarityrefs.Event{Stage: "deploy", Status: "passed", Time: time.Unix(21, 0)},
			),
		},
	}
	declared := []core.Flow{
		{Name: "web", Targets: []string{""}},
		{Name: "ios", Targets: []string{"ios"}},
	}

	view := core.DeriveView(snap, core.DefaultLeadTimeMode, declared)

	if got := view.Flows[0].Deploy; got != "passed" {
		t.Errorf("web flow Deploy = %q, want passed", got)
	}
	if got := view.Flows[1].Deploy; got != "failed" {
		t.Errorf("ios flow Deploy = %q, want failed", got)
	}
}

// TestDeriveView_WeeklyIsPerFlow guards the reason the split exists at all: a
// two-minute web deploy and a multi-day store review must not be averaged into
// one throughput number that describes neither.
func TestDeriveView_WeeklyIsPerFlow(t *testing.T) {
	day := int64(86400)
	snap := core.Snapshot{
		Commits: []core.CommitView{
			flowCommit("web-change", ciEv(10*day), dep("", 10*day+60)),
			flowCommit("ios-change", ciEv(1*day), dep("ios", 8*day)),
		},
	}
	declared := []core.Flow{
		{Name: "web", Targets: []string{""}},
		{Name: "ios", Targets: []string{"ios"}},
	}

	view := core.DeriveView(snap, core.DefaultLeadTimeMode, declared)

	webDeploys := totalDeploys(view.Flows[0].Weekly)
	iosDeploys := totalDeploys(view.Flows[1].Weekly)

	if webDeploys != 1 {
		t.Errorf("web flow counted %d deploys, want 1 — it must not see the ios deploy", webDeploys)
	}
	if iosDeploys != 1 {
		t.Errorf("ios flow counted %d deploys, want 1 — it must not see the untargeted deploy", iosDeploys)
	}
}

// TestResolveFlows_DiscoveredFlowsAreNotUndeclared is the other half of the
// expectation model: when nothing is declared there is no expectation to
// violate, so no flow may be marked a surprise.
func TestResolveFlows_DiscoveredFlowsAreNotUndeclared(t *testing.T) {
	commits := []core.CommitView{flowCommit("a", dep("", 1)), flowCommit("b", dep("ios", 2))}

	for _, f := range core.ResolveFlows(commits, nil) {
		if f.Undeclared {
			t.Errorf("flow %s marked undeclared, but the repo declares nothing", f.Name)
		}
	}
}

// TestResolveFlows_UntargetedLeadsAndNameCollisionYields covers the ordering
// promise and the one input that can break it: a pipeline reporting a target
// literally named "deploy". The invented label is the one that gives way,
// because a target name is data and has to render as itself.
func TestResolveFlows_UntargetedLeadsAndNameCollisionYields(t *testing.T) {
	commits := []core.CommitView{
		flowCommit("a", dep("zzz", 1)),
		flowCommit("b", dep("", 2)),
		flowCommit("c", dep("deploy", 3)),
	}

	got := core.ResolveFlows(commits, nil)

	if got[0].Targets[0] != "" {
		t.Errorf("first flow claims %q, want the untargeted deploy to lead", got[0].Targets[0])
	}

	seen := map[string]bool{}
	for _, f := range got {
		if seen[f.Name] {
			t.Fatalf("two flows render under the same name %q: %v", f.Name, names(got))
		}
		seen[f.Name] = true
	}
}

func ciPassedContains(f core.FlowView, subject string) bool {
	for _, c := range f.Groups.CIPassed {
		if c.Subject == subject {
			return true
		}
	}
	return false
}

func totalDeploys(weeks []core.WeekStat) int {
	n := 0
	for _, w := range weeks {
		n += w.Deploys
	}
	return n
}

func names(fs []core.FlowView) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Name
	}
	return out
}

func deployedContains(f core.FlowView, subject string) bool {
	for _, batch := range f.Groups.Deployed {
		for _, c := range batch.Commits {
			if c.Subject == subject {
				return true
			}
		}
	}
	return false
}

// TestResolveFlows_NamesAreUnique is the invariant the whole per-flow
// rendering rests on: a flow is addressed by name, in the strip, in plain
// output and by anything grepping it. Two flows sharing a label are
// indistinguishable to all three.
//
// The collisions are reachable from two directions — a target literally named
// "deploy", and a flow declared under that name — and only the invented label
// may move.
func TestResolveFlows_NamesAreUnique(t *testing.T) {
	cases := []struct {
		name     string
		declared []core.Flow
		commits  []core.CommitView
	}{
		{
			name:    "a target named like the default flow",
			commits: []core.CommitView{flowCommit("a", dep("", 1)), flowCommit("b", dep("deploy", 2))},
		},
		{
			name:     "a declared flow named like the default flow",
			declared: []core.Flow{{Name: "deploy", Targets: []string{"web"}}},
			commits:  []core.CommitView{flowCommit("a", dep("", 1)), flowCommit("b", dep("web", 2))},
		},
		{
			name:     "both escape hatches taken at once",
			declared: []core.Flow{{Name: "deploy", Targets: []string{"web"}}, {Name: "(untargeted)", Targets: []string{"x"}}},
			commits:  []core.CommitView{flowCommit("a", dep("", 1)), flowCommit("b", dep("web", 2))},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := core.ResolveFlows(c.commits, c.declared)
			seen := map[string]bool{}
			for _, f := range got {
				if seen[f.Name] {
					t.Fatalf("two flows render under the name %q: %v", f.Name, names(got))
				}
				seen[f.Name] = true
			}
		})
	}
}

// TestResolveFlows_SingleFlowClaimsEveryDeploy pins what DeriveView's
// short-circuit depends on. Reusing the whole-repo grouping for a lone flow is
// only correct while "one flow resolved" implies "that flow owns every deploy
// event present" — if ResolveFlows ever stops minting a flow per unclaimed
// target, the short-circuit would start silently dropping events.
func TestResolveFlows_SingleFlowClaimsEveryDeploy(t *testing.T) {
	cases := [][]core.CommitView{
		{flowCommit("a")},
		{flowCommit("a", dep("", 1))},
		{flowCommit("a", dep("", 1)), flowCommit("b", dep("", 2))},
		{flowCommit("a", ciEv(1))},
	}
	declarations := [][]core.Flow{nil, {{Name: "only", Targets: []string{""}}}}

	for _, commits := range cases {
		for _, declared := range declarations {
			got := core.ResolveFlows(commits, declared)
			if len(got) != 1 {
				continue
			}
			for _, c := range commits {
				for _, e := range c.Events {
					if e.Stage == "deploy" && !got[0].Claims(e.Target) {
						t.Fatalf("single flow %q does not claim deploy target %q — "+
							"DeriveView's short-circuit would drop it", got[0].Name, e.Target)
					}
				}
			}
		}
	}
}

// TestDeriveView_SingleFlowReusesTheRepoGrouping pins the derivation cost
// itself. Without it, computing a second identical grouping for every
// single-flow repo — every repo that has no targets — could silently return.
func TestDeriveView_SingleFlowReusesTheRepoGrouping(t *testing.T) {
	snap := core.Snapshot{
		Commits: []core.CommitView{
			flowCommit("shipped", ciEv(1), dep("", 2)),
			flowCommit("older", ciEv(3), dep("", 4)),
		},
	}

	view := core.DeriveView(snap, core.DefaultLeadTimeMode, nil)

	if len(view.Flows) != 1 {
		t.Fatalf("want a single flow, got %v", names(view.Flows))
	}
	if len(view.Groups.Deployed) == 0 || len(view.Flows[0].Groups.Deployed) == 0 {
		t.Fatal("expected deployed batches in both groupings")
	}
	if &view.Flows[0].Groups.Deployed[0] != &view.Groups.Deployed[0] {
		t.Error("single flow recomputed its grouping instead of reusing the repo-wide one")
	}
}

// TestResolveFlows_DeclaredNamesReserveTheLabel covers the collision the
// config layer cannot see: a flow declared under a name that some *other*
// target also happens to use. Load-time validation only compares declared
// names against declared targets, so the discovered flow is the one that has
// to give way, and only ResolveFlows can do it.
func TestResolveFlows_DeclaredNamesReserveTheLabel(t *testing.T) {
	cases := []struct {
		name     string
		declared []core.Flow
		commits  []core.CommitView
	}{
		{
			name:     "a discovered target matching a declared flow's name",
			declared: []core.Flow{{Name: "web", Targets: []string{"frontend"}}},
			commits:  []core.CommitView{flowCommit("a", dep("frontend", 1)), flowCommit("b", dep("web", 2))},
		},
		{
			name:     "matching case-insensitively",
			declared: []core.Flow{{Name: "Web", Targets: []string{"frontend"}}},
			commits:  []core.CommitView{flowCommit("a", dep("frontend", 1)), flowCommit("b", dep("web", 2))},
		},
		{
			name:     "two discovered targets differing only by case",
			declared: nil,
			commits:  []core.CommitView{flowCommit("a", dep("Web", 1)), flowCommit("b", dep("web", 2))},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := core.ResolveFlows(c.commits, c.declared)
			seen := map[string]bool{}
			for _, f := range got {
				key := strings.ToLower(f.Name)
				if seen[key] {
					t.Fatalf("two flows render under the name %q: %v", f.Name, names(got))
				}
				seen[key] = true
			}
			// Every deploy target present must still have exactly one home —
			// disambiguating a label must never drop a flow.
			if len(got) != len(c.commits) && len(c.declared) == 0 {
				t.Errorf("expected a flow per target, got %v", names(got))
			}
		})
	}
}

// TestMatchFlow is the classifier behind --deploy. Matching is forgiving on
// case because a flow is a label a human types, and falls back from names to
// targets because the two usually coincide and a user shouldn't have to know
// which one they're naming.
func TestMatchFlow(t *testing.T) {
	flows := []core.FlowView{
		{Flow: core.Flow{Name: "web", Targets: []string{"", "web"}}},
		{Flow: core.Flow{Name: "iOS", Targets: []string{"ios"}}},
		{Flow: core.Flow{Name: "android", Targets: []string{"droid"}}},
	}

	cases := []struct {
		query string
		want  int // -1 for no match
	}{
		{query: "web", want: 0},
		{query: "iOS", want: 1},
		{query: "ios", want: 1},   // case-insensitive name match
		{query: "IOS", want: 1},   // and the other way
		{query: "droid", want: 2}, // falls back to a target name
		{query: "DROID", want: 2},
		{query: " web ", want: 0}, // surrounding whitespace is a typo, not a name
		{query: "android", want: 2},
		{query: "", want: -1},
		{query: "nope", want: -1},
		{query: "we", want: -1}, // no prefix matching: too easy to hit the wrong flow
	}

	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			got, ok := core.MatchFlow(flows, c.query)
			if c.want < 0 {
				if ok {
					t.Errorf("MatchFlow(%q) matched flow %d, want no match", c.query, got)
				}
				return
			}
			if !ok || got != c.want {
				t.Errorf("MatchFlow(%q) = %d, %v; want %d, true", c.query, got, ok, c.want)
			}
		})
	}
}

// A name must win over another flow's target. Config load rejects that
// collision, but discovery can still produce it, and the name is the label the
// user actually sees in the strip.
func TestMatchFlow_NameBeatsAnotherFlowsTarget(t *testing.T) {
	flows := []core.FlowView{
		{Flow: core.Flow{Name: "legacy", Targets: []string{"web"}}},
		{Flow: core.Flow{Name: "web", Targets: []string{"frontend"}}},
	}

	got, ok := core.MatchFlow(flows, "web")
	if !ok || got != 1 {
		t.Errorf("MatchFlow(web) = %d, %v; want the flow *named* web (1)", got, ok)
	}
}

func scopeOf(target string, affected bool, ts int64) clarityrefs.Scope {
	return clarityrefs.Scope{Target: target, Affected: affected, Time: time.Unix(ts, 0)}
}

// TestIsCandidate is the classifier behind candidacy. Silence means candidate:
// a repo that reports nothing must keep the numbers it always had, which is
// the same principle that makes `all` the default lead-time mode.
func TestIsCandidate(t *testing.T) {
	web := core.Flow{Name: "web", Targets: []string{"frontend", "web"}}
	ios := core.Flow{Name: "ios", Targets: []string{"ios"}}

	cases := []struct {
		name  string
		flow  core.Flow
		scope []clarityrefs.Scope
		want  bool
	}{
		{name: "nothing reported is a candidate", flow: ios, want: true},
		{
			name: "reported affected", flow: ios,
			scope: []clarityrefs.Scope{scopeOf("ios", true, 10)}, want: true,
		},
		{
			name: "reported unaffected", flow: ios,
			scope: []clarityrefs.Scope{scopeOf("ios", false, 10)}, want: false,
		},
		{
			name: "another target's record says nothing about this flow", flow: ios,
			scope: []clarityrefs.Scope{scopeOf("android", false, 10)}, want: true,
		},
		{
			name: "a flow claiming several targets takes any of them", flow: web,
			scope: []clarityrefs.Scope{scopeOf("web", false, 10)}, want: false,
		},
		{
			name: "the latest record wins", flow: ios,
			scope: []clarityrefs.Scope{scopeOf("ios", false, 10), scopeOf("ios", true, 20)}, want: true,
		},
		{
			name: "the latest record wins in either direction", flow: ios,
			scope: []clarityrefs.Scope{scopeOf("ios", true, 20), scopeOf("ios", false, 30)}, want: false,
		},
		{
			name: "order in the slice does not decide it", flow: ios,
			scope: []clarityrefs.Scope{scopeOf("ios", false, 30), scopeOf("ios", true, 20)}, want: false,
		},
		{
			name: "affected for one claimed target beats unaffected for another", flow: web,
			// A flow mid-rename claims both "" and "web". If either says the
			// commit is affected, it ships in that flow.
			scope: []clarityrefs.Scope{scopeOf("web", false, 10), scopeOf("frontend", true, 10)}, want: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := core.IsCandidate(c.scope, c.flow); got != c.want {
				t.Errorf("IsCandidate = %v, want %v", got, c.want)
			}
		})
	}
}

// TestDeriveView_CandidacyExcludesFromLeadTime is what candidacy is for. The
// ios flow ships one commit of its own and sweeps up a web-only commit that
// happened to be in the tree. Without candidacy that web commit contributes a
// lead time measured from its own authoring, and the ios average becomes the
// average age of the monorepo.
func TestDeriveView_CandidacyExcludesFromLeadTime(t *testing.T) {
	day := int64(86400)
	snap := core.Snapshot{
		Commits: []core.CommitView{
			{
				SHA: "ios-change", Subject: "ios-change", Author: "a", Time: time.Unix(9*day, 0),
				Events: []clarityrefs.Event{ciEv(9*day + 60), dep("ios", 9*day+120)},
				Scope:  []clarityrefs.Scope{scopeOf("ios", true, 9*day)},
			},
			{
				// Authored long before, ships nothing ios — but it is in the
				// tree that the ios deploy above built from.
				SHA: "web-only", Subject: "web-only", Author: "b", Time: time.Unix(1*day, 0),
				Events: []clarityrefs.Event{ciEv(1*day + 60)},
				Scope:  []clarityrefs.Scope{scopeOf("ios", false, 1*day)},
			},
		},
	}
	declared := []core.Flow{{Name: "ios", Targets: []string{"ios"}}}

	view := core.DeriveView(snap, core.DefaultLeadTimeMode, declared)
	flow := view.Flows[0]
	now := time.Unix(20*day, 0)

	// Both commits still appear — the intent is to stop one skewing the
	// number, not to hide that it shipped.
	if total := countCommits(flow.Groups); total != 2 {
		t.Errorf("candidacy removed a commit from the log: %d of 2 remain", total)
	}

	iosLead, _, iosOK := flow.Groups.LeadTime(0, now)
	_, _, webOK := flow.Groups.LeadTime(1, now)

	if !iosOK {
		t.Error("the ios-affecting commit lost its lead time")
	}
	if webOK {
		t.Error("a commit reported unaffected still contributes a lead time — " +
			"this is the monorepo-average bug candidacy exists to fix")
	}
	if iosLead <= 0 {
		t.Errorf("ios lead time is %v", iosLead)
	}
}

// Silence still means candidate, so a repo that reports no candidacy keeps
// exactly the numbers it had before the feature existed.
func TestDeriveView_NoCandidacyReportedChangesNothing(t *testing.T) {
	day := int64(86400)
	commits := []core.CommitView{
		{SHA: "a", Subject: "a", Author: "a", Time: time.Unix(9*day, 0),
			Events: []clarityrefs.Event{ciEv(9*day + 60), dep("ios", 9*day+120)}},
		{SHA: "b", Subject: "b", Author: "b", Time: time.Unix(1*day, 0),
			Events: []clarityrefs.Event{ciEv(1*day + 60)}},
	}
	declared := []core.Flow{{Name: "ios", Targets: []string{"ios"}}}

	view := core.DeriveView(core.Snapshot{Commits: commits}, core.DefaultLeadTimeMode, declared)
	now := time.Unix(20*day, 0)

	for i := range commits {
		if _, _, ok := view.Flows[0].Groups.LeadTime(i, now); !ok {
			t.Errorf("commit %d lost its lead time with no candidacy reported", i)
		}
	}
}

func countCommits(g core.Groupings) int {
	n := len(g.Head) + len(g.CIPassed)
	for _, b := range g.InFlight {
		n += len(b.Commits)
	}
	for _, b := range g.Deployed {
		n += len(b.Commits)
	}
	return n
}

// TestWeeklyStatsForFlow_ExcludesNonCandidates covers the other number
// candidacy protects. Deploy frequency counts batches and is unaffected, but
// the weekly average is a mean of per-commit lead times — so a swept-up
// commit drags it exactly as it drags the rows.
func TestWeeklyStatsForFlow_ExcludesNonCandidates(t *testing.T) {
	day := int64(86400)
	commits := []core.CommitView{
		{
			SHA: "ios", Subject: "ios", Author: "a", Time: time.Unix(9*day, 0),
			Events: []clarityrefs.Event{ciEv(9*day + 60), dep("ios", 9*day+120)},
			Scope:  []clarityrefs.Scope{scopeOf("ios", true, 9*day)},
		},
		{
			// Authored eight days earlier: if it counted, it would roughly
			// double the average.
			SHA: "web", Subject: "web", Author: "b", Time: time.Unix(1*day, 0),
			Events: []clarityrefs.Event{ciEv(1*day + 60)},
			Scope:  []clarityrefs.Scope{scopeOf("ios", false, 1*day)},
		},
	}
	snap := core.Snapshot{Commits: commits}
	flow := core.Flow{Name: "ios", Targets: []string{"ios"}}

	withCandidacy := core.WeeklyStatsForFlow(snap, core.DefaultLeadTimeMode, flow)
	without := core.WeeklyStatsMode(snap, core.DefaultLeadTimeMode)

	sum := func(ws []core.WeekStat) time.Duration {
		var d time.Duration
		for _, w := range ws {
			d += w.AvgLead
		}
		return d
	}

	if sum(withCandidacy) == 0 {
		t.Fatal("the candidate commit contributed no lead time at all")
	}
	if sum(withCandidacy) >= sum(without) {
		t.Errorf("candidacy did not reduce the weekly average: with=%v without=%v",
			sum(withCandidacy), sum(without))
	}
}

// TestIsCandidate_EqualTimestamps pins the one case "latest wins" cannot
// answer. Two records for a target at the same instant have no newer one, and
// without an explicit rule the winner is decided by whichever content hash
// sorts first — so a correction can be silently discarded while the command
// that wrote it reports success.
//
// The tie resolves to affected: counting a commit can only inflate a lead
// time, whereas resolving the other way could hide delivery, and a metric that
// flatters is the worse failure.
func TestIsCandidate_EqualTimestamps(t *testing.T) {
	ios := core.Flow{Name: "ios", Targets: []string{"ios"}}

	both := []clarityrefs.Scope{scopeOf("ios", false, 10), scopeOf("ios", true, 10)}
	reversed := []clarityrefs.Scope{scopeOf("ios", true, 10), scopeOf("ios", false, 10)}

	if !core.IsCandidate(both, ios) {
		t.Error("a tie resolved to unaffected")
	}
	if core.IsCandidate(both, ios) != core.IsCandidate(reversed, ios) {
		t.Error("a tie resolves differently depending on record order — " +
			"the winner is being decided by content hash, not by the data")
	}
}

// A correction with a later timestamp must actually take effect — that is the
// whole reason supersedence is by time.
func TestIsCandidate_LaterRecordCorrectsAnEarlierOne(t *testing.T) {
	ios := core.Flow{Name: "ios", Targets: []string{"ios"}}

	// Nanosecond resolution: two reports a microsecond apart are ordered.
	earlier := clarityrefs.Scope{Target: "ios", Affected: true, Time: time.Unix(10, 0)}
	later := clarityrefs.Scope{Target: "ios", Affected: false, Time: time.Unix(10, 1000)}

	if core.IsCandidate([]clarityrefs.Scope{earlier, later}, ios) {
		t.Error("a later correction to unaffected was ignored")
	}
	if core.IsCandidate([]clarityrefs.Scope{later, earlier}, ios) {
		t.Error("the correction depends on slice order")
	}
}
