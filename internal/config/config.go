// Package config holds clarity's project configuration and the formats it
// arrives in.
//
// The config of record is `.clarity` — a git-config-syntax file (see
// ParseClarityConfig) committed at the root of the repository and read from
// the trunk's tip, never from the working tree. On the trunk it is covered
// by review, branch protection and CODEOWNERS, which matters because the
// config decides things like where events come from; a change to it is an
// ordinary commit in `git log` and blame rather than a silent write to a
// side ref.
//
// Two legacy formats are still read as fallbacks so existing repos keep
// working: the same dialect stored in the `refs/clarity/config` ref (the
// first home of the config, walked back because hosting platforms cannot
// protect custom refs), and the original committed `.ezcd.json`. New fields
// land only in the `.clarity` format.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ezcdlabs/clarity/internal/core"
)

// DefaultBranch is the trunk-branch name used when neither detection nor any
// configuration yields one. Matches the historical hard-coded value the
// watcher / refsource used before this loader existed.
const DefaultBranch = "main"

// FileName is the legacy configuration file, at the root of the repository.
// Named here so a client reading it out of a fetched tree asks for the same
// path a working copy would.
const FileName = ".ezcd.json"

// ClarityFileName is the configuration file of record: committed at the
// root of the repository, read from the trunk's tip. Named here so every
// reader — the CLI reading a ref's tree, the mobile client reading a
// fetched commit — asks for the same path.
const ClarityFileName = ".clarity"

// Config is the parsed surface every caller cares about, whichever format
// it was read from.
type Config struct {
	// Branch is the trunk every clarity-aware tool operates on. The config
	// formats no longer carry a branch of their own (legacy .ezcd.json
	// excepted): callers resolve it from the repository itself and fill
	// this field in.
	Branch string

	// BranchPinned reports whether Branch came from a legacy .ezcd.json
	// rather than the default. A pinned branch wins over detection, so a
	// repo that explicitly named its trunk keeps it even when origin/HEAD
	// disagrees. The ref format deliberately has no branch key.
	BranchPinned bool

	// SetUp reports whether clarity has been set up on this repo: .clarity
	// on the trunk, the legacy refs/clarity/config, or a working-tree
	// .clarity whose commit is presumably in flight. The TUI's welcome
	// screen keys off it. Filled by the reader, not the parser: it is a
	// fact about the repository, not about the bytes. Only the trunk's
	// .clarity (or the legacy ref) contributes values; a working-tree file
	// flips this bit and nothing else.
	SetUp bool

	// Notice is a one-line situational message for surfaces that can show
	// one — plain mode's stderr, `git clarity config`'s output: the config
	// still lives in the legacy ref, or a working-tree .clarity has not
	// been committed yet. Empty on a fully migrated repo. Never printed by
	// `check` (silent on green is its contract) or mid-frame in the TUI.
	Notice string

	// LegacyFile reports whether this Config came from an .ezcd.json that
	// actually exists. "Configured the old way" must not land on the
	// welcome screen — the repo works, it just hasn't migrated — so the
	// welcome keys off SetUp and this both being false.
	LegacyFile bool

	// LeadTime selects which commits contribute a lead time and what it is
	// measured from. Empty means core.DefaultLeadTimeMode — read it through
	// LeadTimeMode(), which resolves the default in one place.
	LeadTime core.LeadTimeMode

	// DeclaredDeploys are the declared deploy flows, in declaration order.
	// Nil when the config declares none, which means flows are discovered
	// from the events instead — declaring them is what turns the config
	// into an expectation that a missing or unexpected flow can be
	// measured against.
	DeclaredDeploys []core.Flow
}

// LeadTimeMode returns the configured lead time mode, falling back to the
// default when none was configured. Lives here rather than at the call site
// so every caller — the TUI, plain mode, tests — resolves it the same way.
func (c Config) LeadTimeMode() core.LeadTimeMode {
	if c.LeadTime == "" {
		return core.DefaultLeadTimeMode
	}
	return c.LeadTime
}

// Load reads the legacy `.ezcd.json` from repoRoot and returns its parsed
// Config. A missing file is not an error — the returned Config carries the
// defaults. A malformed file is an error, with the filename included so
// users know which file to fix.
func Load(repoRoot string) (Config, error) {
	path := filepath.Join(repoRoot, FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Defaults(), nil
		}
		return Config{}, fmt.Errorf("read .ezcd.json: %w", err)
	}
	cfg, err := Parse(data)
	if err == nil {
		cfg.LegacyFile = true
	}
	return cfg, err
}

// Defaults is the configuration of a repo with no configuration at all,
// which is the ordinary case and not an error.
func Defaults() Config { return Config{Branch: DefaultBranch} }

// Parse turns the bytes of a legacy .ezcd.json into a Config.
//
// Separate from Load because where those bytes come from is not fixed: a
// working tree through Load, or a blob read straight out of a fetched
// commit's tree, which is how a client with no checkout gets it.
//
// Empty input is the same as a missing file: a repo that has not configured
// anything. Unknown fields — including the clarity.github section that
// older builds understood — are silently ignored; that is the
// forward-compat contract, and for github it is also the retirement plan.
func Parse(data []byte) (Config, error) {
	cfg := Defaults()
	if len(bytes.TrimSpace(data)) == 0 {
		return cfg, nil
	}

	var raw struct {
		Branch  string            `json:"branch"`
		Clarity *rawClarityConfig `json:"clarity"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("parse .ezcd.json: %w", err)
	}
	if raw.Branch != "" {
		cfg.Branch = raw.Branch
		cfg.BranchPinned = true
	}
	if raw.Clarity != nil {
		mode, err := core.ParseLeadTimeMode(raw.Clarity.LeadTime)
		if err != nil {
			return Config{}, fmt.Errorf("parse .ezcd.json: clarity.leadTime: %w", err)
		}
		cfg.LeadTime = mode
		deploys, err := hydrateDeploys(raw.Clarity.Deploys)
		if err != nil {
			return Config{}, fmt.Errorf("parse .ezcd.json: %w", err)
		}
		cfg.DeclaredDeploys = deploys
	}
	return cfg, nil
}

// rawClarityConfig is the legacy file's `clarity` section wire shape. The
// github key older builds read here is gone — the GitHub Actions source was
// retired — and json.Unmarshal ignores it like any other unknown field.
type rawClarityConfig struct {
	LeadTime string      `json:"leadTime"`
	Deploys  []rawDeploy `json:"deploys"`
}
