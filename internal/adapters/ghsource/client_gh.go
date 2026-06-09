package ghsource

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ezcdlabs/clarity/internal/gitenv"
)

// CLIClient is the production GHClient: it shells out to the `gh` CLI,
// which handles GitHub auth via the user's existing login. Tests
// substitute fakeGHClient instead so they don't need gh on PATH.
//
// Workflow ID resolution is cached after first lookup — the
// workflows endpoint doesn't change often within a single process.
type CLIClient struct {
	repoPath string
	slug     string           // cached "owner/name"
	wfIDs    map[string]int64 // workflow name → ID, lazily populated
}

// NewCLIClient returns a CLIClient bound to a local repository (used to
// resolve the GitHub owner/name slug via `gh repo view`).
func NewCLIClient(repoPath string) *CLIClient {
	return &CLIClient{repoPath: repoPath, wfIDs: map[string]int64{}}
}

// ListRuns hits GitHub for the most recent page of runs of one workflow
// on one branch, then fans out a /jobs call per run to inline the per-
// job timings the event derivation needs.
//
// The since parameter is currently ignored: GH's runs endpoint only
// offers a `created` filter (not `updated_at`), and even that uses an
// awkward `>=` syntax that the gh CLI's URL handling mangles. Instead
// we fetch the most recent 100 runs every poll and let the cache merge
// by run ID — newer runs supersede older copies, and historical runs
// outside the 100-run window stay sticky in cache. Same shape the old
// generate-backfill.sh script used and that's been proven in practice.
//
// Per-run /jobs failures are logged and skipped — a transient blip on
// one run shouldn't discard the other 99 successful fetches.
func (c *CLIClient) ListRuns(workflowName, branch string, since time.Time) ([]Run, error) {
	_ = since // intentionally unused; see doc comment above.

	if err := c.ensureSlug(); err != nil {
		return nil, err
	}
	wfID, err := c.workflowID(workflowName)
	if err != nil {
		return nil, err
	}

	// url.Values gives us correct percent-encoding of every parameter
	// value. Hand-rolling the query string here is what previously
	// produced a literal "created=>=..." that confused the API.
	params := url.Values{}
	params.Set("branch", branch)
	params.Set("per_page", "100")
	endpoint := fmt.Sprintf("/repos/%s/actions/workflows/%d/runs?%s", c.slug, wfID, params.Encode())

	var resp struct {
		Runs []struct {
			ID        int64     `json:"id"`
			HeadSHA   string    `json:"head_sha"`
			UpdatedAt time.Time `json:"updated_at"`
		} `json:"workflow_runs"`
	}
	if err := c.ghJSON(&resp, "api", endpoint); err != nil {
		return nil, fmt.Errorf("list runs for workflow %q on %s: %w", workflowName, c.slug, err)
	}

	out := make([]Run, 0, len(resp.Runs))
	for _, r := range resp.Runs {
		jobs, err := c.fetchJobs(r.ID)
		if err != nil {
			// One run's /jobs failure shouldn't take out the whole
			// poll. Surface via stderr so users debugging "where are
			// my events?" see something, but keep going.
			fmt.Fprintf(os.Stderr, "ghsource: skip run %d: %v\n", r.ID, err)
			continue
		}
		out = append(out, Run{
			ID:        r.ID,
			Workflow:  workflowName,
			HeadSHA:   r.HeadSHA,
			UpdatedAt: r.UpdatedAt,
			Jobs:      jobs,
		})
	}
	return out, nil
}

// fetchJobs resolves the per-job timings + conclusions for one run.
func (c *CLIClient) fetchJobs(runID int64) ([]Job, error) {
	endpoint := fmt.Sprintf("/repos/%s/actions/runs/%d/jobs", c.slug, runID)
	var resp struct {
		Jobs []struct {
			Name        string    `json:"name"`
			Status      string    `json:"status"`
			Conclusion  string    `json:"conclusion"`
			StartedAt   time.Time `json:"started_at"`
			CompletedAt time.Time `json:"completed_at"`
		} `json:"jobs"`
	}
	if err := c.ghJSON(&resp, "api", "--paginate", endpoint); err != nil {
		return nil, err
	}
	out := make([]Job, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		out = append(out, Job(j))
	}
	return out, nil
}

// ListWorkflows returns every workflow defined on the repo. Used by
// `git clarity init --github` to populate the workflow picker and by
// Source.Validate to check connectivity + name resolution.
func (c *CLIClient) ListWorkflows() ([]WorkflowSummary, error) {
	if err := c.ensureSlug(); err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("/repos/%s/actions/workflows", c.slug)
	var resp struct {
		Workflows []WorkflowSummary `json:"workflows"`
	}
	if err := c.ghJSON(&resp, "api", "--paginate", endpoint); err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}
	return resp.Workflows, nil
}

// ListJobsInWorkflow returns the distinct job names from the most
// recent run of the named workflow on the given branch. Used by the
// init flow's per-stage job picker — "no runs yet" is reported as an
// empty slice, not an error, so the user can still write a config
// pointing at jobs they know will appear once a run lands.
func (c *CLIClient) ListJobsInWorkflow(workflowID int64, branch string) ([]string, error) {
	if err := c.ensureSlug(); err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("/repos/%s/actions/workflows/%d/runs?branch=%s&per_page=1",
		c.slug, workflowID, url.QueryEscape(branch))
	var runsResp struct {
		Runs []struct {
			ID int64 `json:"id"`
		} `json:"workflow_runs"`
	}
	if err := c.ghJSON(&runsResp, "api", endpoint); err != nil {
		return nil, fmt.Errorf("sample run for workflow %d: %w", workflowID, err)
	}
	if len(runsResp.Runs) == 0 {
		return nil, nil
	}
	jobs, err := c.fetchJobs(runsResp.Runs[0].ID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	names := make([]string, 0, len(jobs))
	for _, j := range jobs {
		if seen[j.Name] {
			continue
		}
		seen[j.Name] = true
		names = append(names, j.Name)
	}
	return names, nil
}

// workflowID resolves a workflow name to its numeric ID, caching the
// result across subsequent calls within the same process.
func (c *CLIClient) workflowID(name string) (int64, error) {
	if id, ok := c.wfIDs[name]; ok {
		return id, nil
	}
	workflows, err := c.ListWorkflows()
	if err != nil {
		return 0, err
	}
	for _, w := range workflows {
		// Match by display name first (what users put in .ezcd.json),
		// falling back to filename so `clarity.github.ci.workflow:
		// "ci.yml"` works too.
		if w.Name == name || strings.HasSuffix(w.Path, "/"+name) {
			c.wfIDs[name] = w.ID
			return w.ID, nil
		}
	}
	return 0, fmt.Errorf("workflow %q not found on %s", name, c.slug)
}

// ensureSlug resolves "owner/name" once via `gh repo view`, caching
// for the rest of the process.
func (c *CLIClient) ensureSlug() error {
	if c.slug != "" {
		return nil
	}
	cmd := exec.Command("gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
	cmd.Dir = c.repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("resolve repo slug: %w", err)
	}
	c.slug = strings.TrimSpace(string(out))
	if c.slug == "" {
		return fmt.Errorf("resolve repo slug: gh returned empty")
	}
	return nil
}

// ghJSON runs `gh <args...>` and decodes its stdout as JSON into v.
func (c *CLIClient) ghJSON(v any, args ...string) error {
	cmd := exec.Command("gh", args...)
	cmd.Dir = c.repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return json.Unmarshal(out, v)
}
