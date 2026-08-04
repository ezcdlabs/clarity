package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/core"
)

// TestLoad_NoFile is the must-not-error path that protects every existing
// install: a repo without an .ezcd.json must keep working with the
// previous defaults (branch=main). If this ever returned an error, every
// non-configured user would break on the first config-aware build.
func TestLoad_NoFile(t *testing.T) {
	cfg, err := config.Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load with no file should not error, got %v", err)
	}
	if cfg.Branch != "main" {
		t.Errorf("expected default branch=main, got %q", cfg.Branch)
	}
}

// TestLoad_EmptyJSON tests an .ezcd.json that exists but is just `{}` —
// the user committed the file but hasn't filled in any fields yet (or
// they only have nested sections we don't care about at this layer).
// All top-level fields must fall back to defaults.
func TestLoad_EmptyJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".ezcd.json", `{}`)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load with empty JSON: %v", err)
	}
	if cfg.Branch != "main" {
		t.Errorf("expected default branch=main, got %q", cfg.Branch)
	}
}

// TestLoad_CustomBranch is the only functional knob step 6 adds — set a
// non-main branch in the file and confirm Load surfaces it so callers
// (refsource.Options.Branch, future pushq integration) can pick it up.
func TestLoad_CustomBranch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".ezcd.json", `{"branch": "trunk"}`)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Branch != "trunk" {
		t.Errorf("expected branch=trunk, got %q", cfg.Branch)
	}
}

// TestLoad_MalformedJSON is the only path where Load returns an error.
// Users will eventually edit this file by hand; a broken edit needs to
// surface as a clear error at startup, not a silent "use defaults".
func TestLoad_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".ezcd.json", `{"branch": "trunk"`) // missing closing brace

	_, err := config.Load(dir)
	if err == nil {
		t.Fatal("expected Load to error on malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), ".ezcd.json") {
		t.Errorf("expected error to reference the filename, got %v", err)
	}
}

// TestLoad_UnknownFields_Ignored: forward-compat. Step 6's loader only
// owns `branch`; later steps add `pushq` and `clarity` sections. A user
// running an older build against a newer config must not error out —
// loaders ignore fields they don't know about.
func TestLoad_UnknownFields_Ignored(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".ezcd.json", `{
		"branch": "main",
		"pushq":   { "test_command": "go test ./..." },
		"clarity": { "github": { "ci": { "workflow": "CI" } } }
	}`)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load with future-fields should not error, got %v", err)
	}
	if cfg.Branch != "main" {
		t.Errorf("expected branch=main, got %q", cfg.Branch)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
}

// TestLoad_LeadTime_Absent protects existing installs: a clarity section
// without a leadTime key must keep producing the numbers it always has,
// rather than silently switching anyone's DORA metrics on upgrade.
func TestLoad_LeadTime_Absent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".ezcd.json", `{"clarity": {}}`)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Clarity == nil {
		t.Fatal("expected a clarity section")
	}
	if cfg.Clarity.LeadTime != core.DefaultLeadTimeMode {
		t.Errorf("leadTime = %q, want the default %q",
			cfg.Clarity.LeadTime, core.DefaultLeadTimeMode)
	}
}

// TestLoad_LeadTime_Modes checks each documented value round-trips from the
// file into the typed mode callers dispatch on.
func TestLoad_LeadTime_Modes(t *testing.T) {
	cases := map[string]core.LeadTimeMode{
		"all":      core.LeadAll,
		"reported": core.LeadReported,
		"pipeline": core.LeadPipeline,
	}
	for raw, want := range cases {
		dir := t.TempDir()
		writeFile(t, dir, ".ezcd.json", `{"clarity": {"leadTime": "`+raw+`"}}`)

		cfg, err := config.Load(dir)
		if err != nil {
			t.Fatalf("Load(%q): %v", raw, err)
		}
		if cfg.Clarity.LeadTime != want {
			t.Errorf("leadTime %q parsed as %q, want %q", raw, cfg.Clarity.LeadTime, want)
		}
	}
}

// TestLoad_LeadTime_Invalid fails at load rather than falling back to a
// default. A typo in this key silently changes what the DORA numbers mean,
// which is exactly the kind of thing a user should be told about at the point
// they can still see the file they just edited.
func TestLoad_LeadTime_Invalid(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".ezcd.json", `{"clarity": {"leadTime": "pipelien"}}`)

	_, err := config.Load(dir)
	if err == nil {
		t.Fatal("expected an error for an unknown leadTime value")
	}
	for _, want := range []string{"leadTime", "pipelien", "pipeline"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}
