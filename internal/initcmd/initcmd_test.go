package initcmd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/internal/adapters/ghsource"
	"github.com/ezcdlabs/clarity/internal/initcmd"
)

// fakeDiscovery returns canned workflow + jobs lists. Mirrors the
// methods on ghsource.CLIClient that the init flow consumes, without
// shelling out to gh.
type fakeDiscovery struct {
	workflows []ghsource.WorkflowSummary
	jobs      map[int64][]string
	wfErr     error
	jobsErr   error
}

func (f *fakeDiscovery) ListWorkflows() ([]ghsource.WorkflowSummary, error) {
	return f.workflows, f.wfErr
}

func (f *fakeDiscovery) ListJobsInWorkflow(workflowID int64, branch string) ([]string, error) {
	if f.jobsErr != nil {
		return nil, f.jobsErr
	}
	return f.jobs[workflowID], nil
}

// TestRun_WritesGitHubMapping is the load-bearing happy-path: discover
// two workflows, pick one for CI + jobs, pick one for deploy + jobs,
// confirm .ezcd.json now contains the right clarity.github mapping.
// The shape mirrors the README example.
func TestRun_WritesGitHubMapping(t *testing.T) {
	dir := t.TempDir()
	fake := &fakeDiscovery{
		workflows: []ghsource.WorkflowSummary{
			{ID: 1, Name: "CI", Path: ".github/workflows/ci.yml"},
			{ID: 2, Name: "Deploy", Path: ".github/workflows/deploy.yml"},
		},
		jobs: map[int64][]string{
			1: {"Test", "Lint"},
			2: {"deploy"},
		},
	}
	// Scripted answers: workflow 1 for CI, jobs 1,2; workflow 2 for deploy, jobs 1.
	in := strings.NewReader("1\n1,2\n2\n1\n")

	var out bytes.Buffer
	err := initcmd.Run(initcmd.Options{
		Client:    fake,
		In:        in,
		Out:       &out,
		ConfigDir: dir,
		Branch:    "main",
	})
	if err != nil {
		t.Fatalf("Run: %v\nstdout:\n%s", err, out.String())
	}

	// Read the file back and decode the clarity.github section.
	data, err := os.ReadFile(filepath.Join(dir, ".ezcd.json"))
	if err != nil {
		t.Fatalf("read .ezcd.json: %v", err)
	}
	var got struct {
		Clarity struct {
			GitHub map[string]struct {
				Workflow string   `json:"workflow"`
				Jobs     []string `json:"jobs"`
			} `json:"github"`
		} `json:"clarity"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse written config: %v\n%s", err, data)
	}
	if got.Clarity.GitHub["ci"].Workflow != "CI" {
		t.Errorf("ci.workflow: expected CI, got %q", got.Clarity.GitHub["ci"].Workflow)
	}
	if !reflect.DeepEqual(got.Clarity.GitHub["ci"].Jobs, []string{"Test", "Lint"}) {
		t.Errorf("ci.jobs: expected [Test Lint], got %v", got.Clarity.GitHub["ci"].Jobs)
	}
	if got.Clarity.GitHub["deploy"].Workflow != "Deploy" {
		t.Errorf("deploy.workflow: expected Deploy, got %q", got.Clarity.GitHub["deploy"].Workflow)
	}
	if !reflect.DeepEqual(got.Clarity.GitHub["deploy"].Jobs, []string{"deploy"}) {
		t.Errorf("deploy.jobs: expected [deploy], got %v", got.Clarity.GitHub["deploy"].Jobs)
	}
}

// TestRun_PreservesExistingFields: the user already has a .ezcd.json
// with a top-level branch + pushq section. init must merge — clobbering
// those would silently lose the user's existing config.
func TestRun_PreservesExistingFields(t *testing.T) {
	dir := t.TempDir()
	existing := `{"branch": "trunk", "pushq": {"test_command": "go test ./..."}}`
	if err := os.WriteFile(filepath.Join(dir, ".ezcd.json"), []byte(existing), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fake := &fakeDiscovery{
		workflows: []ghsource.WorkflowSummary{{ID: 1, Name: "CI", Path: ".github/workflows/ci.yml"}},
		jobs:      map[int64][]string{1: {"Test"}},
	}
	// Workflow 1 for CI, jobs 1, then "skip" for deploy.
	in := strings.NewReader("1\n1\nskip\n")
	var out bytes.Buffer
	if err := initcmd.Run(initcmd.Options{
		Client: fake, In: in, Out: &out, ConfigDir: dir, Branch: "trunk",
	}); err != nil {
		t.Fatalf("Run: %v\nstdout:\n%s", err, out.String())
	}

	data, _ := os.ReadFile(filepath.Join(dir, ".ezcd.json"))
	var got struct {
		Branch string `json:"branch"`
		Pushq  struct {
			TestCommand string `json:"test_command"`
		} `json:"pushq"`
		Clarity struct {
			GitHub map[string]any `json:"github"`
		} `json:"clarity"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse: %v\n%s", err, data)
	}
	if got.Branch != "trunk" {
		t.Errorf("branch: expected trunk, got %q", got.Branch)
	}
	if got.Pushq.TestCommand != "go test ./..." {
		t.Errorf("pushq.test_command: expected preserved, got %q", got.Pushq.TestCommand)
	}
	if _, ok := got.Clarity.GitHub["ci"]; !ok {
		t.Errorf("clarity.github.ci missing — init didn't write its half:\n%s", data)
	}
}

