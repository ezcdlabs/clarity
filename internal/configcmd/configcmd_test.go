package configcmd_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
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

func commitClarityFile(t *testing.T, remote *gittest.Remote, content string) {
	t.Helper()
	writer := remote.NewClone(t)
	writer.WriteFile(config.ClarityFileName, content)
	writer.CommitAll("clarity: set up")
	writer.Push("main")
}

func readWorkingTreeFile(t *testing.T, repoPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoPath, config.ClarityFileName))
	if err != nil {
		t.Fatalf("read %s: %v", config.ClarityFileName, err)
	}
	return string(data)
}

// TestSet_RewritesTheWorkingTreeFileAndLeavesTheCommitToTheUser: a set is
// an edit to .clarity for the user to commit — it must not push anything,
// because the config lives on the trunk precisely so that landing a change
// goes through the same review as any other trunk change.
func TestSet_RewritesTheWorkingTreeFileAndLeavesTheCommitToTheUser(t *testing.T) {
	remote := gittest.NewRemote(t)
	commitClarityFile(t, remote, "[clarity]\n\tsource = events-ref\n")
	clone := remote.NewClone(t)
	trunkBefore := remote.LogBranch("main")

	out, err := run(t, clone.Path, "leadTime", "reported")
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	for _, want := range []string{"reported", config.ClarityFileName, "commit"} {
		if !strings.Contains(out, want) {
			t.Errorf("set output should mention %q:\n%s", want, out)
		}
	}

	cfg, err := config.ParseClarityConfig([]byte(readWorkingTreeFile(t, clone.Path)))
	if err != nil {
		t.Fatalf("the file set wrote does not parse: %v", err)
	}
	if cfg.LeadTimeMode() != core.LeadReported {
		t.Errorf("LeadTimeMode = %q after set, want reported", cfg.LeadTimeMode())
	}

	// Nothing shared moved.
	if after := remote.LogBranch("main"); len(after) != len(trunkBefore) {
		t.Errorf("set pushed to the trunk: %d commits -> %d", len(trunkBefore), len(after))
	}
	cmd := exec.Command("git", "rev-parse", "--verify", clarityrefs.ConfigRef)
	cmd.Dir = remote.Path
	if err := cmd.Run(); err == nil {
		t.Error("set created the legacy config ref")
	}
}

// TestSet_SeedsFromTheTrunkAndPreservesTheRestOfTheFile: setting one key
// starts from the trunk's file, so the deploy sections someone declared
// survive the edit.
func TestSet_SeedsFromTheTrunkAndPreservesTheRestOfTheFile(t *testing.T) {
	remote := gittest.NewRemote(t)
	commitClarityFile(t, remote, "[clarity]\n\tsource = events-ref\n[deploy \"web\"]\n\ttarget = web-eu\n[deploy \"ios\"]\n")
	clone := remote.NewClone(t)

	if _, err := run(t, clone.Path, "leadTime", "pipeline"); err != nil {
		t.Fatalf("set: %v", err)
	}
	cfg, err := config.ParseClarityConfig([]byte(readWorkingTreeFile(t, clone.Path)))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	flows := cfg.Deploys()
	if len(flows) != 2 || flows[0].Name != "web" || flows[1].Name != "ios" {
		t.Errorf("deploy declarations did not survive the edit: %+v", flows)
	}
	if strings.Join(flows[0].Targets, ",") != "web-eu" {
		t.Errorf("web's targets did not survive: %+v", flows[0].Targets)
	}
}

