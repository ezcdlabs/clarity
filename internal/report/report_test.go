package report_test

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/gittest"
	"github.com/ezcdlabs/clarity/internal/report"
)

// clearEnv unsets every env var that report.Run might inspect.
func clearEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"GITHUB_SHA", "CI_COMMIT_SHA",
		"CI", "GITHUB_ACTIONS", "GITHUB_RUN_ID", "GITHUB_SERVER_URL",
		"GITHUB_REPOSITORY", "GITHUB_ACTOR",
		"GITLAB_CI", "CI_PIPELINE_ID", "CI_JOB_ID",
		"CI_PIPELINE_URL", "CI_JOB_URL", "GITLAB_USER_LOGIN",
	}
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			t.Setenv(k, v)
			os.Unsetenv(k)
		}
	}
}

// headSHA resolves HEAD on the given clone using git CLI.
func headSHA(t *testing.T, repoPath string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func fetchEventsRef(t *testing.T, repoPath string) {
	t.Helper()
	cmd := exec.Command("git", "fetch", "origin", "+refs/clarity/events:refs/clarity/events")
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil &&
		!strings.Contains(string(out), "couldn't find remote ref") {
		t.Fatalf("fetch events ref: %v\n%s", err, out)
	}
}

func TestRun_WritesEventForHEAD(t *testing.T) {
	clearEnv(t)
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "x")
	clone.CommitAll("commit one")
	clone.Push("main")

	headBefore := headSHA(t, clone.Path)

	_, err := report.Run(report.Options{
		RepoPath: clone.Path,
		Remote:   "origin",
		Stage:    "ci",
		Status:   "passed",
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	fetchEventsRef(t, clone.Path)
	got, err := clarityrefs.ReadEvents(clone.Path, headBefore)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event for HEAD, got %d", len(got))
	}
	if got[0].Stage != "ci" || got[0].Status != "passed" {
		t.Errorf("unexpected event: %+v", got[0])
	}
}

func TestRun_PrefersGITHUB_SHA_OverHEAD(t *testing.T) {
	clearEnv(t)
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "x")
	clone.CommitAll("commit one")
	clone.Push("main")

	const fakeSHA = "1111111111111111111111111111111111111111"
	t.Setenv("GITHUB_SHA", fakeSHA)

	if _, err := report.Run(report.Options{
		RepoPath: clone.Path,
		Remote:   "origin",
		Stage:    "ci",
		Status:   "passed",
	}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	fetchEventsRef(t, clone.Path)
	got, err := clarityrefs.ReadEvents(clone.Path, fakeSHA)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected event under GITHUB_SHA, got %d events", len(got))
	}

	// HEAD SHA should NOT have an event (GITHUB_SHA was used instead).
	headSHA := headSHA(t, clone.Path)
	if headEvents, _ := clarityrefs.ReadEvents(clone.Path, headSHA); len(headEvents) != 0 {
		t.Errorf("HEAD SHA should have no events when GITHUB_SHA is set, got %d", len(headEvents))
	}
}

func TestRun_FallsBackToCI_COMMIT_SHA_WhenGITHUBNotSet(t *testing.T) {
	clearEnv(t)
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "x")
	clone.CommitAll("commit one")
	clone.Push("main")

	const fakeSHA = "2222222222222222222222222222222222222222"
	t.Setenv("CI_COMMIT_SHA", fakeSHA)

	if _, err := report.Run(report.Options{
		RepoPath: clone.Path,
		Remote:   "origin",
		Stage:    "ci",
		Status:   "passed",
	}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	fetchEventsRef(t, clone.Path)
	got, err := clarityrefs.ReadEvents(clone.Path, fakeSHA)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected event under CI_COMMIT_SHA, got %d", len(got))
	}
}

func TestRun_AttachesCIMetadata(t *testing.T) {
	clearEnv(t)
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "x")
	clone.CommitAll("commit one")
	clone.Push("main")

	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_RUN_ID", "999")
	t.Setenv("GITHUB_ACTOR", "alice")
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	t.Setenv("GITHUB_REPOSITORY", "ezcdlabs/clarity")

	if _, err := report.Run(report.Options{
		RepoPath: clone.Path,
		Remote:   "origin",
		Stage:    "deploy",
		Status:   "passed",
	}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	fetchEventsRef(t, clone.Path)
	got, err := clarityrefs.ReadEvents(clone.Path, headSHA(t, clone.Path))
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].CI["system"] != "github-actions" {
		t.Errorf("expected CI.system=github-actions, got %q", got[0].CI["system"])
	}
	if got[0].CI["run_id"] != "999" {
		t.Errorf("expected CI.run_id=999, got %q", got[0].CI["run_id"])
	}
}

