package config

import (
	"encoding/json"
	"fmt"
)

// ClarityConfig is the `clarity` section of .ezcd.json. Optional —
// callers receive a nil pointer when the file omits the section. Today
// it only holds the GitHub Actions mapping; future fields (sinks, etc.)
// would slot in alongside.
type ClarityConfig struct {
	GitHub *GitHubConfig
}

// GitHubConfig maps the two clarity lifecycle stages (CI, deploy) onto
// GitHub Actions workflows + jobs. Either stage may be absent (nil) —
// users who only track CI signals via GH Actions but deploy by some
// other means can leave Deploy unmapped. Used by both the ghsource live
// renderer and the future copy-events --from github backfill path.
type GitHubConfig struct {
	CI     *StageMapping
	Deploy *StageMapping
}

// StageMapping pins one clarity stage to one GitHub workflow + the jobs
// within it that signal start / completion. The doc allows two shapes for
// Jobs:
//
//   - ["job-a", "job-b"]  — same set powers both "started" and "completed"
//   - {"started": [...], "completed": [...]}  — distinct sets
//
// JobSet unifies both behind .Started() / .Completed() so callers don't
// have to care which shape was written.
type StageMapping struct {
	Workflow string
	Jobs     JobSet
}

// JobSet stores the parsed job-name lists for a stage's start and
// completion gates. The two slices may alias the same backing array when
// the shared-array shape was used — Started() / Completed() return them
// directly. Callers MUST treat the returned slices as read-only.
type JobSet struct {
	started   []string
	completed []string
}

// Started returns the job names whose first start timestamp signals
// stage.started. nil when the section omitted Jobs entirely.
func (j JobSet) Started() []string { return j.started }

// Completed returns the job names whose terminal conclusions aggregate
// into stage.passed / stage.failed.
func (j JobSet) Completed() []string { return j.completed }

// UnmarshalJSON accepts either of the two documented shapes. A malformed
// payload (e.g. a number where the array / struct should be) surfaces as
// the json package's own error.
func (j *JobSet) UnmarshalJSON(data []byte) error {
	// Shape 1: shared array of names. Try this first — it's the common case.
	var shared []string
	if err := json.Unmarshal(data, &shared); err == nil {
		j.started = shared
		j.completed = shared
		return nil
	}
	// Shape 2: struct with distinct sets.
	var distinct struct {
		Started   []string `json:"started"`
		Completed []string `json:"completed"`
	}
	if err := json.Unmarshal(data, &distinct); err != nil {
		return fmt.Errorf("clarity.github stage `jobs` must be an array or {started, completed} object: %w", err)
	}
	j.started = distinct.Started
	j.completed = distinct.Completed
	return nil
}

// rawClarityConfig is the wire shape used during decoding. Kept separate
// from the exported ClarityConfig so json struct tags don't leak into the
// public type signature.
type rawClarityConfig struct {
	GitHub *rawGitHubConfig `json:"github"`
}

type rawGitHubConfig struct {
	CI     *rawStageMapping `json:"ci"`
	Deploy *rawStageMapping `json:"deploy"`
}

type rawStageMapping struct {
	Workflow string `json:"workflow"`
	Jobs     JobSet `json:"jobs"`
}

// hydrate converts the wire shape into the exported types and validates
// each present stage. nil stages are silently allowed — users only
// tracking CI signals via GH Actions leave Deploy unmapped. Returning an
// error on a present-but-broken stage keeps malformed configs from
// silently producing empty workflow queries downstream.
func (r *rawClarityConfig) hydrate() (*ClarityConfig, error) {
	if r == nil {
		return nil, nil
	}
	out := &ClarityConfig{}
	if r.GitHub != nil {
		gh := &GitHubConfig{}
		ci, err := hydrateStage("ci", r.GitHub.CI)
		if err != nil {
			return nil, err
		}
		gh.CI = ci
		deploy, err := hydrateStage("deploy", r.GitHub.Deploy)
		if err != nil {
			return nil, err
		}
		gh.Deploy = deploy
		out.GitHub = gh
	}
	return out, nil
}

// hydrateStage converts one optional wire stage into the exported type
// and validates the only required field (workflow name). Empty Jobs is
// allowed (it means "any job in the workflow counts") — but no
// workflow means we have nothing to query.
func hydrateStage(name string, raw *rawStageMapping) (*StageMapping, error) {
	if raw == nil {
		return nil, nil
	}
	if raw.Workflow == "" {
		return nil, fmt.Errorf("clarity.github.%s.workflow is required", name)
	}
	s := StageMapping(*raw)
	return &s, nil
}