// TestRun_SkipDeploy: the user only wants CI signal from GH Actions
// (they deploy by some other means). Init lets them say "skip" on
// deploy and writes only the CI mapping. clarity.github.deploy
// must be absent from the output — not an empty struct.
func TestRun_SkipDeploy(t *testing.T) {
	dir := t.TempDir()
	fake := &fakeDiscovery{
		workflows: []ghsource.WorkflowSummary{{ID: 1, Name: "CI", Path: ".github/workflows/ci.yml"}},
		jobs:      map[int64][]string{1: {"Test"}},
	}
	in := strings.NewReader("1\n1\nskip\n")
	var out bytes.Buffer
	if err := initcmd.Run(initcmd.Options{
		Client: fake, In: in, Out: &out, ConfigDir: dir, Branch: "main",
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, ".ezcd.json"))
	var got struct {
		Clarity struct {
			GitHub map[string]any `json:"github"`
		} `json:"clarity"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := got.Clarity.GitHub["ci"]; !ok {
		t.Errorf("ci should be set, got %+v", got.Clarity.GitHub)
	}
	if _, ok := got.Clarity.GitHub["deploy"]; ok {
		t.Errorf("deploy should be absent when skipped, got %+v", got.Clarity.GitHub)
	}
}

// TestRun_NoWorkflows_Errors: brand-new repo with no workflows defined
// can't be configured against GH Actions. Init must error out cleanly,
// not write an empty config that ghsource would then choke on.
func TestRun_NoWorkflows_Errors(t *testing.T) {
	dir := t.TempDir()
	fake := &fakeDiscovery{}
	in := strings.NewReader("")
	var out bytes.Buffer
	err := initcmd.Run(initcmd.Options{
		Client: fake, In: in, Out: &out, ConfigDir: dir, Branch: "main",
	})
	if err == nil {
		t.Fatal("expected error when no workflows found, got nil")
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".ezcd.json")); statErr == nil {
		t.Errorf("expected no .ezcd.json to be written on error")
	}
}

// TestRun_DiscoveryError_PropagatesCleanly: a gh-CLI failure during
// discovery surfaces as the wrapper error, with no file written.
func TestRun_DiscoveryError_PropagatesCleanly(t *testing.T) {
	dir := t.TempDir()
	wantErr := errors.New("gh: api blew up")
	fake := &fakeDiscovery{wfErr: wantErr}
	err := initcmd.Run(initcmd.Options{
		Client: fake, In: strings.NewReader(""), Out: &bytes.Buffer{},
		ConfigDir: dir, Branch: "main",
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("expected wrapped %v, got %v", wantErr, err)
	}
}
