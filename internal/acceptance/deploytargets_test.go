package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/adapters/plain"
	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/core"
)

// twoFlowSnapshot is the shape deploy targets exist to handle: one trunk, two
// deployables, shipping at different rates.
//
// The newest commit has reached web but not ios. That single fact is what the
// whole feature has to get right — the *same commit* belongs in a different
// lifecycle section depending on which flow you are looking at. Any
// implementation that filters events wrongly, or lets one flow's deploy move
// another flow's boundary, renders it in the wrong section.
func twoFlowSnapshot() core.Snapshot {
	return core.Snapshot{
		RepoName: "clarity",
		Commits: []core.CommitView{
			{
				SHA:    "aaa1111111111111111111111111111111111111",
				Author: "alice", Subject: "ship the checkout fix", Time: time.Unix(1000, 0),
				Events: []clarityrefs.Event{
					{Stage: "ci", Status: "passed", Time: time.Unix(1100, 0)},
					// Untargeted deploy: belongs to the default flow.
					{Stage: "deploy", Status: "passed", Time: time.Unix(1200, 0)},
				},
			},
			{
				SHA:    "bbb2222222222222222222222222222222222222",
				Author: "bob", Subject: "bump the target sdk", Time: time.Unix(800, 0),
				Events: []clarityrefs.Event{
					{Stage: "ci", Status: "passed", Time: time.Unix(850, 0)},
					{Stage: "deploy", Status: "passed", Time: time.Unix(900, 0), Target: "ios"},
				},
			},
		},
	}
}

