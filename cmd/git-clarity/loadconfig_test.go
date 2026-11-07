package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// TestLoadConfig_RefConfigWinsAndMarksSetUp: when refs/clarity/config
// exists it is the config of record — SetUp reports it, its values apply,
// and a lingering legacy .ezcd.json is ignored rather than merged.
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
