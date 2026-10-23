package core_test

import (
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
)

// CurrentStage is CurrentStageStatus that also says *which* commit the answer
// came from — the identity `git clarity check` needs to name the commit that
// broke the trunk. The resolution rule is identical and deliberately so: one
// definition of "is main green?" per binary, because a second one that could
// disagree with the header badge would only be discovered during an incident.
//
// So this table re-states the badge rule from the identity side, including the
// two cases it exists to get right: only passed/failed resolve a stage, and
// recency is measured in commits rather than timestamps.
func TestCurrentStage_TableDriven(t *testing.T) {
	cases := []struct {
		name       string
		commits    []core.CommitView
		stage      string
		wantFound  bool
		wantSHA    string
		wantStatus string
		wantTime   int64
	}{
		{
			name:      "no commits",
			commits:   nil,
			stage:     "ci",
			wantFound: false,
		},
		{
			name:      "commits but no events",
			commits:   []core.CommitView{{SHA: "aaa"}, {SHA: "bbb"}},
			stage:     "ci",
			wantFound: false,
		},
		{
			name: "only started — started does not resolve a stage",
			commits: []core.CommitView{
				{SHA: "aaa", Events: []clarityrefs.Event{ev("ci", "started", 100)}},
			},
			stage:     "ci",
			wantFound: false,
		},
		{
			name: "only skipped — skipped does not resolve a stage",
			commits: []core.CommitView{
				{SHA: "aaa", Events: []clarityrefs.Event{ev("ci", "skipped", 100)}},
			},
			stage:     "ci",
			wantFound: false,
		},
		{
			name: "newest commit failed",
			commits: []core.CommitView{
				{SHA: "aaa", Events: []clarityrefs.Event{ev("ci", "failed", 200)}},
				{SHA: "bbb", Events: []clarityrefs.Event{ev("ci", "passed", 100)}},
			},
			stage:      "ci",
			wantFound:  true,
			wantSHA:    "aaa",
			wantStatus: "failed",
			wantTime:   200,
		},
		{
			name: "newest commit is still running — defers to the commit below",
			commits: []core.CommitView{
				{SHA: "aaa", Events: []clarityrefs.Event{ev("ci", "started", 300)}},
				{SHA: "bbb", Events: []clarityrefs.Event{ev("ci", "failed", 200)}},
			},
			stage:      "ci",
			wantFound:  true,
			wantSHA:    "bbb",
			wantStatus: "failed",
			wantTime:   200,
		},
		{
			name: "retry on one commit — latest resolved event within it wins",
			commits: []core.CommitView{
				{SHA: "aaa", Events: []clarityrefs.Event{
					ev("ci", "failed", 100),
					ev("ci", "started", 150),
					ev("ci", "passed", 200),
				}},
			},
			stage:      "ci",
			wantFound:  true,
			wantSHA:    "aaa",
			wantStatus: "passed",
			wantTime:   200,
		},
		{
			name: "green that went red on re-run — the later event wins",
			commits: []core.CommitView{
				{SHA: "aaa", Events: []clarityrefs.Event{
					ev("ci", "passed", 100),
					ev("ci", "failed", 200),
				}},
			},
			stage:      "ci",
			wantFound:  true,
			wantSHA:    "aaa",
			wantStatus: "failed",
			wantTime:   200,
		},
		{
			name: "the revert case — newer commit's green wins over older red " +
				"that reported later",
			commits: []core.CommitView{
				{SHA: "revert", Events: []clarityrefs.Event{ev("ci", "passed", 100)}},
				{SHA: "broken", Events: []clarityrefs.Event{ev("ci", "failed", 999)}},
			},
			stage:      "ci",
			wantFound:  true,
			wantSHA:    "revert",
			wantStatus: "passed",
			wantTime:   100,
		},
		{
			name: "other stages are ignored",
			commits: []core.CommitView{
				{SHA: "aaa", Events: []clarityrefs.Event{ev("deploy", "failed", 300)}},
				{SHA: "bbb", Events: []clarityrefs.Event{ev("ci", "passed", 100)}},
			},
			stage:      "ci",
			wantFound:  true,
			wantSHA:    "bbb",
			wantStatus: "passed",
			wantTime:   100,
		},
		{
			name: "deploy resolves independently of ci",
			commits: []core.CommitView{
				{SHA: "aaa", Events: []clarityrefs.Event{
					ev("ci", "passed", 100),
					ev("deploy", "failed", 200),
				}},
			},
			stage:      "deploy",
			wantFound:  true,
			wantSHA:    "aaa",
			wantStatus: "failed",
			wantTime:   200,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, found := core.CurrentStage(tc.commits, tc.stage)
			if found != tc.wantFound {
				t.Fatalf("found = %v, want %v", found, tc.wantFound)
			}
			if !tc.wantFound {
				return
			}
			if got.Commit.SHA != tc.wantSHA {
				t.Errorf("commit SHA = %q, want %q", got.Commit.SHA, tc.wantSHA)
			}
			if got.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tc.wantStatus)
			}
			if want := time.Unix(tc.wantTime, 0); !got.Time.Equal(want) {
				t.Errorf("time = %v, want %v", got.Time, want)
			}
		})
	}
}

// The two functions must never be able to disagree: CurrentStageStatus is the
// header badge and CurrentStage is what check reports, and a repo where one
// says green and the other names a red commit is the failure mode this test
// exists to make impossible.
func TestCurrentStage_AgreesWithCurrentStageStatus(t *testing.T) {
	commits := []core.CommitView{
		{SHA: "aaa", Events: []clarityrefs.Event{ev("ci", "started", 400)}},
		{SHA: "bbb", Events: []clarityrefs.Event{ev("ci", "failed", 300)}},
		{SHA: "ccc", Events: []clarityrefs.Event{ev("ci", "passed", 200)}},
	}

	res, found := core.CurrentStage(commits, "ci")
	status := core.CurrentStageStatus(commits, "ci")

	if !found {
		t.Fatalf("CurrentStage found nothing, but CurrentStageStatus said %q", status)
	}
	if res.Status != status {
		t.Errorf("CurrentStage status = %q, CurrentStageStatus = %q", res.Status, status)
	}
}

func TestCurrentStage_NotFoundMeansEmptyStatus(t *testing.T) {
	commits := []core.CommitView{
		{SHA: "aaa", Events: []clarityrefs.Event{ev("ci", "started", 100)}},
	}

	if _, found := core.CurrentStage(commits, "ci"); found {
		t.Errorf("CurrentStage found a resolution for a started-only stage")
	}
	if status := core.CurrentStageStatus(commits, "ci"); status != "" {
		t.Errorf("CurrentStageStatus = %q, want empty", status)
	}
}
