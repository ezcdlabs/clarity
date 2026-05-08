package ci_test

import (
	"os"
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/internal/ci"
)

// clearCIEnv unsets every CI-related env var that Detect might inspect, so
// each subtest starts from a clean baseline regardless of the host environment.
func clearCIEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"CI",
		"GITHUB_ACTIONS", "GITHUB_RUN_ID", "GITHUB_SERVER_URL",
		"GITHUB_REPOSITORY", "GITHUB_ACTOR",
		"GITLAB_CI", "CI_PIPELINE_ID", "CI_JOB_ID",
		"CI_PIPELINE_URL", "CI_JOB_URL", "GITLAB_USER_LOGIN",
	}
	for _, k := range keys {
		// t.Setenv only works to set; to clear and have it restored we have
		// to read first then set to empty.
		if v, ok := os.LookupEnv(k); ok {
			t.Setenv(k, v) // register original value for restore
			os.Unsetenv(k)
		}
	}
}

func TestDetect_NoCIEnv_ReturnsEmpty(t *testing.T) {
	clearCIEnv(t)

	got := ci.Detect()
	if len(got) != 0 {
		t.Errorf("expected empty map outside any CI env, got %v", got)
	}
}

func TestDetect_GitHubActions_PopulatesAllFields(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_RUN_ID", "12345")
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	t.Setenv("GITHUB_REPOSITORY", "ezcdlabs/clarity")
	t.Setenv("GITHUB_ACTOR", "alice")

	got := ci.Detect()

	if got["system"] != "github-actions" {
		t.Errorf("system: expected %q, got %q", "github-actions", got["system"])
	}
	if got["run_id"] != "12345" {
		t.Errorf("run_id: expected %q, got %q", "12345", got["run_id"])
	}
	if got["actor"] != "alice" {
		t.Errorf("actor: expected %q, got %q", "alice", got["actor"])
	}
	wantURL := "https://github.com/ezcdlabs/clarity/actions/runs/12345"
	if got["run_url"] != wantURL {
		t.Errorf("run_url: expected %q, got %q", wantURL, got["run_url"])
	}
}

func TestDetect_GitHubActions_MissingOptionals_StillIdentifiesSystem(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	// no run id, server url, etc.

	got := ci.Detect()
	if got["system"] != "github-actions" {
		t.Errorf("expected system=github-actions even without optionals, got %q", got["system"])
	}
	// run_url cannot be constructed without server+repo+run id; must be absent or empty
	if got["run_url"] != "" {
		t.Errorf("run_url should be empty when components are missing, got %q", got["run_url"])
	}
}

func TestDetect_GitLab_PopulatesFields(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("GITLAB_CI", "true")
	t.Setenv("CI_PIPELINE_ID", "678")
	t.Setenv("CI_PIPELINE_URL", "https://gitlab.com/org/repo/-/pipelines/678")
	t.Setenv("GITLAB_USER_LOGIN", "bob")

	got := ci.Detect()

	if got["system"] != "gitlab" {
		t.Errorf("system: expected %q, got %q", "gitlab", got["system"])
	}
	if got["run_id"] != "678" {
		t.Errorf("run_id: expected %q, got %q", "678", got["run_id"])
	}
	if got["run_url"] != "https://gitlab.com/org/repo/-/pipelines/678" {
		t.Errorf("run_url: expected pipeline URL, got %q", got["run_url"])
	}
	if got["actor"] != "bob" {
		t.Errorf("actor: expected %q, got %q", "bob", got["actor"])
	}
}

func TestDetect_GenericCI_OnlySystemSet(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("CI", "true")

	got := ci.Detect()
	if got["system"] != "generic" {
		t.Errorf("expected system=generic, got %q", got["system"])
	}
}

// TestDetect_PrefersSpecificOverGeneric verifies that when both CI=true and
// GITHUB_ACTIONS=true are set (the common case in GitHub Actions), the more
// specific identifier wins.
func TestDetect_PrefersSpecificOverGeneric(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("CI", "true")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_RUN_ID", "1")

	got := ci.Detect()
	if got["system"] != "github-actions" {
		t.Errorf("expected system=github-actions when both CI and GITHUB_ACTIONS are set, got %q", got["system"])
	}
}

// TestDetect_NeverPanics_OnUnusualValues verifies the function tolerates
// odd env values without crashing.
func TestDetect_NeverPanics_OnUnusualValues(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_RUN_ID", "")
	t.Setenv("GITHUB_SERVER_URL", "not a url")
	t.Setenv("GITHUB_REPOSITORY", "")
	got := ci.Detect()
	// Just ensure we got something and it didn't crash.
	if got == nil {
		t.Fatal("expected non-nil map")
	}
	if !strings.Contains(got["system"], "github") {
		t.Errorf("expected github-actions system marker, got %q", got["system"])
	}
}
