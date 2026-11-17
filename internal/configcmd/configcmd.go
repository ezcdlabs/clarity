// Package configcmd implements `git clarity config`: the get/set surface
// over the `.clarity` file, so changing the lead time mode is one command
// instead of a hand edit.
//
//	git clarity config                      # print the stored file
//	git clarity config leadTime             # read one key
//	git clarity config leadTime pipeline    # rewrite .clarity for you to commit
//
// Reads answer from the trunk — the config in effect — honouring the
// legacy refs/clarity/config until the repo migrates. A set rewrites the
// working-tree .clarity and leaves the commit to the user: the config
// lives on the trunk precisely so that changing it goes through the same
// review as any other trunk change, and a tool that pushed there itself
// would bypass exactly that.
//
// Writes are stricter than reads on purpose. The read path ignores keys it
// doesn't know (forward compat: an old binary must survive a newer config),
// but this command refuses to *write* a key this build cannot validate —
// writing forward-compatibly is how typos become permanent shared state.
// For the same reason a set re-parses the whole resulting file before
// anything is written.
package configcmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/configload"
	"github.com/ezcdlabs/clarity/internal/core"
	"github.com/ezcdlabs/clarity/internal/gitlog"
	gitcfg "github.com/go-git/go-git/v5/plumbing/format/config"
)

// knownKeys maps a lowercased key to its canonical spelling. Only keys in
// this map can be set; [deploy "..."] sections are deliberately absent —
// declaring flows is a structural edit, not a key write, and stays a
// hand edit (or a future subcommand) for now.
var knownKeys = map[string]string{
	"leadtime": "leadTime",
	"source":   "source",
}

// Options configures Run.
type Options struct {
	RepoPath string
	Out      io.Writer // required
	Args     []string  // [], [key] or [key value]
}

// Run dispatches on argument count: show, get, or set.
func Run(opts Options) error {
	switch len(opts.Args) {
	case 0:
		return show(opts)
	case 1:
		return get(opts, opts.Args[0])
	case 2:
		return set(opts, opts.Args[0], opts.Args[1])
	default:
		return fmt.Errorf("usage: git clarity config [<key> [<value>]]")
	}
}

// trunk names the branch the config of record is read from.
func trunk(repoPath string) string {
	if b := gitlog.DetectTrunk(repoPath); b != "" {
		return b
	}
	return config.DefaultBranch
}

// show prints the stored file — the config in effect, which is the trunk's
// .clarity (or the legacy ref until the repo migrates), never the working
// tree's possibly-edited copy.
func show(opts Options) error {
	branch := trunk(opts.RepoPath)
	data, origin, err := configload.Stored(opts.RepoPath, branch)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if origin == configload.OriginNone {
		fmt.Fprintf(opts.Out, "clarity isn't set up on this repo (no %s on %s) — defaults are in effect; run `git clarity init`\n",
			config.ClarityFileName, branch)
		return nil
	}
	fmt.Fprintf(opts.Out, "%s", data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		fmt.Fprintln(opts.Out)
	}
	if origin == configload.OriginConfigRef {
		fmt.Fprintf(opts.Out, "\nnote: this config still lives in the legacy %s — run `git clarity init` to write %s, then commit and push it to %s\n",
			clarityrefs.ConfigRef, config.ClarityFileName, branch)
	}
	return nil
}

// get prints one key: the configured value plainly, or the default with a
// note — a bare default that looked configured would send someone hunting
// for where it is set.
func get(opts Options, key string) error {
	canon, err := canonicalKey(key)
	if err != nil {
		return err
	}
	data, _, err := configload.Stored(opts.RepoPath, trunk(opts.RepoPath))
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	value := ""
	if len(data) > 0 {
		decoded := gitcfg.New()
		if err := gitcfg.NewDecoder(bytes.NewReader(data)).Decode(decoded); err != nil {
			return fmt.Errorf("config: parse the stored config: %w", err)
		}
		value = decoded.Section("clarity").Option(canon)
	}
	if value == "" {
		fmt.Fprintf(opts.Out, "%s (default)\n", defaultFor(canon))
		return nil
	}
	fmt.Fprintln(opts.Out, value)
	return nil
}