// TestSet_UncommittedEditsAreTheSeedWhenPresent: a working-tree .clarity
// is a change mid-flight; a set folds into it rather than silently
// discarding it for the trunk's older copy.
func TestSet_UncommittedEditsAreTheSeedWhenPresent(t *testing.T) {
	remote := gittest.NewRemote(t)
	commitClarityFile(t, remote, "[clarity]\n\tsource = events-ref\n")
	clone := remote.NewClone(t)
	// An uncommitted edit the trunk doesn't have yet.
	if err := os.WriteFile(filepath.Join(clone.Path, config.ClarityFileName),
		[]byte("[clarity]\n\tsource = events-ref\n[deploy \"android\"]\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := run(t, clone.Path, "leadTime", "all"); err != nil {
		t.Fatalf("set: %v", err)
	}
	cfg, err := config.ParseClarityConfig([]byte(readWorkingTreeFile(t, clone.Path)))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if flows := cfg.Deploys(); len(flows) != 1 || flows[0].Name != "android" {
		t.Errorf("the uncommitted deploy declaration was discarded: %+v", flows)
	}
	if cfg.LeadTimeMode() != core.LeadAll {
		t.Errorf("LeadTimeMode = %q, want all", cfg.LeadTimeMode())
	}
}

// TestSet_SeedsFromTheLegacyRefOnAnUnmigratedRepo: on a repo whose config
// still lives in refs/clarity/config, a set writes .clarity carrying the
// ref's choices plus the change — the set doubles as the migration.
func TestSet_SeedsFromTheLegacyRefOnAnUnmigratedRepo(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	refCfg := []byte("[clarity]\n\tsource = events-ref\n[deploy \"web\"]\n")
	if err := clarityrefs.WriteConfigFile(clone.Path, "origin", refCfg, "clarity: set up"); err != nil {
		t.Fatalf("seed ref: %v", err)
	}

	out, err := run(t, clone.Path, "leadTime", "reported")
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	cfg, err := config.ParseClarityConfig([]byte(readWorkingTreeFile(t, clone.Path)))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.LeadTimeMode() != core.LeadReported {
		t.Errorf("LeadTimeMode = %q, want reported", cfg.LeadTimeMode())
	}
	if flows := cfg.Deploys(); len(flows) != 1 || flows[0].Name != "web" {
		t.Errorf("the ref's deploy declaration did not carry over: %+v", flows)
	}
	if !strings.Contains(out, "commit") {
		t.Errorf("output should say the file needs committing:\n%s", out)
	}
}

// TestSet_InvalidValueChangesNothing: a typo must fail with the mode list
// and write no file — never leave a config the dashboard will refuse.
func TestSet_InvalidValueChangesNothing(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	_, err := run(t, clone.Path, "leadTime", "pipelien")
	if err == nil {
		t.Fatal("an invalid mode was accepted")
	}
	for _, want := range []string{"pipelien", "pipeline", "all", "reported"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q: %v", want, err)
		}
	}
	if _, statErr := os.Stat(filepath.Join(clone.Path, config.ClarityFileName)); statErr == nil {
		t.Error("a refused set still wrote .clarity")
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

// TestGet_ConfiguredAndDefault: one argument reads the key from the trunk —
// the configured value plainly, an unset one as the default, saying so.
func TestGet_ConfiguredAndDefault(t *testing.T) {
	remote := gittest.NewRemote(t)
	commitClarityFile(t, remote, "[clarity]\n\tsource = events-ref\n")
	clone := remote.NewClone(t)

	out, err := run(t, clone.Path, "leadTime")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.Contains(out, string(core.DefaultLeadTimeMode)) || !strings.Contains(out, "default") {
		t.Errorf("an unset key should print the default and say so, got: %q", out)
	}
}

// TestGet_ReadsTheTrunkNotTheWorkingTree: an uncommitted edit has not
// taken effect, and get must say what is in effect.
func TestGet_ReadsTheTrunkNotTheWorkingTree(t *testing.T) {
	remote := gittest.NewRemote(t)
	commitClarityFile(t, remote, "[clarity]\n\tleadTime = reported\n")
	clone := remote.NewClone(t)
	if err := os.WriteFile(filepath.Join(clone.Path, config.ClarityFileName),
		[]byte("[clarity]\n\tleadTime = all\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	out, err := run(t, clone.Path, "leadTime")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.Contains(out, "reported") {
		t.Errorf("get should read the trunk's value, got: %q", out)
	}
}

// TestShow_PrintsTheTrunkFile: no arguments prints the config in effect —
// the bytes committed on the trunk.
func TestShow_PrintsTheTrunkFile(t *testing.T) {
	remote := gittest.NewRemote(t)
	commitClarityFile(t, remote, "[clarity]\n\tsource = events-ref\n\tleadTime = reported\n")
	clone := remote.NewClone(t)

	out, err := run(t, clone.Path)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if !strings.Contains(out, "leadTime = reported") {
		t.Errorf("show should print the stored file, got:\n%s", out)
	}
}

// TestShow_LegacyRefComesWithAMigrationNote: an unmigrated repo still sees
// its config, plus where it should move.
func TestShow_LegacyRefComesWithAMigrationNote(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	refCfg := []byte("[clarity]\n\tleadTime = reported\n")
	if err := clarityrefs.WriteConfigFile(clone.Path, "origin", refCfg, "clarity: set up"); err != nil {
		t.Fatalf("seed ref: %v", err)
	}

	out, err := run(t, clone.Path)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if !strings.Contains(out, "leadTime = reported") {
		t.Errorf("show should print the ref's file, got:\n%s", out)
	}
	if !strings.Contains(out, config.ClarityFileName) {
		t.Errorf("show on an unmigrated repo should mention %s:\n%s", config.ClarityFileName, out)
	}
}

// TestShow_FreshRepoSaysSo: no config anywhere — defaults are in effect
// and init is the way in.
func TestShow_FreshRepoSaysSo(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	out, err := run(t, clone.Path)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if !strings.Contains(out, "init") {
		t.Errorf("show on a fresh repo should point at init, got:\n%s", out)
	}
}
