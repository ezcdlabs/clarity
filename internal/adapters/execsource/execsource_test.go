package execsource_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/adapters/execsource"
	"github.com/ezcdlabs/clarity/internal/adapters/refsource"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// seedCommits pushes n distinct commits to main and returns their SHAs,
// newest first.
func seedCommits(t *testing.T, remote *gittest.Remote, n int) []string {
	t.Helper()
	clone := remote.NewClone(t)
	for i := 0; i < n; i++ {
		clone.WriteFile("f.txt", fmt.Sprintf("rev %d", i))
		clone.CommitAll(fmt.Sprintf("feat: change %d", i))
	}
	clone.Push("main")
	commits := clone.LogBranch("main")
	shas := make([]string, 0, len(commits))
	for _, c := range commits {
		shas = append(shas, c.Hash)
	}
	return shas
}

// helperScript writes an executable shell script and returns its path.
// The script lives outside any repo so nothing can mistake it for repo
// content.
func helperScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "helper")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	return path
}

func newSource(t *testing.T, repoPath string, argv []string, timeout time.Duration) *execsource.Source {
	t.Helper()
	src, err := execsource.New(execsource.Options{
		RepoPath: repoPath,
		Branch:   "main",
		RepoName: "app",
		Limit:    50,
		Argv:     argv,
		Timeout:  timeout,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return src
}

// TestOnce_GoldenAgainstRefEvents is the issue's golden test: the exec
// source must parse helper output into the same snapshot as equivalent
// events on the ref — no code path beyond the adapter may distinguish the
// two.
func TestOnce_GoldenAgainstRefEvents(t *testing.T) {
	remote := gittest.NewRemote(t)
	shas := seedCommits(t, remote, 3)

	// One repo gets the events on the ref…
	refClone := remote.NewClone(t)
	if err := clarityrefs.WriteEvents(refClone.Path, "origin", map[string][]clarityrefs.Event{
		shas[0]: {{Stage: "ci", Status: "passed", Time: time.Unix(1744120000, 0)}},
		shas[1]: {
			{Stage: "ci", Status: "passed", Time: time.Unix(1744110000, 0)},
			{Stage: "deploy", Status: "passed", Time: time.Unix(1744115000, 0), Target: "ios"},
		},
	}); err != nil {
		t.Fatalf("seed ref events: %v", err)
	}
	if err := clarityrefs.FetchEventsRef(refClone.Path, "origin"); err != nil {
		t.Fatalf("fetch events ref: %v", err)
	}
	want, err := refsource.BuildSnapshot(refClone.Path, "main", 50)
	if err != nil {
		t.Fatalf("ref snapshot: %v", err)
	}

	// …the other gets a helper printing the same events as protocol lines.
	execClone := remote.NewClone(t)
	helper := helperScript(t, fmt.Sprintf(`cat >/dev/null
printf '%%s\n' '{"sha":"%s","stage":"ci","status":"passed","ts":1744120000}'
printf '%%s\n' '{"sha":"%s","stage":"ci","status":"passed","ts":1744110000}'
printf '%%s\n' '{"sha":"%s","stage":"deploy","status":"passed","ts":1744115000,"target":"ios"}'
`, shas[0], shas[1], shas[1]))
	src := newSource(t, execClone.Path, []string{helper}, 0)

	got, err := src.Once()
	if err != nil {
		t.Fatalf("Once: %v", err)
	}
	if !reflect.DeepEqual(got.Commits, want.Commits) {
		t.Errorf("exec snapshot differs from the ref snapshot:\ngot  %+v\nwant %+v", got.Commits, want.Commits)
	}
	if got.Degraded != "" {
		t.Errorf("Degraded = %q on a healthy read", got.Degraded)
	}
}

// TestOnce_HelperContract pins protocol v1's invocation: the configured
// argv plus `--protocol 1`, the display shas on stdin newest first, no
// shell anywhere — a metacharacter argument arrives as the bytes it is.
func TestOnce_HelperContract(t *testing.T) {
	remote := gittest.NewRemote(t)
	shas := seedCommits(t, remote, 3)
	clone := remote.NewClone(t)

	outDir := t.TempDir()
	helper := helperScript(t, fmt.Sprintf(`cat > %q
printf '%%s\n' "$@" > %q
`, filepath.Join(outDir, "stdin"), filepath.Join(outDir, "args")))

	src := newSource(t, clone.Path, []string{helper, "--store=ch"}, 0)
	if _, err := src.Once(); err != nil {
		t.Fatalf("Once: %v", err)
	}

	stdin, err := os.ReadFile(filepath.Join(outDir, "stdin"))
	if err != nil {
		t.Fatalf("helper got no stdin: %v", err)
	}
	if want := strings.Join(shas, "\n") + "\n"; string(stdin) != want {
		t.Errorf("stdin = %q, want the display shas newest first %q", stdin, want)
	}

	args, err := os.ReadFile(filepath.Join(outDir, "args"))
	if err != nil {
		t.Fatalf("helper recorded no args: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(args), "\n"), "\n")
	if len(lines) != 3 || lines[0] != "--store=ch" || lines[2] != "1" || lines[1] != "--protocol" {
		t.Errorf("helper args = %q, want [--store=ch --protocol 1] after the command", lines)
	}
}

// TestOnce_MetacharacterArgumentArrivesLiterally: there is no shell in the
// path, so an argument full of shell is just bytes — it reaches the helper
// verbatim and nothing it "says" is executed.
func TestOnce_MetacharacterArgumentArrivesLiterally(t *testing.T) {
	remote := gittest.NewRemote(t)
	seedCommits(t, remote, 1)
	clone := remote.NewClone(t)

	outDir := t.TempDir()
	pwned := filepath.Join(outDir, "pwned")
	helper := helperScript(t, fmt.Sprintf(`cat >/dev/null
printf '%%s\n' "$1" > %q
`, filepath.Join(outDir, "arg1")))
	src := newSource(t, clone.Path, []string{helper, `$(touch ` + pwned + `); rm -rf *`}, 0)
	if _, err := src.Once(); err != nil {
		t.Fatalf("Once: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(outDir, "arg1"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.TrimRight(string(got), "\n") != `$(touch `+pwned+`); rm -rf *` {
		t.Errorf("arg = %q, want the literal bytes", got)
	}
	if _, err := os.Stat(pwned); err == nil {
		t.Fatal("a $() argument was interpreted — something ran a shell")
	}
}

// TestOnce_FailureCarriesStderr: stderr is the helper's only diagnostic
// channel, so the error must carry it to whoever reads the failure.
func TestOnce_FailureCarriesStderr(t *testing.T) {
	remote := gittest.NewRemote(t)
	seedCommits(t, remote, 1)
	clone := remote.NewClone(t)

	helper := helperScript(t, `echo "clickhouse: connection refused" >&2
exit 3`)
	src := newSource(t, clone.Path, []string{helper}, 0)
	_, err := src.Once()
	if err == nil {
		t.Fatal("a failing helper produced a snapshot")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error should carry the helper's stderr: %v", err)
	}
}

// TestOnce_TimeoutIsFailure: a hung helper is a failed poll, not a hung
// dashboard.
func TestOnce_TimeoutIsFailure(t *testing.T) {
	remote := gittest.NewRemote(t)
	seedCommits(t, remote, 1)
	clone := remote.NewClone(t)

	helper := helperScript(t, `sleep 5`)
	src := newSource(t, clone.Path, []string{helper}, 100*time.Millisecond)
	start := time.Now()
	_, err := src.Once()
	if err == nil {
		t.Fatal("a hung helper produced a snapshot")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("timeout took %s; the deadline did not bind", elapsed)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error should say it timed out: %v", err)
	}
}

// TestOnce_MalformedOutputIsFailure: garbage is not events. Treating a
// half-broken helper's parseable prefix as the truth would render a
// confident, wrong dashboard.
func TestOnce_MalformedOutputIsFailure(t *testing.T) {
	remote := gittest.NewRemote(t)
	seedCommits(t, remote, 1)
	clone := remote.NewClone(t)

	helper := helperScript(t, `cat >/dev/null
echo '{"sha":"abc","stage":"ci","status":"passed","ts":1}'
echo 'not json at all'`)
	src := newSource(t, clone.Path, []string{helper}, 0)
	if _, err := src.Once(); err == nil {
		t.Fatal("malformed helper output produced a snapshot")
	}
}

// TestWatch_FailureEmitsLastGoodMarkedDegraded: the stale-marker
// behaviour the issue specifies — a failing poll keeps the last good
// snapshot on screen and says the source is degraded, instead of today's
// silent watcher.
func TestWatch_FailureEmitsLastGoodMarkedDegraded(t *testing.T) {
	remote := gittest.NewRemote(t)
	shas := seedCommits(t, remote, 2)
	clone := remote.NewClone(t)

	flag := filepath.Join(t.TempDir(), "fail")
	helper := helperScript(t, fmt.Sprintf(`cat >/dev/null
if [ -e %q ]; then echo "store down" >&2; exit 1; fi
printf '%%s\n' '{"sha":"%s","stage":"ci","status":"passed","ts":1744120000}'
`, flag, shas[0]))

	src, err := execsource.New(execsource.Options{
		RepoPath: clone.Path,
		Branch:   "main",
		RepoName: "app",
		Limit:    50,
		Argv:     []string{helper},
		Interval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	snaps := src.Watch(ctx)

	first := <-snaps
	if first.Degraded != "" {
		t.Fatalf("first snapshot degraded: %q", first.Degraded)
	}
	if len(first.Commits) != len(shas) || len(first.Commits[0].Events) != 1 {
		t.Fatalf("first snapshot wrong shape: %+v", first.Commits)
	}

	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	var degraded *bool
	for snap := range snaps {
		if snap.Degraded != "" {
			if len(snap.Commits) != len(shas) || len(snap.Commits[0].Events) != 1 {
				t.Fatalf("degraded snapshot lost the last good data: %+v", snap.Commits)
			}
			if !strings.Contains(snap.Degraded, "store down") {
				t.Errorf("Degraded = %q, want the helper's stderr in it", snap.Degraded)
			}
			v := true
			degraded = &v
			break
		}
	}
	if degraded == nil {
		t.Fatal("the failing helper never surfaced as a degraded snapshot")
	}

	// And recovery clears the marker.
	if err := os.Remove(flag); err != nil {
		t.Fatalf("clear flag: %v", err)
	}
	for snap := range snaps {
		if snap.Degraded == "" {
			return // recovered
		}
	}
	t.Fatal("the source never recovered after the helper did")
}

// TestValidate_FailsLoudlyBeforeAnyFirstState: "degrade quietly" only
// holds once a first state exists. A helper that cannot run at all must
// fail the pre-flight with its stderr, not hang a blank dashboard.
func TestValidate_FailsLoudlyBeforeAnyFirstState(t *testing.T) {
	remote := gittest.NewRemote(t)
	seedCommits(t, remote, 1)
	clone := remote.NewClone(t)

	helper := helperScript(t, `echo "no such store" >&2; exit 1`)
	src := newSource(t, clone.Path, []string{helper}, 0)
	err := src.Validate(context.Background())
	if err == nil {
		t.Fatal("Validate passed a helper that cannot run")
	}
	if !strings.Contains(err.Error(), "no such store") {
		t.Errorf("Validate error should carry the helper's stderr: %v", err)
	}
}
