package config_test

import (
	"reflect"
	"testing"

	"github.com/ezcdlabs/clarity/internal/config"
)

// TestLoad_NoClaritySection: forward-compat hold-over from step 6 — now
// the loader DOES know about clarity.github but a missing section must
// still mean "use the ref source", not "fail". A nil Clarity pointer is
// the signal callers will dispatch on.
func TestLoad_NoClaritySection(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".ezcd.json", `{"branch": "main"}`)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Clarity != nil {
		t.Errorf("expected Clarity=nil when no section present, got %+v", cfg.Clarity)
	}
}

// TestLoad_ClarityGitHub_SharedJobsArray is the 95% case the doc calls
// out: one array of jobs applies to both the "started" and "completed"
// readings. Parse it once, expose .Started() and .Completed() returning
// the same slice.
func TestLoad_ClarityGitHub_SharedJobsArray(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".ezcd.json", `{
		"clarity": {
			"github": {
				"ci":     { "workflow": "CI",     "jobs": ["Test", "Integration"] },
				"deploy": { "workflow": "Deploy", "jobs": ["deploy"] }
			}
		}
	}`)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Clarity == nil || cfg.Clarity.GitHub == nil {
		t.Fatalf("expected Clarity.GitHub populated, got %+v", cfg.Clarity)
	}
	if cfg.Clarity.GitHub.CI == nil || cfg.Clarity.GitHub.Deploy == nil {
		t.Fatalf("expected both CI and Deploy populated, got %+v", cfg.Clarity.GitHub)
	}
	ci := cfg.Clarity.GitHub.CI
	if ci.Workflow != "CI" {
		t.Errorf("CI.Workflow: expected %q, got %q", "CI", ci.Workflow)
	}
	want := []string{"Test", "Integration"}
	if got := ci.Jobs.Started(); !reflect.DeepEqual(got, want) {
		t.Errorf("CI.Jobs.Started(): expected %v, got %v", want, got)
	}
	if got := ci.Jobs.Completed(); !reflect.DeepEqual(got, want) {
		t.Errorf("CI.Jobs.Completed(): expected %v, got %v", want, got)
	}
	if got := cfg.Clarity.GitHub.Deploy.Jobs.Started(); !reflect.DeepEqual(got, []string{"deploy"}) {
		t.Errorf("Deploy.Jobs.Started(): expected [deploy], got %v", got)
	}
}

// TestLoad_ClarityGitHub_DistinctJobSets exercises the struct-shape
// escape hatch for repos whose start signal is on a different job from
// the completion signal (e.g. CI's "queue" job starts but "test" is what
// finishes).
func TestLoad_ClarityGitHub_DistinctJobSets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".ezcd.json", `{
		"clarity": {
			"github": {
				"ci": {
					"workflow": "CI",
					"jobs": {
						"started":   ["queue"],
						"completed": ["test", "lint"]
					}
				}
			}
		}
	}`)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ci := cfg.Clarity.GitHub.CI
	if ci == nil {
		t.Fatalf("expected CI populated, got nil")
	}
	if got := ci.Jobs.Started(); !reflect.DeepEqual(got, []string{"queue"}) {
		t.Errorf("Started(): expected [queue], got %v", got)
	}
	wantCompleted := []string{"test", "lint"}
	if got := ci.Jobs.Completed(); !reflect.DeepEqual(got, wantCompleted) {
		t.Errorf("Completed(): expected %v, got %v", wantCompleted, got)
	}
}

// TestLoad_ClarityGitHub_MissingWorkflow is the simplest config error
// surface: the user wrote a section with a workflow name typo'd as
// nothing. Should error out with something pointing at the section,
// not silently produce empty queries.
func TestLoad_ClarityGitHub_MissingWorkflow(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".ezcd.json", `{
		"clarity": {
			"github": {
				"ci": { "jobs": ["Test"] }
			}
		}
	}`)

	_, err := config.Load(dir)
	if err == nil {
		t.Fatal("expected Load to error on missing workflow, got nil")
	}
}
