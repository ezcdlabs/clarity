package report_test

import (
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
		Stage:    "build",
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
	if got[0].Stage != "build" || got[0].Status != "passed" {
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
		Stage:    "build",
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
		Stage:    "build",
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
		Stage:    "build",
		Status:   "",
	})
	if err == nil {
		t.Fatal("expected error for empty status")
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
		Stage:    "build",
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
		Stage:    "build",
		Status:   "passed",
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if got != fakeSHA {
		t.Errorf("expected returned sha=%s, got %s", fakeSHA, got)
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
		Stage:    "build",
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