func TestRun_RequiresStage(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	_, err := report.Run(report.Options{
		RepoPath: clone.Path,
		Stage:    "",
		Status:   "passed",
	})
	if err == nil {
		t.Fatal("expected error for empty stage")
	}
}

func TestRun_RequiresStatus(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	_, err := report.Run(report.Options{
		RepoPath: clone.Path,
		Stage:    "ci",
		Status:   "",
	})
	if err == nil {
		t.Fatal("expected error for empty status")
	}
}

func TestRun_RejectsUnknownStage(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	for _, bad := range []string{"build", "test", "lint", "release", "Deploy" /* case sensitive */} {
		_, err := report.Run(report.Options{
			RepoPath: clone.Path,
			Stage:    bad,
			Status:   "passed",
		})
		if err == nil {
			t.Errorf("expected stage %q to be rejected", bad)
		}
	}
}

func TestRun_AcceptsValidStages(t *testing.T) {
	clearEnv(t)
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "x")
	clone.CommitAll("commit one")
	clone.Push("main")
	for _, ok := range []string{"ci", "deploy"} {
		if _, err := report.Run(report.Options{
			RepoPath: clone.Path,
			Remote:   "origin",
			Stage:    ok,
			Status:   "passed",
		}); err != nil {
			t.Errorf("stage %q should be accepted, got: %v", ok, err)
		}
	}
}

func TestRun_RejectsUnknownStatus(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	for _, bad := range []string{"green", "red", "ok", "Passed" /* case sensitive */} {
		_, err := report.Run(report.Options{
			RepoPath: clone.Path,
			Stage:    "ci",
			Status:   bad,
		})
		if err == nil {
			t.Errorf("expected status %q to be rejected", bad)
		}
	}
}

