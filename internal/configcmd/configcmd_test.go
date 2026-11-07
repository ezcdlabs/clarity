package configcmd_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/configcmd"
	"github.com/ezcdlabs/clarity/internal/core"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

func run(t *testing.T, repoPath string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := configcmd.Run(configcmd.Options{RepoPath: repoPath, Out: &out, Args: args})
	return out.String(), err
}

// TestSet_RoundTripsThroughTheRemote: the whole point — one command sets
// leadTime, and every other clone reads the new value after a fetch.
func TestSet_RoundTripsThroughTheRemote(t *testing.T) {
	remote := gittest.NewRemote(t)
	setter := remote.NewClone(t)
	reader := remote.NewClone(t)

	if err := clarityrefs.WriteConfigFile(setter.Path, "origin", config.DefaultClarityConfigFile(), "clarity: set up"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	out, err := run(t, setter.Path, "leadTime", "reported")
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if !strings.Contains(out, "reported") || !strings.Contains(out, "refs/clarity/config") {
		t.Errorf("set output should say what was written where:\n%s", out)
	}

	if err := clarityrefs.FetchConfigRef(reader.Path, "origin"); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	data, ok, err := clarityrefs.ReadConfigFile(reader.Path)
	if err != nil || !ok {
		t.Fatalf("read back: ok=%v err=%v", ok, err)
	}
	cfg, err := config.ParseClarityConfig(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.LeadTimeMode() != core.LeadReported {
		t.Errorf("LeadTimeMode = %q, want reported", cfg.LeadTimeMode())
	}

	// The change chained onto the setup commit — the audit trail the ref
	// format promises.
	cmd := exec.Command("git", "rev-list", "--count", clarityrefs.ConfigRef)
	cmd.Dir = remote.Path
	countOut, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-list: %v", err)
	}
	if got := strings.TrimSpace(string(countOut)); got != "2" {
		t.Errorf("config ref history = %s commits, want 2", got)
	}
}

// TestSet_PreservesTheRestOfTheFile: setting one key must not eat the
// deploy sections someone declared.
func TestSet_PreservesTheRestOfTheFile(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	seed := []byte("[clarity]\n\tsource = events-ref\n[deploy \"web\"]\n\ttarget = web-eu\n[deploy \"ios\"]\n")
	if err := clarityrefs.WriteConfigFile(clone.Path, "origin", seed, "clarity: set up"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := run(t, clone.Path, "leadTime", "pipeline"); err != nil {
		t.Fatalf("set: %v", err)
	}

	if err := clarityrefs.FetchConfigRef(clone.Path, "origin"); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	data, _, err := clarityrefs.ReadConfigFile(clone.Path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	cfg, err := config.ParseClarityConfig(data)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, data)
	}
	flows := cfg.Deploys()
	if len(flows) != 2 || flows[0].Name != "web" || flows[1].Name != "ios" {
		t.Errorf("deploy declarations did not survive the edit: %+v\n%s", flows, data)
	}
	if strings.Join(flows[0].Targets, ",") != "web-eu" {
		t.Errorf("web's targets did not survive: %+v", flows[0].Targets)
	}
}

// TestSet_InvalidValueChangesNothing: a typo must fail with the mode list
// and leave the ref exactly where it was — never land in shared config.
func TestSet_InvalidValueChangesNothing(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	if err := clarityrefs.WriteConfigFile(clone.Path, "origin", config.DefaultClarityConfigFile(), "clarity: set up"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	before := refSHA(t, remote.Path)

	_, err := run(t, clone.Path, "leadTime", "pipelien")
	if err == nil {
		t.Fatal("an invalid mode was accepted")
	}
	for _, want := range []string{"pipelien", "pipeline", "all", "reported"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q: %v", want, err)
		}
	}
	if after := refSHA(t, remote.Path); after != before {
		t.Errorf("a refused set still moved the ref: %s -> %s", before, after)
	}
}

// TestSet_UnknownKeyIsRefused: reading forward-compatibly is right, but
// writing a key this build cannot validate is how typos become permanent.
func TestSet_UnknownKeyIsRefused(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	_, err := run(t, clone.Path, "leedTime", "pipeline")
	if err == nil {
		t.Fatal("an unknown key was accepted")
	}
	for _, want := range []string{"leedTime", "leadTime", "source"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q: %v", want, err)
		}
	}
}

// TestGet_ConfiguredAndDefault: one argument reads the key — the
// configured value plainly, an unset one as the default, saying so.
func TestGet_ConfiguredAndDefault(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	if err := clarityrefs.WriteConfigFile(clone.Path, "origin", config.DefaultClarityConfigFile(), "clarity: set up"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	out, err := run(t, clone.Path, "leadTime")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.Contains(out, string(core.DefaultLeadTimeMode)) || !strings.Contains(out, "default") {
		t.Errorf("an unset key should print the default and say so, got: %q", out)
	}

	if _, err := run(t, clone.Path, "leadTime", "all"); err != nil {
		t.Fatalf("set: %v", err)
	}
	out, err = run(t, clone.Path, "leadTime")
	if err != nil {
		t.Fatalf("get after set: %v", err)
	}
	if !strings.Contains(out, "all") || strings.Contains(out, "default") {
		t.Errorf("a configured key should print its value without the default note, got: %q", out)
	}
}

// TestShow_PrintsTheFile: no arguments prints the config as stored, the
// same bytes `git cat-file` would show.
func TestShow_PrintsTheFile(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	seed := []byte("[clarity]\n\tsource = events-ref\n\tleadTime = reported\n")
	if err := clarityrefs.WriteConfigFile(clone.Path, "origin", seed, "clarity: set up"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	out, err := run(t, clone.Path)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if !strings.Contains(out, "leadTime = reported") {
		t.Errorf("show should print the stored file, got:\n%s", out)
	}
}

// TestSet_OnAFreshRepoAlsoSetsItUp: writing config to a repo with no ref
// creates the ref — which IS setup — and the output says so instead of
// leaving the user to discover it.
func TestSet_OnAFreshRepoAlsoSetsItUp(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	out, err := run(t, clone.Path, "leadTime", "all")
	if err != nil {
		t.Fatalf("set on fresh repo: %v", err)
	}
	if !strings.Contains(out, "set up") {
		t.Errorf("output should say the repo was set up as a side effect:\n%s", out)
	}

	cmd := exec.Command("git", "rev-parse", "--verify", clarityrefs.ConfigRef)
	cmd.Dir = remote.Path
	if err := cmd.Run(); err != nil {
		t.Fatalf("remote has no config ref after the set: %v", err)
	}
}

func refSHA(t *testing.T, repoPath string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", clarityrefs.ConfigRef)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	return strings.TrimSpace(string(out))
}
