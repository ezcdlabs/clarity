package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

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
	snapshots := watcher.Watch(ctx, watcher.Options{
		RepoPath: repoPath,
		Remote:   "origin",
		Branch:   "main",
	})
	return tui.Run(snapshots)
}

func runReport(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: git clarity report <stage> <status>")
	}
	repoPath, err := repoRoot()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}
	return report.Run(report.Options{
		RepoPath: repoPath,
		Remote:   "origin",
		Stage:    args[0],
		Status:   args[1],
	})
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
