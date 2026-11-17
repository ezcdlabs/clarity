package config

import (
	"bytes"
	"fmt"
	"time"

	"github.com/ezcdlabs/clarity/internal/core"
	gitcfg "github.com/go-git/go-git/v5/plumbing/format/config"
)

// SourceEventsRef is the default events source: the repo's own
// refs/clarity/events, read through the refsource adapter. The
// clarity.source key is the dispatch point other sources plug into, and
// an old binary reading a value it does not know refuses loudly instead
// of silently rendering the wrong repo's events as this one's.
const SourceEventsRef = "events-ref"

// SourceExecName selects the exec source: a configured local command
// clarity runs and reads events from — the credential-helper pattern, so
// clarity stays transport-agnostic and each team brings a helper for its
// own store. The command's argv lives in the `[source]` section as
// repeated `arg =` lines; there is deliberately no single command string,
// because an argv list leaves nothing for a shell to interpret.
const SourceExecName = "exec"

// DefaultExecTimeout bounds one helper run when the config names none. A
// helper is a one-shot answering for the commits on screen; ten seconds is
// generous for that and short enough that a hung helper reads as a failed
// poll rather than a hung dashboard.
const DefaultExecTimeout = 10 * time.Second

// ExecSource is the parsed `[source]` section for `clarity.source = exec`:
// the exact argument list to exec — no shell, ever — and the per-run
// timeout.
type ExecSource struct {
	Argv    []string
	Timeout time.Duration
}

// ParseClarityConfig parses the `.clarity` file (and the same dialect in
// the legacy refs/clarity/config) — git-config syntax, because that is the
// dialect git users already read and git tooling already edits:
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
	case SourceExecName:
		exec, err := parseExecSource(decoded)
		if err != nil {
			return Config{}, err
		}
		cfg.SourceExec = exec
	default:
		return Config{}, fmt.Errorf(
			"clarity.source is %q, but this build only reads %q or %q — reading events from elsewhere needs a newer git-clarity",
			src, SourceEventsRef, SourceExecName)
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

// parseExecSource reads the `[source]` section for clarity.source = exec.
// An empty argv is an error rather than a default: there is no command
// clarity could sensibly invent, and a config that says "exec" and names
// nothing is a config mid-edit.
func parseExecSource(decoded *gitcfg.Config) (*ExecSource, error) {
	section := decoded.Section("source")
	argv := section.OptionAll("arg")
	if len(argv) == 0 {
		return nil, fmt.Errorf("clarity.source is %q but the [source] section has no `arg = ` lines — the command to run is the argv, one arg per line", SourceExecName)
	}
	out := &ExecSource{Argv: argv, Timeout: DefaultExecTimeout}
	if raw := section.Option("timeout"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("source.timeout %q: want a positive Go duration such as 10s", raw)
		}
		out.Timeout = d
	}
	return out, nil
}

// DefaultClarityConfigFile is the file `git clarity init` writes: the
// minimal config that marks a repo as set up. Everything it could say is
// already the default — it pins the source explicitly anyway, so the file
// a user eventually opens to edit shows them the shape of the dialect
// instead of an empty section.
func DefaultClarityConfigFile() []byte {
	return []byte("[clarity]\n\tsource = events-ref\n")
}
