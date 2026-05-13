package main

import (
	"context"
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
