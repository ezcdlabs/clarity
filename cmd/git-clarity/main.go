package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	const branch = "main"
	snapshots := watcher.Watch(ctx, watcher.Options{
		RepoPath: repoPath,
		Remote:   "origin",
		Branch:   branch,
	})
	return tui.Run(filepath.Base(repoPath), snapshots)
}

func runReport(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: git clarity report <stage> <status>")
	}
	repoPath, err := repoRoot()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}
	stage, status := args[0], args[1]
	sha, err := report.Run(report.Options{
		RepoPath: repoPath,
		Remote:   "origin",
		Stage:    stage,
		Status:   status,
	})
	if err != nil {
		return err
	}
	short := sha
	if len(short) > 8 {
		short = short[:8]
	}
	fmt.Printf("wrote event: %s %s %s\n", short, stage, status)
	return nil
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
