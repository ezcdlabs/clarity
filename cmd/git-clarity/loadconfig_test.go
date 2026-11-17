package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// TestLoadConfig_TrunkFileIsTheConfigOfRecord: with .clarity committed on
// the trunk, its values apply and both legacy stores — the config ref and
// .ezcd.json — are ignored rather than merged, whatever they say.
func TestLoadConfig_TrunkFileIsTheConfigOfRecord(t *testing.T) {
	remote := gittest.NewRemote(t)
	writer := remote.NewClone(t)
	writer.WriteFile(".clarity", "[clarity]\n\tleadTime = reported\n")
	writer.CommitAll("add .clarity")
	writer.Push("main")

	clone := remote.NewClone(t)
	writeFile(t, clone.Path, ".ezcd.json", `{"clarity": {"leadTime": "pipeline"}}`)
	refCfg := []byte("[clarity]\n\tleadTime = all\n")
	if err := clarityrefs.WriteConfigFile(clone.Path, "origin", refCfg, "clarity: set up"); err != nil {
		t.Fatalf("seed ref config: %v", err)
	}

	cfg, err := loadConfig(clone.Path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !cfg.SetUp {
		t.Error("SetUp = false with .clarity on the trunk")
	}
	if cfg.LeadTimeMode() != core.LeadReported {
		t.Errorf("LeadTimeMode = %q, want %q from .clarity on the trunk", cfg.LeadTimeMode(), core.LeadReported)
	}
}

// TestLoadConfig_WorkingTreeClarityContributesNoValues: the config is read
// from the trunk, never the working tree — editing .clarity locally (or
// checking out a branch with a different one) must not change what clarity
// runs.
func TestLoadConfig_WorkingTreeClarityContributesNoValues(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	writeFile(t, clone.Path, ".clarity", "[clarity]\n\tleadTime = all\n")

	cfg, err := loadConfig(clone.Path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.LeadTimeMode() != core.DefaultLeadTimeMode {
		t.Errorf("LeadTimeMode = %q from a working-tree .clarity, want the %q default",
			cfg.LeadTimeMode(), core.DefaultLeadTimeMode)
	}
	if !cfg.SetUp {
		t.Error("SetUp = false — an uncommitted .clarity should still suppress the welcome pane")
	}
}

// TestLoadConfig_RefConfigWinsAndMarksSetUp: without a .clarity on the
// trunk, the legacy refs/clarity/config is still honoured — SetUp reports
// it, its values apply, and a lingering legacy .ezcd.json is ignored
// rather than merged.
func TestLoadConfig_RefConfigWinsAndMarksSetUp(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	writeFile(t, clone.Path, ".ezcd.json", `{"clarity": {"leadTime": "pipeline"}}`)
	refCfg := []byte("[clarity]\n\tsource = events-ref\n\tleadTime = reported\n")
	if err := clarityrefs.WriteConfigFile(clone.Path, "origin", refCfg, "clarity: set up"); err != nil {
		t.Fatalf("seed ref config: %v", err)
	}

	cfg, err := loadConfig(clone.Path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !cfg.SetUp {
		t.Error("SetUp = false with refs/clarity/config present")
	}
	if cfg.LeadTimeMode() != core.LeadReported {
		t.Errorf("LeadTimeMode = %q, want %q from the ref config (not the legacy file)", cfg.LeadTimeMode(), core.LeadReported)
	}
	if cfg.Branch != "main" {
		t.Errorf("Branch = %q, want detected main", cfg.Branch)
	}
}

// TestLoadConfig_LegacyFileIsTheFallback: a repo with only an .ezcd.json
// keeps working — its values load, but it does not count as set up, because
// setup is the ref's existence.
func TestLoadConfig_LegacyFileIsTheFallback(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	writeFile(t, clone.Path, ".ezcd.json", `{"clarity": {"leadTime": "pipeline"}}`)

	cfg, err := loadConfig(clone.Path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.SetUp {
		t.Error("SetUp = true without refs/clarity/config")
	}
	if cfg.LeadTimeMode() != core.LeadPipeline {
		t.Errorf("LeadTimeMode = %q, want %q from the legacy file", cfg.LeadTimeMode(), core.LeadPipeline)
	}
}

// TestLoadConfig_ZeroConfigDetectsTrunk: nothing configured anywhere — the
// trunk still resolves from the repository itself.
func TestLoadConfig_ZeroConfigDetectsTrunk(t *testing.T) {
	remote := gittest.NewRemoteOnBranch(t, "master")
	clone := remote.NewClone(t)

	cfg, err := loadConfig(clone.Path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.SetUp {
		t.Error("SetUp = true on a fresh repo")
	}
	if cfg.Branch != "master" {
		t.Errorf("Branch = %q, want detected master", cfg.Branch)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