func TestRun_ReturnsResolvedSHA(t *testing.T) {
	clearEnv(t)
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "x")
	clone.CommitAll("commit one")
	clone.Push("main")

	want := headSHA(t, clone.Path)
	got, err := report.Run(report.Options{
		RepoPath: clone.Path,
		Remote:   "origin",
		Stage:    "ci",
		Status:   "passed",
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if got != want {
		t.Errorf("expected returned sha=%s, got %s", want, got)
	}
}

func TestRun_ReturnsCISHA_WhenSet(t *testing.T) {
	clearEnv(t)
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "x")
	clone.CommitAll("commit one")
	clone.Push("main")

	const fakeSHA = "1111111111111111111111111111111111111111"
	t.Setenv("GITHUB_SHA", fakeSHA)

	got, err := report.Run(report.Options{
		RepoPath: clone.Path,
		Remote:   "origin",
		Stage:    "ci",
		Status:   "passed",
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if got != fakeSHA {
		t.Errorf("expected returned sha=%s, got %s", fakeSHA, got)
	}
}

func TestRun_OptionsSHA_BeatsGITHUB_SHA(t *testing.T) {
	clearEnv(t)
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "x")
	clone.CommitAll("commit one")
	clone.Push("main")

	const envSHA = "1111111111111111111111111111111111111111"
	const optSHA = "2222222222222222222222222222222222222222"
	t.Setenv("GITHUB_SHA", envSHA)

	got, err := report.Run(report.Options{
		RepoPath: clone.Path,
		Remote:   "origin",
		Stage:    "ci",
		Status:   "passed",
		SHA:      optSHA,
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if got != optSHA {
		t.Errorf("expected returned sha=%s (explicit Options.SHA), got %s", optSHA, got)
	}

	fetchEventsRef(t, clone.Path)
	if optEvents, _ := clarityrefs.ReadEvents(clone.Path, optSHA); len(optEvents) != 1 {
		t.Errorf("expected 1 event under Options.SHA, got %d", len(optEvents))
	}
	if envEvents, _ := clarityrefs.ReadEvents(clone.Path, envSHA); len(envEvents) != 0 {
		t.Errorf("expected 0 events under GITHUB_SHA when Options.SHA is set, got %d", len(envEvents))
	}
}

func TestRunBatch_WritesAllEventsInOneCommit(t *testing.T) {
	clearEnv(t)
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	events := []report.BatchEvent{
		{SHA: "1111111111111111111111111111111111111111", Time: time.Unix(1744120000, 0), Stage: "ci", Status: "started"},
		{SHA: "1111111111111111111111111111111111111111", Time: time.Unix(1744120134, 0), Stage: "ci", Status: "passed"},
		{SHA: "2222222222222222222222222222222222222222", Time: time.Unix(1744120300, 0), Stage: "deploy", Status: "passed"},
	}

	if err := report.RunBatch(report.BatchOptions{
		RepoPath: clone.Path,
		Remote:   "origin",
	}, events); err != nil {
		t.Fatalf("RunBatch failed: %v", err)
	}

	commits := remote.LogBranch("refs/clarity/events")
	if len(commits) != 1 {
		t.Fatalf("batch should produce exactly 1 commit, got %d", len(commits))
	}

	fetchEventsRef(t, clone.Path)
	for _, ev := range events {
		got, err := clarityrefs.ReadEvents(clone.Path, ev.SHA)
		if err != nil {
			t.Fatalf("ReadEvents(%s): %v", ev.SHA, err)
		}
		if len(got) == 0 {
			t.Errorf("no events written for %s", ev.SHA)
		}
	}
}

func TestRunBatch_RejectsInvalidStage(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	err := report.RunBatch(report.BatchOptions{
		RepoPath: clone.Path,
		Remote:   "origin",
	}, []report.BatchEvent{
		{SHA: "1111111111111111111111111111111111111111", Time: time.Unix(1744120000, 0), Stage: "build", Status: "passed"},
	})
	if err == nil {
		t.Fatal("expected error for invalid stage 'build'")
	}
}

func TestRunBatch_RejectsInvalidStatus(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	err := report.RunBatch(report.BatchOptions{
		RepoPath: clone.Path,
		Remote:   "origin",
	}, []report.BatchEvent{
		{SHA: "1111111111111111111111111111111111111111", Time: time.Unix(1744120000, 0), Stage: "ci", Status: "green"},
	})
	if err == nil {
		t.Fatal("expected error for invalid status 'green'")
	}
}

func TestRunBatch_RejectsMissingSHA(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	err := report.RunBatch(report.BatchOptions{
		RepoPath: clone.Path,
		Remote:   "origin",
	}, []report.BatchEvent{
		{Time: time.Unix(1744120000, 0), Stage: "ci", Status: "passed"},
	})
	if err == nil {
		t.Fatal("expected error for missing SHA")
	}
}

func TestRunBatch_EmptyBatchIsNoop(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	if err := report.RunBatch(report.BatchOptions{
		RepoPath: clone.Path,
		Remote:   "origin",
	}, nil); err != nil {
		t.Fatalf("empty RunBatch should not error: %v", err)
	}
	for _, r := range remote.ListRefs() {
		if r == "refs/clarity/events" {
			t.Errorf("empty batch should not create events ref")
		}
	}
}

func TestRun_UsesProvidedTime(t *testing.T) {
	clearEnv(t)
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "x")
	clone.CommitAll("commit one")
	clone.Push("main")

	want := time.Unix(1744120134, 0)

	if _, err := report.Run(report.Options{
		RepoPath: clone.Path,
		Remote:   "origin",
		Stage:    "ci",
		Status:   "passed",
		Time:     want,
	}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	fetchEventsRef(t, clone.Path)
	got, err := clarityrefs.ReadEvents(clone.Path, headSHA(t, clone.Path))
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if !got[0].Time.Equal(want) {
		t.Errorf("expected Time=%v, got %v", want, got[0].Time)
	}
}

// TestCommandLine pins the fully-explicit invocation clarity echoes before
// writing and quotes back on failure. Its value is that re-running it
// reproduces the same event: the on-disk payload stores the timestamp as Unix
// seconds, so an RFC3339 --at round-trips exactly, and event filenames are
// content-addressed — so a re-run from the same environment is a no-op rather
// than a duplicate.
func TestCommandLine(t *testing.T) {
	cases := []struct {
		name string
		opts report.Options
		want string
	}{
		{
			name: "deploy passed",
			opts: report.Options{
				SHA:    "9f9edc8673b331befd2adda3eadb62effde0fbe9",
				Time:   time.Date(2026, 8, 4, 12, 30, 5, 0, time.UTC),
				Stage:  "deploy",
				Status: "passed",
			},
			want: "git clarity report --sha 9f9edc8673b331befd2adda3eadb62effde0fbe9 " +
				"--at 2026-08-04T12:30:05Z deploy passed",
		},
		{
			// Rendered in UTC whatever the runner's zone: the event stores
			// Unix seconds, so the offset carries no information, and a
			// stable rendering keeps copy-pasted commands comparable.
			name: "non-UTC timestamp is normalised",
			opts: report.Options{
				SHA:    "abc1234",
				Time:   time.Date(2026, 8, 4, 13, 30, 5, 0, time.FixedZone("BST", 3600)),
				Stage:  "ci",
				Status: "started",
			},
			want: "git clarity report --sha abc1234 --at 2026-08-04T12:30:05Z ci started",
		},
		{
			// Sub-second precision is dropped, because the event does too.
			// Keeping it would print a command that writes a different file.
			name: "sub-second precision is truncated",
			opts: report.Options{
				SHA:    "abc1234",
				Time:   time.Date(2026, 8, 4, 12, 30, 5, 999_000_000, time.UTC),
				Stage:  "ci",
				Status: "failed",
			},
			want: "git clarity report --sha abc1234 --at 2026-08-04T12:30:05Z ci failed",
		},
	}

	for _, c := range cases {
		if got := report.CommandLine(c.opts); got != c.want {
			t.Errorf("%s:\n got: %s\nwant: %s", c.name, got, c.want)
		}
	}
}

// TestResolve_KeepsExplicitValues checks Resolve leaves a caller-supplied SHA
// and timestamp alone — the echoed command must describe the event actually
// being written, not a re-derived one.
func TestResolve_KeepsExplicitValues(t *testing.T) {
	at := time.Date(2026, 8, 4, 12, 30, 5, 0, time.UTC)
	got, err := report.Resolve(report.Options{
		RepoPath: t.TempDir(),
		SHA:      "abc1234",
		Time:     at,
		Stage:    "ci",
		Status:   "passed",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.SHA != "abc1234" {
		t.Errorf("SHA = %q, want abc1234", got.SHA)
	}
	if !got.Time.Equal(at) {
		t.Errorf("Time = %v, want %v", got.Time, at)
	}
}

// TestResolve_FillsDefaults checks the SHA comes from the CI environment and
// the timestamp defaults to now, so the echoed command is fully explicit even
// when the caller passed neither.
func TestResolve_FillsDefaults(t *testing.T) {
	t.Setenv("GITHUB_SHA", "fedcba9876543210fedcba9876543210fedcba98")
	before := time.Now()

	got, err := report.Resolve(report.Options{
		RepoPath: t.TempDir(),
		Stage:    "deploy",
		Status:   "passed",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.SHA != "fedcba9876543210fedcba9876543210fedcba98" {
		t.Errorf("SHA = %q, want the GITHUB_SHA value", got.SHA)
	}
	if got.Time.Before(before.Truncate(time.Second)) {
		t.Errorf("Time = %v, want roughly now (>= %v)", got.Time, before)
	}
}

// TestFailureError pins the message a failed report produces. The reported
// symptom is that a failed deploy report leaves the commit spinning in the
// TUI forever, so the message has to say both what was lost and exactly how
// to put it back.
func TestFailureError(t *testing.T) {
	opts := report.Options{
		SHA:    "9f9edc8673b331befd2adda3eadb62effde0fbe9",
		Time:   time.Date(2026, 8, 4, 12, 30, 5, 0, time.UTC),
		Stage:  "deploy",
		Status: "passed",
	}
	cause := errors.New("push events ref: exit status 1")

	err := report.FailureError(opts, cause)
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()

	for _, want := range []string{
		// what failed
		"failed to report deploy passed",
		// why
		"push events ref: exit status 1",
		// the consequence the user actually noticed
		"in-flight",
		// how to fix it, verbatim and copy-pasteable
		report.CommandLine(opts),
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}

	// The cause must stay inspectable rather than being flattened to text.
	if !errors.Is(err, cause) {
		t.Error("expected the cause to remain unwrappable")
	}
}
