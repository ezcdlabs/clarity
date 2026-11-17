package initcmd_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/gittest"
	"github.com/ezcdlabs/clarity/internal/initcmd"
)

// TestRun_WritesTheWorkingTreeFile: init is now the act of writing a
// .clarity file for the user to commit — it needs no push permission and
// touches nothing shared. The remote gains no ref and no commit; the
// output says what was written and that committing it is what finishes
// setup.
func TestRun_WritesTheWorkingTreeFile(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	var out bytes.Buffer

	if err := initcmd.Run(initcmd.Options{RepoPath: clone.Path, Out: &out}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(clone.Path, config.ClarityFileName))
	if err != nil {
		t.Fatalf("init wrote no %s: %v", config.ClarityFileName, err)
	}
	cfg, err := config.ParseClarityConfig(data)
	if err != nil {
		t.Fatalf("the config init wrote does not parse: %v\n%s", err, data)
	}
	if len(cfg.Deploys()) != 0 {
		t.Errorf("init declared deploys: %+v", cfg.Deploys())
	}

	// Nothing shared moved: no config ref, and the trunk is where it was.
	cmd := exec.Command("git", "rev-parse", "--verify", clarityrefs.ConfigRef)
	cmd.Dir = remote.Path
	if err := cmd.Run(); err == nil {
		t.Error("init created the legacy config ref; it must only write the file")
	}

	for _, want := range []string{config.ClarityFileName, "commit", "push", "main"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output should mention %q:\n%s", want, out.String())
		}
	}
}

// TestRun_AlreadyOnTrunk_TouchesNothing: re-running init on a migrated
// repo reports it is set up and changes nothing — choices someone already
// made must survive a re-run.
func TestRun_AlreadyOnTrunk_TouchesNothing(t *testing.T) {
	remote := gittest.NewRemote(t)
	first := remote.NewClone(t)
	first.WriteFile(config.ClarityFileName, "[clarity]\n\tleadTime = reported\n")
	first.CommitAll("clarity: set up")
	first.Push("main")

	second := remote.NewClone(t)
	var out bytes.Buffer
	if err := initcmd.Run(initcmd.Options{RepoPath: second.Path, Out: &out}); err != nil {
		t.Fatalf("Run on a set-up repo: %v", err)
	}
	if !strings.Contains(out.String(), "already") {
		t.Errorf("output should say the repo is already set up:\n%s", out.String())
	}
	data, err := os.ReadFile(filepath.Join(second.Path, config.ClarityFileName))
	if err != nil || string(data) != "[clarity]\n\tleadTime = reported\n" {
		t.Errorf("init changed the existing file: %q, %v", data, err)
	}
}

// TestRun_WorkingTreeFileAlreadyWritten: a .clarity that was written but
// not committed yet is a setup mid-flight — init must not clobber it, and
// should repeat what remains to be done.
func TestRun_WorkingTreeFileAlreadyWritten(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile(config.ClarityFileName, "[clarity]\n\tleadTime = reported\n")

	var out bytes.Buffer
	if err := initcmd.Run(initcmd.Options{RepoPath: clone.Path, Out: &out}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(clone.Path, config.ClarityFileName))
	if string(data) != "[clarity]\n\tleadTime = reported\n" {
		t.Errorf("init overwrote the uncommitted file:\n%s", data)
	}
	if !strings.Contains(out.String(), "commit") {
		t.Errorf("output should say the file still needs committing:\n%s", out.String())
	}
}

// TestRun_MigratesTheLegacyRef: on a repo whose config still lives in
// refs/clarity/config, init writes .clarity with the ref's exact content —
// migration must carry choices over, not reset them to defaults.
func TestRun_MigratesTheLegacyRef(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	refCfg := []byte("[clarity]\n\tsource = events-ref\n\tleadTime = reported\n[deploy \"web\"]\n")
	if err := clarityrefs.WriteConfigFile(clone.Path, "origin", refCfg, "clarity: set up"); err != nil {
		t.Fatalf("seed ref config: %v", err)
	}

	var out bytes.Buffer
	if err := initcmd.Run(initcmd.Options{RepoPath: clone.Path, Out: &out}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(clone.Path, config.ClarityFileName))
	if err != nil {
		t.Fatalf("init wrote no %s: %v", config.ClarityFileName, err)
	}
	if string(data) != string(refCfg) {
		t.Errorf("migrated file = %q, want the ref's exact content %q", data, refCfg)
	}
	if !strings.Contains(out.String(), clarityrefs.ConfigRef) {
		t.Errorf("output should say the content came from the legacy ref:\n%s", out.String())
	}
}
