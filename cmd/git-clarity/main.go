package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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

func dispatch(args []string) error {
	if len(args) == 0 {
		return runTUI()
	}
	switch args[0] {
	case "report":
		return runReport(args[1:])
	default:
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func runTUI() error {
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
	})
	return tui.Run(filepath.Base(repoPath), snapshots)
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