// renderDiscovered runs the full path with no flow declarations: .ezcd.json on
// disk, through config.Load and the Lens, out as the plain text a piped
// consumer reads.
func renderDiscovered(t *testing.T, snap core.Snapshot) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".ezcd.json"), []byte(`{"clarity": {}}`), 0o644); err != nil {
		t.Fatalf("write .ezcd.json: %v", err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	// nil flows: nothing is declared, so they are discovered from the events.
	// This is the zero-setup path — a repo that starts reporting a target gets
	// a flow for it without touching any configuration.
	lens := core.NewLens(&fakeSource{snap: snap}, cfg.LeadTimeMode(), nil)
	select {
	case v, ok := <-lens.Views(t.Context()):
		if !ok {
			t.Fatal("lens closed without emitting a view")
		}
		return plain.RenderSnapshot(snap.RepoName, v, time.Unix(9999, 0), plain.Options{})
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the lens to emit")
		return ""
	}
}

// blockFor returns the slice of output belonging to one flow: everything from
// that flow's "deploy: <name>" header up to the next one (or the end). The
// header line is what makes the plain output greppable per flow — without it a
// `grep ✗` hit could not be attributed to a deployable.
func blockFor(t *testing.T, out, flow string) string {
	t.Helper()
	marker := "deploy: " + flow
	start := strings.Index(out, marker)
	if start < 0 {
		t.Fatalf("no block rendered for flow %q in:\n%s", flow, out)
	}
	rest := out[start+len(marker):]
	if next := strings.Index(rest, "\ndeploy: "); next >= 0 {
		return rest[:next]
	}
	return rest
}

// sectionOf reports which lifecycle section a commit subject was rendered
// under within one flow's block.
func sectionOf(block, subject string) string {
	section := ""
	for _, line := range strings.Split(block, "\n") {
		switch {
		case strings.HasPrefix(line, "HEAD"):
			section = "HEAD"
		case strings.HasPrefix(line, "CI Passed"):
			section = "CI Passed"
		case strings.HasPrefix(line, "Deployed"):
			section = "Deployed"
		}
		if strings.Contains(line, subject) {
			return section
		}
	}
	return ""
}

// TestDeployTargets_EachFlowGroupsByItsOwnDeploys is the acceptance test for
// deploy targets: two flows discovered from the events, one commit that has
// shipped to only one of them, and the demand that each flow's rendering
// reflects only its own deploy events.
func TestDeployTargets_EachFlowGroupsByItsOwnDeploys(t *testing.T) {
	out := renderDiscovered(t, twoFlowSnapshot())

	// Both flows were discovered from the events alone. The untargeted one
	// keeps the name the header has always shown it under.
	web := blockFor(t, out, "deploy")
	ios := blockFor(t, out, "ios")

	// The newest commit shipped via the untargeted deploy, so that flow shows
	// it as deployed.
	if got := sectionOf(web, "ship the checkout fix"); got != "Deployed" {
		t.Errorf("default flow: newest commit in %q section, want Deployed\n%s", got, web)
	}

	// The same commit has *not* reached ios — ios's last deploy is older — so
	// it must render as still in flight there, not as shipped.
	if got := sectionOf(ios, "ship the checkout fix"); got == "Deployed" {
		t.Errorf("ios flow: newest commit rendered as Deployed, but no ios deploy has shipped it\n%s", ios)
	}

	// And the commit that did ship to ios is deployed there.
	if got := sectionOf(ios, "bump the target sdk"); got != "Deployed" {
		t.Errorf("ios flow: ios-targeted commit in %q section, want Deployed\n%s", got, ios)
	}
}

// TestDeployTargets_WeeklyThroughputIsPerFlowInTheOutput closes the seam the
// leadTime bug taught us about: core can compute per-flow throughput correctly
// and the renderer can still print the whole-repo number.
//
// The snapshot has exactly one deploy per flow, so the repo-wide count is two
// and each flow's is one. A renderer reading the wrong field prints "2
// deploys" under both blocks and every unit test still passes.
func TestDeployTargets_WeeklyThroughputIsPerFlowInTheOutput(t *testing.T) {
	out := renderDiscovered(t, twoFlowSnapshot())

	for _, flow := range []string{"deploy", "ios"} {
		block := blockFor(t, out, flow)
		if strings.Contains(block, "2 deploys") {
			t.Errorf("flow %s: rendered the repo-wide deploy count, not its own\n%s", flow, block)
		}
		if !strings.Contains(block, "1 deploy") {
			t.Errorf("flow %s: expected its own throughput of 1 deploy\n%s", flow, block)
		}
	}
}

// TestDeployTargets_HeaderCarriesEveryFlowsDeployStatus guards the header
// against the failure it exists to avoid: with several flows there is no such
// thing as "the repo's deploy status", so a single badge would have to pick
// one flow's answer and present it as everyone's.
//
// Here web is green and ios is red. A header showing one badge shows a lie
// whichever it picks.
func TestDeployTargets_HeaderCarriesEveryFlowsDeployStatus(t *testing.T) {
	snap := core.Snapshot{
		RepoName: "clarity",
		Commits: []core.CommitView{
			{
				SHA: "aaa1111111111111111111111111111111111111", Author: "alice",
				Subject: "ship the checkout fix", Time: time.Unix(1000, 0),
				Events: []clarityrefs.Event{
					{Stage: "ci", Status: "passed", Time: time.Unix(1100, 0)},
					{Stage: "deploy", Status: "passed", Time: time.Unix(1200, 0)},
					{Stage: "deploy", Status: "failed", Time: time.Unix(1300, 0), Target: "ios"},
				},
			},
		},
	}

	header := strings.SplitN(renderDiscovered(t, snap), "\n", 2)[0]

	if !strings.Contains(header, "deploy:") || !strings.Contains(header, "ios:") {
		t.Errorf("header does not name each flow's deploy status: %q", header)
	}
	if !strings.Contains(header, "failed") {
		t.Errorf("header hides the failing ios flow behind a passing one: %q", header)
	}
	if !strings.Contains(header, "passed") {
		t.Errorf("header lost the passing flow: %q", header)
	}
}

// renderDeclared is renderDiscovered's sibling for a repo that declares its
// flows: the same path, but with the `deploys` section actually reaching the
// Lens. Declaring is what turns the config into an expectation, so this is the
// path where order, naming and undeclared targets have to be checked.
func renderDeclared(t *testing.T, deploys string, snap core.Snapshot) string {
	t.Helper()

	dir := t.TempDir()
	body := `{"clarity": {"deploys": ` + deploys + `}}`
	if err := os.WriteFile(filepath.Join(dir, ".ezcd.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write .ezcd.json: %v", err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	lens := core.NewLens(&fakeSource{snap: snap}, cfg.LeadTimeMode(), cfg.Deploys())
	select {
	case v, ok := <-lens.Views(t.Context()):
		if !ok {
			t.Fatal("lens closed without emitting a view")
		}
		return plain.RenderSnapshot(snap.RepoName, v, time.Unix(9999, 0), plain.Options{})
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the lens to emit")
		return ""
	}
}

// TestDeployTargets_DeclaredFlowsReachTheRendering is the config-file half of
// the feature. Discovery already works without it, so what declaring has to
// buy is exactly three things, and each is asserted here: a chosen display
// name, a chosen order, and the folding of several targets into one flow.
func TestDeployTargets_DeclaredFlowsReachTheRendering(t *testing.T) {
	// "web" claims both the untargeted deploy and the "web" target — the
	// mid-rename case, where a flow's identity outlives the name its pipeline
	// reports. Declared ios second, though discovery would sort it first.
	deploys := `[{"name": "web", "targets": ["", "web"]}, {"name": "ios", "targets": ["ios"]}]`
	out := renderDeclared(t, deploys, twoFlowSnapshot())

	web := blockFor(t, out, "web")
	ios := blockFor(t, out, "ios")

	// The declared name reaches the output: no "deploy" block survives.
	if strings.Contains(out, "deploy: deploy") {
		t.Errorf("declared config still rendered the default flow name:\n%s", out)
	}

	// Declaration order is display order, not alphabetical and not
	// discovery order — otherwise a team cannot put its primary deploy first.
	if strings.Index(out, "deploy: web") > strings.Index(out, "deploy: ios") {
		t.Errorf("declared order not preserved — ios rendered before web:\n%s", out)
	}

	// The untargeted deploy was folded into web, so web still groups by it.
	if got := sectionOf(web, "ship the checkout fix"); got != "Deployed" {
		t.Errorf("web flow: newest commit in %q section, want Deployed\n%s", got, web)
	}
	if got := sectionOf(ios, "ship the checkout fix"); got == "Deployed" {
		t.Errorf("ios flow: newest commit rendered as Deployed, but no ios deploy shipped it\n%s", ios)
	}
}

// TestDeployTargets_UndeclaredTargetsStillSurface guards the failure mode that
// matters most: a deploy that nobody declared must never be silently dropped.
// A typo in a pipeline should show up as a surprise in the output, not as
// missing data.
func TestDeployTargets_UndeclaredTargetsStillSurface(t *testing.T) {
	deploys := `[{"name": "web", "targets": ["", "web"]}]`
	out := renderDeclared(t, deploys, twoFlowSnapshot())

	ios := blockFor(t, out, "ios")
	if got := sectionOf(ios, "bump the target sdk"); got != "Deployed" {
		t.Errorf("undeclared ios flow: commit in %q section, want Deployed\n%s", got, ios)
	}
	if !strings.Contains(out, "(undeclared)") {
		t.Errorf("undeclared flow not marked as such:\n%s", out)
	}
}
