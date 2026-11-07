// Package configcmd implements `git clarity config`: the get/set surface
// over refs/clarity/config, so changing the lead time mode is one command
// instead of raw ref plumbing.
//
//	git clarity config                      # print the stored file
//	git clarity config leadTime             # read one key
//	git clarity config leadTime pipeline    # set it, push it
//
// Writes are stricter than reads on purpose. The read path ignores keys it
// doesn't know (forward compat: an old binary must survive a newer config),
// but this command refuses to *write* a key this build cannot validate —
// writing forward-compatibly is how typos become permanent shared state.
// For the same reason a set re-parses the whole resulting file before
// anything is pushed.
package configcmd

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/core"
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
	Remote   string    // defaults to "origin"
	Out      io.Writer // required
	Args     []string  // [], [key] or [key value]
}

// Run dispatches on argument count: show, get, or set.
func Run(opts Options) error {
	if opts.Remote == "" {
		opts.Remote = "origin"
	}
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

// show prints the stored file — the same bytes `git cat-file -p
// refs/clarity/config:config` would show. The fetch is best-effort, so
// offline prints the last fetched config rather than nothing.
func show(opts Options) error {
	_ = clarityrefs.FetchConfigRef(opts.RepoPath, opts.Remote)
	data, ok, err := clarityrefs.ReadConfigFile(opts.RepoPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if !ok {
		fmt.Fprintf(opts.Out, "clarity isn't set up on this repo (no %s) — defaults are in effect; run `git clarity init`\n", clarityrefs.ConfigRef)
		return nil
	}
	fmt.Fprintf(opts.Out, "%s", data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		fmt.Fprintln(opts.Out)
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
	_ = clarityrefs.FetchConfigRef(opts.RepoPath, opts.Remote)
	data, _, err := clarityrefs.ReadConfigFile(opts.RepoPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	value := ""
	if len(data) > 0 {
		decoded := gitcfg.New()
		if err := gitcfg.NewDecoder(bytes.NewReader(data)).Decode(decoded); err != nil {
			return fmt.Errorf("config: parse %s: %w", clarityrefs.ConfigRef, err)
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

// set validates the value, rewrites the file with the one key changed, and
// pushes. On a repo with no config ref the write creates it, which is
// setup — the output says so rather than leaving it to be discovered.
func set(opts Options, key, value string) error {
	canon, err := canonicalKey(key)
	if err != nil {
		return err
	}
	if err := validate(canon, value); err != nil {
		return err
	}

	// The remote's truth, surfaced: writing over a config this clone
	// cannot currently see would silently discard someone else's change.
	if err := clarityrefs.FetchConfigRef(opts.RepoPath, opts.Remote); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	data, existed, err := clarityrefs.ReadConfigFile(opts.RepoPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if !existed {
		data = config.DefaultClarityConfigFile()
	}

	decoded := gitcfg.New()
	if err := gitcfg.NewDecoder(bytes.NewReader(data)).Decode(decoded); err != nil {
		return fmt.Errorf("config: the stored config does not parse, fix it before setting keys: %w", err)
	}
	decoded.Section("clarity").SetOption(canon, value)
	var buf bytes.Buffer
	if err := gitcfg.NewEncoder(&buf).Encode(decoded); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	// The whole resulting file must parse as this build understands it
	// before anything is pushed — a set can never leave shared config in a
	// state the dashboard will refuse.
	if _, err := config.ParseClarityConfig(buf.Bytes()); err != nil {
		return fmt.Errorf("config: refusing to write: %w", err)
	}

	message := fmt.Sprintf("clarity: %s = %s", canon, value)
	if err := clarityrefs.WriteConfigFile(opts.RepoPath, opts.Remote, buf.Bytes(), message); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	fmt.Fprintf(opts.Out, "clarity.%s = %s — written to %s and pushed to %s\n", canon, value, clarityrefs.ConfigRef, opts.Remote)
	if !existed {
		fmt.Fprintf(opts.Out, "this repo had no clarity config; it is now set up\n")
	}
	return nil
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
		if value != config.SourceEventsRef {
			return fmt.Errorf("clarity.source %q: this build only reads %q", value, config.SourceEventsRef)
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
