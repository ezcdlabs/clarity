package ghsource_test

import (
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

// TestValidate_Success is the must-pass-quickly check: a well-formed
// .ezcd.json + a working gh client + a workflow name that exists →
// no error. This is the "no surprises at startup" path.
func TestValidate_Success(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	fake := newFakeGHClient()
	fake.workflows = []ghsource.WorkflowSummary{
		{ID: 1, Name: "CI", Path: ".github/workflows/ci.yml"},
	}
	src, err := ghsource.New(ghsource.Options{
		RepoPath: clone.Path,
		Branch:   "main",
		Mapping:  &config.GitHubConfig{CI: stageMapping("CI", "Test")},
		Cache:    cache.New(filepath.Join(t.TempDir(), "x.json.gz")),
		Client:   fake,
		Clock:    clock.NewFake(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := src.Validate(context.Background()); err != nil {
		t.Errorf("Validate: expected nil, got %v", err)
	}
}

// TestValidate_WorkflowNotFound is the most common config-error
// surface: the user typed a workflow name that doesn't exist (typo,
// renamed yaml file, wrong default branch). The error MUST name the
// missing workflow and list what's available, so the fix is obvious
// without grepping logs.
func TestValidate_WorkflowNotFound(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	fake := newFakeGHClient()
	fake.workflows = []ghsource.WorkflowSummary{
		{ID: 1, Name: "Continuous Integration", Path: ".github/workflows/ci.yml"},
	}
	src, err := ghsource.New(ghsource.Options{
		RepoPath: clone.Path,
		Branch:   "main",
		Mapping:  &config.GitHubConfig{CI: stageMapping("CI", "Test")},
		Cache:    cache.New(filepath.Join(t.TempDir(), "x.json.gz")),
		Client:   fake,
		Clock:    clock.NewFake(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = src.Validate(context.Background())
	if err == nil {
		t.Fatal("expected Validate to error on unknown workflow, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, `"CI"`) {
		t.Errorf("error should name the missing workflow: %v", err)
	}
	if !strings.Contains(msg, "Continuous Integration") {
		t.Errorf("error should list the available workflow names: %v", err)
	}
}

// TestValidate_AuthFailure: gh CLI / network is broken — wrap the
// underlying client error with context so users know to check `gh
// auth status` rather than chase config.
func TestValidate_AuthFailure(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	fake := newFakeGHClient()
	fake.workflowErr = errors.New("gh: not authenticated")
	src, err := ghsource.New(ghsource.Options{
		RepoPath: clone.Path,
		Branch:   "main",
		Mapping:  &config.GitHubConfig{CI: stageMapping("CI", "Test")},
		Cache:    cache.New(filepath.Join(t.TempDir(), "x.json.gz")),
		Client:   fake,
		Clock:    clock.NewFake(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = src.Validate(context.Background())
	if err == nil {
		t.Fatal("expected Validate to error when ListWorkflows fails, got nil")
	}
	if !strings.Contains(err.Error(), "gh: not authenticated") {
		t.Errorf("expected the underlying gh error in the wrap, got %v", err)
	}
}

// TestValidate_NoWorkflowsOnRepo: brand-new repo with zero workflows
// is a config-error too — clarity.github only makes sense if the user
// has *something* to map. Distinct from a missing workflow because the
// error message wants different remediation ("define a workflow first").
func TestValidate_NoWorkflowsOnRepo(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	fake := newFakeGHClient()
	fake.workflows = nil
	src, err := ghsource.New(ghsource.Options{
		RepoPath: clone.Path,
		Branch:   "main",
		Mapping:  &config.GitHubConfig{CI: stageMapping("CI", "Test")},
		Cache:    cache.New(filepath.Join(t.TempDir(), "x.json.gz")),
		Client:   fake,
		Clock:    clock.NewFake(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = src.Validate(context.Background())
	if err == nil {
		t.Fatal("expected Validate to error on a workflow-less repo, got nil")
	}
}
