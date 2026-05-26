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
)

// DefaultBranch is the trunk-branch name used when the file is absent or
// doesn't set `branch`. Matches the historical hard-coded value the
// watcher / refsource used before this loader existed.
const DefaultBranch = "main"

// Config is the parsed surface every caller in step 6 cares about.
// Later steps add Pushq and Clarity sub-shapes when their consumers
// land.
type Config struct {
	// Branch is the trunk every clarity-aware tool operates on.
	// Defaults to DefaultBranch when the file is absent or omits the
	// field.
	Branch string
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
		Branch string `json:"branch"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("parse .ezcd.json: %w", err)
	}
	if raw.Branch != "" {
		cfg.Branch = raw.Branch
	}
	return cfg, nil
}
