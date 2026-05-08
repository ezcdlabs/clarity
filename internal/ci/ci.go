// Package ci opportunistically detects the surrounding CI environment so
// clarity can attach optional metadata (system, run id, run url, actor) to
// reported events. The functions here never fail — when env vars are missing
// the corresponding fields are simply omitted.
package ci

import "os"

// Detect inspects well-known CI env vars and returns a metadata map suitable
// for the optional Event.CI field. An empty map (length 0) means we are not
// running under any recognised CI.
func Detect() map[string]string {
	out := map[string]string{}

	switch {
	case os.Getenv("GITHUB_ACTIONS") == "true":
		out["system"] = "github-actions"
		if v := os.Getenv("GITHUB_RUN_ID"); v != "" {
			out["run_id"] = v
		}
		if v := os.Getenv("GITHUB_ACTOR"); v != "" {
			out["actor"] = v
		}
		server := os.Getenv("GITHUB_SERVER_URL")
		repo := os.Getenv("GITHUB_REPOSITORY")
		runID := os.Getenv("GITHUB_RUN_ID")
		if server != "" && repo != "" && runID != "" {
			out["run_url"] = server + "/" + repo + "/actions/runs/" + runID
		}

	case os.Getenv("GITLAB_CI") == "true":
		out["system"] = "gitlab"
		// Pipeline ID is preferred — it's the unit users actually care about.
		// CI_JOB_ID is a fallback for job-only contexts.
		if v := os.Getenv("CI_PIPELINE_ID"); v != "" {
			out["run_id"] = v
		} else if v := os.Getenv("CI_JOB_ID"); v != "" {
			out["run_id"] = v
		}
		if v := os.Getenv("CI_PIPELINE_URL"); v != "" {
			out["run_url"] = v
		} else if v := os.Getenv("CI_JOB_URL"); v != "" {
			out["run_url"] = v
		}
		if v := os.Getenv("GITLAB_USER_LOGIN"); v != "" {
			out["actor"] = v
		}

	case os.Getenv("CI") == "true":
		out["system"] = "generic"
	}

	if len(out) == 0 {
		return out
	}
	return out
}
