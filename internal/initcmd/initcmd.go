// Package initcmd implements `git clarity init --github` — the
// interactive wizard that discovers GitHub Actions workflows + jobs and
// writes the chosen mapping into .ezcd.json. Replaces the discovery
// half of the old scripts/generate-backfill.sh.
//
// Kept out of cmd/git-clarity so the prompt + config-merge logic is
// testable end to end with bytes.Buffer-backed I/O and a fake
// DiscoveryClient — no gh CLI, no real stdin.
package initcmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ezcdlabs/clarity/internal/adapters/ghsource"
)

// DiscoveryClient is the slice of the GH API the init flow needs.
// ghsource.CLIClient satisfies it; tests substitute a fake.
type DiscoveryClient interface {
	ListWorkflows() ([]ghsource.WorkflowSummary, error)
	ListJobsInWorkflow(workflowID int64, branch string) ([]string, error)
}

// Options configures Run. ConfigDir is the directory that will contain
// the .ezcd.json file (typically the repo root); Branch is passed to
// ListJobsInWorkflow so the job-list comes from a run on the right
// branch.
type Options struct {
	Client    DiscoveryClient
	In        io.Reader
	Out       io.Writer
	ConfigDir string
	Branch    string
}

// Run drives the interactive workflow + jobs picker for CI and (optionally)
// deploy, then merges the resulting mapping into <ConfigDir>/.ezcd.json.
// Existing top-level fields (branch, pushq, any unknown future
// sections) are preserved.
func Run(opts Options) error {
	workflows, err := opts.Client.ListWorkflows()
	if err != nil {
		return fmt.Errorf("init: list workflows: %w", err)
	}
	if len(workflows) == 0 {
		return errors.New("init: no workflows found on the repo — define a .github/workflows/*.yml first")
	}

	fmt.Fprintln(opts.Out, "Discovered workflows:")
	for i, w := range workflows {
		fmt.Fprintf(opts.Out, "  %d) %s  (%s)\n", i+1, w.Name, w.Path)
	}

	scanner := bufio.NewScanner(opts.In)

	ci, err := pickStage("CI", workflows, opts, scanner)
	if err != nil {
		return err
	}
	deploy, err := pickStage("deploy", workflows, opts, scanner)
	if err != nil {
		return err
	}
	if ci == nil && deploy == nil {
		return errors.New("init: nothing to configure — at least one of CI or deploy must be mapped")
	}

	if err := writeMapping(filepath.Join(opts.ConfigDir, ".ezcd.json"), ci, deploy); err != nil {
		return fmt.Errorf("init: write config: %w", err)
	}
	fmt.Fprintln(opts.Out, "Wrote .ezcd.json")
	return nil
}

// stageMapping is the wire shape one stage takes in the merged
// .ezcd.json. Kept here (not config) because that package's parser
// already accepts this shape — we just emit it.
type stageMapping struct {
	Workflow string   `json:"workflow"`
	Jobs     []string `json:"jobs"`
}

// pickStage prompts for a workflow and (if chosen) job set for one
// clarity stage. Returns nil if the user answered "skip" — both stages
// being optional is the doc'd contract for clarity.github.
func pickStage(stageName string, workflows []ghsource.WorkflowSummary, opts Options, scanner *bufio.Scanner) (*stageMapping, error) {
	fmt.Fprintf(opts.Out, "Which workflow powers %s? [1-%d, or 'skip']: ", stageName, len(workflows))
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("init: unexpected EOF picking %s workflow", stageName)
	}
	answer := strings.TrimSpace(scanner.Text())
	if answer == "" || strings.EqualFold(answer, "skip") {
		return nil, nil
	}
	idx, err := strconv.Atoi(answer)
	if err != nil || idx < 1 || idx > len(workflows) {
		return nil, fmt.Errorf("init: %q is not a valid workflow choice (1-%d, or 'skip')", answer, len(workflows))
	}
	wf := workflows[idx-1]

	fmt.Fprintf(opts.Out, "Discovering jobs in %q...\n", wf.Name)
	jobs, err := opts.Client.ListJobsInWorkflow(wf.ID, opts.Branch)
	if err != nil {
		return nil, fmt.Errorf("init: list jobs for %s: %w", wf.Name, err)
	}
	if len(jobs) == 0 {
		return nil, fmt.Errorf("init: no jobs found in %q on branch %q — run the workflow at least once first", wf.Name, opts.Branch)
	}
	for i, j := range jobs {
		fmt.Fprintf(opts.Out, "  %d) %s\n", i+1, j)
	}
	fmt.Fprintf(opts.Out, "Which jobs power %s? (comma-separated numbers): ", stageName)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("init: unexpected EOF picking %s jobs", stageName)
	}
	picks, err := parseJobChoices(scanner.Text(), jobs)
	if err != nil {
		return nil, fmt.Errorf("init: %s jobs: %w", stageName, err)
	}
	return &stageMapping{Workflow: wf.Name, Jobs: picks}, nil
}

// parseJobChoices accepts a comma-separated list of 1-based indices and
// returns the matching job names in the order they were typed. Trims
// whitespace around each entry so "1, 3, 5" works.
func parseJobChoices(input string, jobs []string) ([]string, error) {
	parts := strings.Split(strings.TrimSpace(input), ",")
	if len(parts) == 0 {
		return nil, fmt.Errorf("expected at least one job number")
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		idx, err := strconv.Atoi(p)
		if err != nil || idx < 1 || idx > len(jobs) {
			return nil, fmt.Errorf("%q is not a valid job choice (1-%d)", p, len(jobs))
		}
		out = append(out, jobs[idx-1])
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("expected at least one job number")
	}
	return out, nil
}

// writeMapping merges the new ci/deploy mapping into the .ezcd.json at
// path. Existing top-level fields (branch, pushq, future unknown
// sections) survive the round-trip via json.RawMessage; only the
// clarity.github section is rewritten.
func writeMapping(path string, ci, deploy *stageMapping) error {
	root := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("parse existing %s: %w", filepath.Base(path), err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// Preserve any other keys under "clarity" (future sections); only
	// replace .github.
	clarity := map[string]json.RawMessage{}
	if raw, ok := root["clarity"]; ok {
		_ = json.Unmarshal(raw, &clarity)
	}

	github := map[string]*stageMapping{}
	if ci != nil {
		github["ci"] = ci
	}
	if deploy != nil {
		github["deploy"] = deploy
	}
	githubBytes, err := json.Marshal(github)
	if err != nil {
		return err
	}
	clarity["github"] = githubBytes

	clarityBytes, err := json.Marshal(clarity)
	if err != nil {
		return err
	}
	root["clarity"] = clarityBytes

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0o644)
}
