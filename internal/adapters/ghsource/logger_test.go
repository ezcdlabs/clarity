package ghsource_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/internal/adapters/ghsource"
	"github.com/ezcdlabs/clarity/internal/cache"
	"github.com/ezcdlabs/clarity/internal/clock"
	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// TestSource_PollErrors_LandInLogger pins the user-reported bug: when a
// ListRuns call fails mid-session, the source must not write to stderr
// (that corrupts the TUI's alt-screen). Errors go to the configured
// Logger — the CLI plumbs that to a file under <cache-dir>.
//
// Drives the bug into the test by giving the fake client a runsErr,
// running one poll, and asserting the buffer captured the error.
func TestSource_PollErrors_LandInLogger(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	fake := newFakeGHClient()
	fake.runsErr = errors.New("api: rate-limited")

	var logbuf bytes.Buffer
	src, err := ghsource.New(ghsource.Options{
		RepoPath: clone.Path,
		Branch:   "main",
		Mapping:  &config.GitHubConfig{CI: stageMapping("CI", "Test")},
		Cache:    cache.New(filepath.Join(t.TempDir(), "runs.json.gz")),
		Client:   fake,
		Clock:    clock.NewFake(),
		Logger:   &logbuf,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// First emit happens after pollOnce — that's where the error lands.
	_ = waitForSnapshot(t, src.Watch(ctx))

	logged := logbuf.String()
	if !strings.Contains(logged, "ListRuns") {
		t.Errorf("expected ListRuns error in log, got %q", logged)
	}
	if !strings.Contains(logged, "rate-limited") {
		t.Errorf("expected the underlying error in log, got %q", logged)
	}
}

// TestSource_NoLogger_Silent: omitting Logger from Options must not
// panic / nil-pointer; we default to io.Discard. Protects against a
// caller forgetting to plumb a logger and getting a crash instead of
// quiet behavior.
func TestSource_NoLogger_Silent(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	fake := newFakeGHClient()
	fake.runsErr = errors.New("api: rate-limited")

	src, err := ghsource.New(ghsource.Options{
		RepoPath: clone.Path,
		Branch:   "main",
		Mapping:  &config.GitHubConfig{CI: stageMapping("CI", "Test")},
		Cache:    cache.New(filepath.Join(t.TempDir(), "runs.json.gz")),
		Client:   fake,
		Clock:    clock.NewFake(),
		// Logger intentionally omitted.
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// If the default Logger were nil, the fmt.Fprintf in pollOnce
	// would panic and Watch's goroutine would die — the test would
	// hang waiting for a snapshot. The snapshot arriving proves the
	// error path is safe.
	_ = waitForSnapshot(t, src.Watch(ctx))
}
