package ghsource

import (
	"encoding/json"
	"fmt"
	"net/url"
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

// ListRuns hits GitHub for runs of one workflow on one branch with an
// updated_at filter, then fans out a /jobs call per run to inline the
// per-job timings the event derivation needs.
func (c *CLIClient) ListRuns(workflowName, branch string, since time.Time) ([]Run, error) {
	if err := c.ensureSlug(); err != nil {
		return nil, err
	}
	wfID, err := c.workflowID(workflowName)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("/repos/%s/actions/workflows/%d/runs?branch=%s&per_page=100",
		c.slug, wfID, url.QueryEscape(branch))
	if !since.IsZero() {
		endpoint += "&created=>=" + since.UTC().Format(time.RFC3339)
	}

	var resp struct {
		Runs []struct {
			ID        int64     `json:"id"`
			HeadSHA   string    `json:"head_sha"`
			UpdatedAt time.Time `json:"updated_at"`
		} `json:"workflow_runs"`
	}
	if err := c.ghJSON(&resp, "api", "--paginate", endpoint); err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}

	out := make([]Run, 0, len(resp.Runs))
	for _, r := range resp.Runs {
		jobs, err := c.fetchJobs(r.ID)
		if err != nil {
			return nil, fmt.Errorf("fetch jobs for run %d: %w", r.ID, err)
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

// workflowID resolves a workflow name to its numeric ID, caching the
// result across subsequent calls within the same process.
func (c *CLIClient) workflowID(name string) (int64, error) {
	if id, ok := c.wfIDs[name]; ok {
		return id, nil
	}
	endpoint := fmt.Sprintf("/repos/%s/actions/workflows", c.slug)
	var resp struct {
		Workflows []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"workflows"`
	}
	if err := c.ghJSON(&resp, "api", "--paginate", endpoint); err != nil {
		return 0, fmt.Errorf("list workflows: %w", err)
	}
	for _, w := range resp.Workflows {
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