// set validates the value and rewrites the working-tree .clarity with the
// one key changed, for the user to commit. The seed is, in order: the
// working-tree file (an uncommitted edit is a change mid-flight, not
// something to discard), the trunk's file, the legacy ref (so a set on an
// unmigrated repo doubles as the migration), or the defaults.
func set(opts Options, key, value string) error {
	canon, err := canonicalKey(key)
	if err != nil {
		return err
	}
	if err := validate(canon, value); err != nil {
		return err
	}

	branch := trunk(opts.RepoPath)
	path := filepath.Join(opts.RepoPath, config.ClarityFileName)

	seed, origin, fresh, err := seedFor(opts.RepoPath, path, branch)
	if err != nil {
		return err
	}

	decoded := gitcfg.New()
	if err := gitcfg.NewDecoder(bytes.NewReader(seed)).Decode(decoded); err != nil {
		return fmt.Errorf("config: the stored config does not parse, fix it before setting keys: %w", err)
	}
	decoded.Section("clarity").SetOption(canon, value)
	var buf bytes.Buffer
	if err := gitcfg.NewEncoder(&buf).Encode(decoded); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	// The whole resulting file must parse as this build understands it
	// before anything is written — a set can never leave a config the
	// dashboard will refuse.
	if _, err := config.ParseClarityConfig(buf.Bytes()); err != nil {
		return fmt.Errorf("config: refusing to write: %w", err)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("config: write %s: %w", config.ClarityFileName, err)
	}
	fmt.Fprintf(opts.Out, "clarity.%s = %s — written to %s; commit and push it to %s to take effect\n",
		canon, value, config.ClarityFileName, branch)
	if origin == configload.OriginConfigRef {
		fmt.Fprintf(opts.Out, "the rest of the file was migrated from the legacy %s, which is ignored once the file lands\n", clarityrefs.ConfigRef)
	}
	if fresh {
		fmt.Fprintf(opts.Out, "this repo had no clarity config; committing %s also sets it up\n", config.ClarityFileName)
	}
	return nil
}

// seedFor picks the bytes a set starts from. fresh reports that the repo
// had no config anywhere, so the write is also setup.
func seedFor(repoPath, path, branch string) (seed []byte, origin configload.Origin, fresh bool, err error) {
	if data, readErr := os.ReadFile(path); readErr == nil {
		return data, configload.OriginNone, false, nil
	} else if !errors.Is(readErr, fs.ErrNotExist) {
		return nil, configload.OriginNone, false, fmt.Errorf("config: read %s: %w", config.ClarityFileName, readErr)
	}
	data, origin, err := configload.Stored(repoPath, branch)
	if err != nil {
		return nil, origin, false, fmt.Errorf("config: %w", err)
	}
	if origin == configload.OriginNone || len(bytes.TrimSpace(data)) == 0 {
		return config.DefaultClarityConfigFile(), origin, origin == configload.OriginNone, nil
	}
	return data, origin, false, nil
}

func canonicalKey(key string) (string, error) {
	canon, ok := knownKeys[strings.ToLower(strings.TrimSpace(key))]
	if !ok {
		return "", fmt.Errorf("unknown config key %q — this build knows: leadTime, source", key)
	}
	return canon, nil
}

func validate(canon, value string) error {
	switch canon {
	case "leadTime":
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("clarity.leadTime needs a value — one of %q, %q or %q", core.LeadAll, core.LeadReported, core.LeadPipeline)
		}
		if _, err := core.ParseLeadTimeMode(value); err != nil {
			return fmt.Errorf("clarity.leadTime: %w", err)
		}
	case "source":
		if value != config.SourceEventsRef && value != config.SourceExecName {
			return fmt.Errorf("clarity.source %q: this build only reads %q or %q", value, config.SourceEventsRef, config.SourceExecName)
		}
	}
	return nil
}

func defaultFor(canon string) string {
	switch canon {
	case "leadTime":
		return string(core.DefaultLeadTimeMode)
	case "source":
		return config.SourceEventsRef
	}
	return ""
}
