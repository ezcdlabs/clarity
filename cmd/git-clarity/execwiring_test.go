package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/internal/gittest"
	"github.com/ezcdlabs/clarity/internal/trust"
)

// execSourceClone builds a clone whose trunk .clarity configures an exec
// source running the given helper, and returns the clone plus the path of
// the side-effect file the helper creates when (and only when) it runs.
func execSourceClone(t *testing.T, helperBody string) (*gittest.Clone, string, string) {
	t.Helper()
	remote := gittest.NewRemote(t)

	outDir := t.TempDir()
	ranFile := filepath.Join(outDir, "ran")
	helper := filepath.Join(outDir, "helper")
	script := fmt.Sprintf("#!/bin/sh\ntouch %q\n%s", ranFile, helperBody)
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}

	writer := remote.NewClone(t)
	writer.WriteFile(".clarity", "[clarity]\n\tsource = exec\n[source]\n\targ = "+helper+"\n")
	writer.CommitAll("clarity: exec source")
	writer.Push("main")

	return remote.NewClone(t), helper, ranFile
}

// TestBuildSource_UnapprovedCommandNeverRuns is this feature's core
// security property: configuration is shared, approval is local, and no
// approval means the command does not run — not in the TUI path, not in
// plain mode, not anywhere buildSource serves. The dashboard falls back
// to the events ref and says why.
func TestBuildSource_UnapprovedCommandNeverRuns(t *testing.T) {
	clone, _, ranFile := execSourceClone(t, "")
	cfg, err := loadConfig(clone.Path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	src, err := buildSource(ctx, cfg, rootOptions{limit: 10}, clone.Path)
	if err != nil {
		t.Fatalf("buildSource: %v", err)
	}
	snap := <-src.Watch(ctx)
	cancel()

	if _, statErr := os.Stat(ranFile); statErr == nil {
		t.Fatal("the unapproved helper ran")
	}
	if !strings.Contains(snap.Degraded, "trust") {
		t.Errorf("Degraded = %q — the fallback must say the source is unapproved and name the remedy", snap.Degraded)
	}
	if len(snap.Commits) == 0 {
		t.Error("the fallback emitted no data at all; the events ref should serve")
	}
}

// TestBuildSource_ApprovedCommandServesItsEvents: once this clone's human
// approved the exact argv, the helper is the source of record.
func TestBuildSource_ApprovedCommandServesItsEvents(t *testing.T) {
	clone, helper, ranFile := execSourceClone(t, `cat >/dev/null
read_sha=$(git -C . rev-parse origin/main)
printf '{"sha":"%s","stage":"ci","status":"passed","ts":1744120000}\n' "$read_sha"`)
	if err := trust.Record(clone.Path, []string{helper}); err != nil {
		t.Fatalf("record trust: %v", err)
	}
	cfg, err := loadConfig(clone.Path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	src, err := buildSource(ctx, cfg, rootOptions{limit: 10}, clone.Path)
	if err != nil {
		t.Fatalf("buildSource: %v", err)
	}
	snap := <-src.Watch(ctx)
	cancel()

	if _, statErr := os.Stat(ranFile); statErr != nil {
		t.Fatal("the approved helper never ran")
	}
	if snap.Degraded != "" {
		t.Fatalf("Degraded = %q on a healthy approved source", snap.Degraded)
	}
	if len(snap.Commits) == 0 || len(snap.Commits[0].Events) != 1 {
		t.Errorf("the helper's event did not reach the snapshot: %+v", snap.Commits)
	}
}

// TestCheck_UnapprovedCommandNeverRuns: the issue's acceptance criterion
// names check explicitly — the push gate must hold the same line.
func TestCheck_UnapprovedCommandNeverRuns(t *testing.T) {
	clone, _, ranFile := execSourceClone(t, "")
	cfg, err := loadConfig(clone.Path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = checkTrunk(ctx, cfg, clone.Path, 10)
	cancel()

	if _, statErr := os.Stat(ranFile); statErr == nil {
		t.Fatal("the unapproved helper ran under check")
	}
}
