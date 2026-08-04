// Package config loads the optional `.ezcd.json` project file. The whole
// file is optional — a repo without one falls back to step-6 defaults
// (branch=main, no pushq integration, no clarity.github source).
//
// `.ezcd.json` is shared with pushq (and any future ezcd tools), so this
// package owns the shape but only surfaces the slice of it the current
// build understands. Unknown fields are silently accepted so an older
// build can read a newer config without choking.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ezcdlabs/clarity/internal/core"
)

// DefaultBranch is the trunk-branch name used when the file is absent or
// doesn't set `branch`. Matches the historical hard-coded value the
// watcher / refsource used before this loader existed.
const DefaultBranch = "main"

// Config is the parsed surface every caller in step 6+ cares about.
// Pushq (step 6) is intentionally absent until a clarity-side consumer
// exists; Clarity (step 7) is populated when .ezcd.json's `clarity`
// section is present.
type Config struct {
	// Branch is the trunk every clarity-aware tool operates on.
	// Defaults to DefaultBranch when the file is absent or omits the
	// field.
	Branch string

	// Clarity is the clarity-specific configuration. nil when the
	// `clarity` section is omitted from .ezcd.json — callers dispatch
	// on nil to mean "fall back to the events ref source".
	Clarity *ClarityConfig
}

// LeadTimeMode returns the configured lead time mode, falling back to the
// default when the file has no clarity section at all. Lives here rather than
// at the call site so every caller — the TUI, plain mode, tests — resolves it
// the same way.
func (c Config) LeadTimeMode() core.LeadTimeMode {
	if c.Clarity == nil {
		return core.DefaultLeadTimeMode
	}
	return c.Clarity.LeadTime
}

// Load reads `.ezcd.json` from repoRoot and returns its parsed Config.
// A missing file is not an error — the returned Config carries the
// step-6 defaults. A malformed file is an error, with the filename
// included so users know which file to fix.
func Load(repoRoot string) (Config, error) {
	cfg := Config{Branch: DefaultBranch}

	path := filepath.Join(repoRoot, ".ezcd.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("read .ezcd.json: %w", err)
	}

	// json.Decode-style: we only pull out the fields this build knows
	// about. Unknown top-level keys are silently ignored — that's the
	// forward-compat contract.
	var raw struct {
		Branch  string            `json:"branch"`
		Clarity *rawClarityConfig `json:"clarity"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("parse .ezcd.json: %w", err)
	}
	if raw.Branch != "" {
		cfg.Branch = raw.Branch
	}
	clarity, err := raw.Clarity.hydrate()
	if err != nil {
		return Config{}, fmt.Errorf("parse .ezcd.json: %w", err)
	}
	cfg.Clarity = clarity
	return cfg, nil
}
