package config

import (
	"bytes"
	"fmt"

	"github.com/ezcdlabs/clarity/internal/core"
	gitcfg "github.com/go-git/go-git/v5/plumbing/format/config"
)

// SourceEventsRef is the one events source this build understands: the
// repo's own refs/clarity/events, read through the refsource adapter.
// The clarity.source key exists so a future config can point somewhere
// else — another repository's ref, another backend — and an old binary
// reading it refuses loudly instead of silently rendering the wrong
// repo's events as this one's.
const SourceEventsRef = "events-ref"

// ParseClarityConfig parses the file stored at refs/clarity/config:config —
// git-config syntax, because that is the dialect git users already read and
// git tooling already edits:
//
//	[clarity]
//		source = events-ref
//		leadTime = all
//	[deploy "web"]
//		target = web-eu
//		target = web-us
//	[deploy "mobile"]
//
// A deploy subsection with no targets claims its own name, same as the
// legacy format. Unknown sections and keys are ignored (forward compat);
// an unknown clarity.source is not, because it means the events live
// somewhere this build cannot read.
//
// Branch is deliberately not a key: the trunk is detected from the
// repository itself (origin/HEAD, then the conventional names), and a
// config that could disagree with the repo would just be a second place
// for the truth to go stale.
func ParseClarityConfig(data []byte) (Config, error) {
	cfg := Defaults()
	if len(bytes.TrimSpace(data)) == 0 {
		return cfg, nil
	}

	decoded := gitcfg.New()
	if err := gitcfg.NewDecoder(bytes.NewReader(data)).Decode(decoded); err != nil {
		return Config{}, fmt.Errorf("parse clarity config: %w", err)
	}

	clarity := decoded.Section("clarity")
	switch src := clarity.Option("source"); src {
	case "", SourceEventsRef:
	default:
		return Config{}, fmt.Errorf(
			"clarity.source is %q, but this build only reads %q — reading events from elsewhere needs a newer git-clarity",
			src, SourceEventsRef)
	}
	if raw := clarity.Option("leadtime"); raw != "" {
		mode, err := core.ParseLeadTimeMode(raw)
		if err != nil {
			return Config{}, fmt.Errorf("clarity.leadTime: %w", err)
		}
		cfg.LeadTime = mode
	}

	var raws []rawDeploy
	for _, sub := range decoded.Section("deploy").Subsections {
		targets := sub.OptionAll("target")
		raws = append(raws, rawDeploy{
			Name:     sub.Name,
			Targets:  targets,
			explicit: len(targets) > 0,
		})
	}
	deploys, err := hydrateDeploys(raws)
	if err != nil {
		return Config{}, err
	}
	cfg.DeclaredDeploys = deploys
	return cfg, nil
}

// DefaultClarityConfigFile is the file `git clarity init` writes: the
// minimal config that marks a repo as set up. Everything it could say is
// already the default — it pins the source explicitly anyway, so the file
// a user eventually opens to edit shows them the shape of the dialect
// instead of an empty section.
func DefaultClarityConfigFile() []byte {
	return []byte("[clarity]\n\tsource = events-ref\n")
}
