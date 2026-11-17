// Package configload resolves the configuration of record for a repository.
//
// The config of record is `.clarity` at the repository root, read from the
// trunk's tip — the shared, committed truth — never from the working tree.
// Two legacy stores are honoured as fallbacks so existing repos keep
// working: the `refs/clarity/config` ref (the config's first home, walked
// back because hosting platforms cannot protect custom refs), and the
// original committed `.ezcd.json`. Precedence is strict and nothing is
// merged: two half-applied configs would be undebuggable.
package configload

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/gitlog"
)

// Origin says where Stored found the config of record.
type Origin int

const (
	// OriginNone: no .clarity on the trunk and no legacy config ref. The
	// repo may still carry a legacy .ezcd.json, which Load handles; the
	// get/set surface does not edit that format.
	OriginNone Origin = iota
	// OriginTrunkFile: .clarity at the trunk's tip — the config of record.
	OriginTrunkFile
	// OriginConfigRef: the legacy refs/clarity/config, honoured until the
	// repo migrates.
	OriginConfigRef
)

// Stored returns the raw bytes of the stored clarity config and where they
// came from: the trunk's .clarity when it exists, else the legacy config
// ref (fetched best-effort first, so offline answers with the last fetch).
// OriginNone with a nil error is a repo with neither, which is most repos.
func Stored(repoPath, branch string) ([]byte, Origin, error) {
	data, ok, err := clarityrefs.ReadTrunkFile(repoPath, branch, config.ClarityFileName)
	if err != nil {
		return nil, OriginNone, err
	}
	if !ok {
		// No local ref for the trunk at all — the shape of a CI job checked
		// out from a tag, which may be exactly the job reporting deploys, so
		// concluding "no config" here would silently switch declared-target
		// enforcement off. Fetch just the tip (best-effort; offline keeps
		// the no-config answer) and look again. A repo that HAS a trunk ref
		// is never fetched here: its ref is the answer, stale or not, and a
		// depth-1 fetch would shallow-mark a developer's full clone.
		if has, hasErr := clarityrefs.HasTrunkRef(repoPath, branch); hasErr == nil && !has {
			if clarityrefs.FetchTrunkTip(repoPath, "origin", branch) == nil {
				data, ok, err = clarityrefs.ReadTrunkFile(repoPath, branch, config.ClarityFileName)
				if err != nil {
					return nil, OriginNone, err
				}
			}
		}
	}
	if ok {
		return data, OriginTrunkFile, nil
	}
	_ = clarityrefs.FetchConfigRef(repoPath, "origin")
	data, ok, err = clarityrefs.ReadConfigFile(repoPath)
	if err != nil {
		return nil, OriginNone, err
	}
	if ok {
		return data, OriginConfigRef, nil
	}
	return nil, OriginNone, nil
}

// Load resolves the configuration every command operates on.
//
// The trunk is detected from the repository itself first (origin/HEAD,
// then main, then master — gitlog.DetectTrunk), because the trunk is where
// the config of record lives; only a legacy .ezcd.json's pinned branch
// still overrides it afterwards. Then, in strict precedence: .clarity at
// the trunk tip, the legacy config ref (with a migration Notice), the
// legacy .ezcd.json, defaults.
//
// A .clarity that exists only in the working tree contributes no values —
// checking out a colleague's branch must not change what clarity runs —
// but it does count as set up, so the welcome pane doesn't greet the
// person whose setup commit is in flight; the Notice says what remains.
func Load(repoPath string) (config.Config, error) {
	detected := gitlog.DetectTrunk(repoPath)
	lookup := detected
	if lookup == "" {
		lookup = config.DefaultBranch
	}

	data, origin, err := Stored(repoPath, lookup)
	if err != nil {
		return config.Config{}, err
	}
	var cfg config.Config
	switch origin {
	case OriginTrunkFile:
		if cfg, err = config.ParseClarityConfig(data); err != nil {
			return config.Config{}, fmt.Errorf("%s on %s: %w", config.ClarityFileName, lookup, err)
		}
		cfg.SetUp = true
	case OriginConfigRef:
		if cfg, err = config.ParseClarityConfig(data); err != nil {
			return config.Config{}, fmt.Errorf("%s: %w", clarityrefs.ConfigRef, err)
		}
		cfg.SetUp = true
		cfg.Notice = fmt.Sprintf("clarity's config still lives in the legacy %s — run `git clarity init` to write %s, then commit and push it to %s",
			clarityrefs.ConfigRef, config.ClarityFileName, lookup)
	default:
		if cfg, err = config.Load(repoPath); err != nil {
			return cfg, err
		}
	}

	if !cfg.SetUp {
		if _, statErr := os.Stat(filepath.Join(repoPath, config.ClarityFileName)); statErr == nil {
			cfg.SetUp = true
			cfg.Notice = fmt.Sprintf("%s is in the working tree but not on %s yet — commit and push it to finish setup (its values apply once it lands on the trunk)",
				config.ClarityFileName, lookup)
		} else if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			return config.Config{}, fmt.Errorf("stat %s: %w", config.ClarityFileName, statErr)
		}
	}

	if !cfg.BranchPinned && detected != "" {
		cfg.Branch = detected
	}
	return cfg, nil
}
