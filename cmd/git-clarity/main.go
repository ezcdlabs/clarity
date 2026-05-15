package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ezcdlabs/clarity/internal/gitenv"
	"github.com/ezcdlabs/clarity/internal/refs"
	"github.com/ezcdlabs/clarity/internal/report"
	"github.com/ezcdlabs/clarity/internal/tui"
	"github.com/ezcdlabs/clarity/internal/watcher"
)

// version is set at build time via -ldflags "-X main.version=<value>".
var version = "dev"

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "--version" {
		fmt.Println(version)
		return
	}
	if err := dispatch(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// rootOptions are the flags accepted on the base `git clarity` invocation.
// Subcommands (currently just `report`) handle their own arg parsing and
// don't see these flags.
type rootOptions struct {
	plain    bool
	showSHAs bool
	limit    int // 0 == unlimited (sentinel); default is 100
}

func dispatch(args []string) error {
	// Subcommands consume their own args before any root-flag parsing.
	if len(args) > 0 && args[0] == "report" {
		return runReport(args[1:])
	}
	opts, err := parseRootArgs(args)
	if err != nil {
		return err
	}
	// Pipe into a non-TTY → plain text automatically. --plain forces text
	// even in an interactive terminal.
	if opts.plain || !isTerminal(os.Stdout) {
		return runPlain(opts)
	}
	return runTUI(opts)
}

func parseRootArgs(args []string) (rootOptions, error) {
	fs := flag.NewFlagSet("clarity", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	plain := fs.Bool("plain", false, "force plain-text output (auto-enabled when stdout is not a tty)")
	showSHAs := fs.Bool("show-shas", false, "include short commit SHA per row")
	limit := fs.Int("limit", 100, "max commits to display; 0 means unlimited")
	if err := fs.Parse(args); err != nil {
		return rootOptions{}, fmt.Errorf("usage: git clarity [--plain] [--show-shas] [--limit N]: %w", err)
	}
	if fs.NArg() != 0 {
		return rootOptions{}, fmt.Errorf("unknown argument %q", fs.Arg(0))
	}
	return rootOptions{plain: *plain, showSHAs: *showSHAs, limit: *limit}, nil
}

// isTerminal reports whether f is a real character device (a terminal). False
// when stdout is being piped to another process, redirected to a file, or
// otherwise non-interactive — that's the trigger for auto-plain mode.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func runTUI(opts rootOptions) error {
	repoPath, err := repoRoot()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}
	if err := refs.EnsureClarityFetchRefspec(repoPath, "origin"); err != nil {
		return fmt.Errorf("configure clarity fetch refspec: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const branch = "main"
	snapshots := watcher.Watch(ctx, watcher.Options{
		RepoPath: repoPath,
		Remote:   "origin",
		Branch:   branch,
		Limit:    effectiveLimit(opts.limit),
	})
	return tui.Run(filepath.Base(repoPath), snapshots)
}

// runPlain takes one snapshot from the watcher (which performs the initial
// fetch on first emit) and renders it as static text. Intended for piped
// agent / shell-script consumers.
func runPlain(opts rootOptions) error {
	repoPath, err := repoRoot()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}
	if err := refs.EnsureClarityFetchRefspec(repoPath, "origin"); err != nil {
		return fmt.Errorf("configure clarity fetch refspec: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const branch = "main"
	snapshots := watcher.Watch(ctx, watcher.Options{
		RepoPath: repoPath,
		Remote:   "origin",
		Branch:   branch,
		Limit:    effectiveLimit(opts.limit),
	})

	// The watcher emits its first snapshot immediately after the initial
	// fetch — we consume that one and exit. A timeout protects against a
	// hung fetch (network problems) so plain mode can't wedge indefinitely.
	select {
	case snap, ok := <-snapshots:
		if !ok {
			return fmt.Errorf("watcher closed before emitting a snapshot")
		}
		fmt.Print(tui.RenderPlain(filepath.Base(repoPath), snap, time.Now(), tui.PlainOptions{
			ShowSHAs: opts.showSHAs,
			// Limit is already applied by the watcher; passing 0 here means
			// "don't truncate further" inside RenderPlain.
			Limit: 0,
		}))
		return nil
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timed out waiting for first snapshot")
	}
}

// effectiveLimit maps the CLI's "0 = unlimited" convention onto the watcher's
// Limit field, which would otherwise reset 0 back to its own default of 50.
func effectiveLimit(cliLimit int) int {
	if cliLimit <= 0 {
		return math.MaxInt32
	}
	return cliLimit
}

func runReport(args []string) error {
	if isBatchInvocation(args) {
		return runReportBatch(args)
	}
	opts, err := parseReportArgs(args)
	if err != nil {
		return err
	}
	repoPath, err := repoRoot()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}
	opts.RepoPath = repoPath
	opts.Remote = "origin"
	sha, err := report.Run(opts)
	if err != nil {
		return err
	}
	short := sha
	if len(short) > 8 {
		short = short[:8]
	}
	fmt.Printf("wrote event: %s %s %s\n", short, opts.Stage, opts.Status)
	return nil
}

// runReportBatch reads JSON Lines from stdin and writes the whole batch in a
// single commit + push. Each line is one event: {"sha","at","stage","status"}.
// The amortised round-trip is the point — a 200-event backfill becomes ~one
// push instead of 200.
func runReportBatch(args []string) error {
	fs := flag.NewFlagSet("report --batch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	batch := fs.Bool("batch", false, "read events from stdin as JSON Lines")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("usage: git clarity report --batch < events.jsonl: %w", err)
	}
	if !*batch || len(fs.Args()) != 0 {
		return fmt.Errorf("usage: git clarity report --batch < events.jsonl")
	}
	repoPath, err := repoRoot()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}
	events, err := readBatchEvents(os.Stdin)
	if err != nil {
		return err
	}
	if err := report.RunBatch(report.BatchOptions{
		RepoPath: repoPath,
		Remote:   "origin",
	}, events); err != nil {
		return err
	}
	fmt.Printf("wrote %d events\n", len(events))
	return nil
}

func isBatchInvocation(args []string) bool {
	for _, a := range args {
		if a == "--batch" {
			return true
		}
	}
	return false
}

func readBatchEvents(r io.Reader) ([]report.BatchEvent, error) {
	var events []report.BatchEvent
	sc := bufio.NewScanner(r)
	// Bump the scanner buffer in case future event payloads grow past the
	// default 64KB line limit; current backfill lines are ~150 bytes.
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNum := 0
	for sc.Scan() {
		lineNum++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		ev, err := parseBatchLine(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	return events, nil
}

func parseBatchLine(line string) (report.BatchEvent, error) {
	var raw struct {
		SHA    string `json:"sha"`
		At     string `json:"at"`
		Stage  string `json:"stage"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return report.BatchEvent{}, fmt.Errorf("invalid JSON: %w", err)
	}
	t, err := time.Parse(time.RFC3339, raw.At)
	if err != nil {
		return report.BatchEvent{}, fmt.Errorf("invalid 'at' (want RFC3339): %w", err)
	}
	return report.BatchEvent{
		SHA:    raw.SHA,
		Time:   t,
		Stage:  raw.Stage,
		Status: raw.Status,
	}, nil
}

// parseReportArgs parses CLI args for `git clarity report` into report.Options.
// Flags must come before the positional <stage> <status> args. `--sha` and
// `--at` are migration overrides: --sha takes precedence over GITHUB_SHA /
// CI_COMMIT_SHA / HEAD, and --at (RFC3339) replaces the default time.Now().
func parseReportArgs(args []string) (report.Options, error) {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	sha := fs.String("sha", "", "explicit commit SHA (overrides GITHUB_SHA / CI_COMMIT_SHA / HEAD)")
	at := fs.String("at", "", "explicit event timestamp in RFC3339 (overrides time.Now())")
	if err := fs.Parse(args); err != nil {
		return report.Options{}, fmt.Errorf("usage: git clarity report [--sha <sha>] [--at <rfc3339>] <stage> <status>: %w", err)
	}
	rest := fs.Args()
	if len(rest) != 2 {
		return report.Options{}, fmt.Errorf("usage: git clarity report [--sha <sha>] [--at <rfc3339>] <stage> <status>")
	}
	opts := report.Options{
		Stage:  rest[0],
		Status: rest[1],
		SHA:    *sha,
	}
	if *at != "" {
		t, err := time.Parse(time.RFC3339, *at)
		if err != nil {
			return report.Options{}, fmt.Errorf("invalid --at timestamp (want RFC3339, e.g. 2006-01-02T15:04:05Z): %w", err)
		}
		opts.Time = t
	}
	return opts, nil
}

func repoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Env = gitenv.Clean()
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
