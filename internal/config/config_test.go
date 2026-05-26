package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/internal/config"
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
