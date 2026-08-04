package core_test

import (
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
)

func TestBuildSnapshot_JoinsCommitsWithEvents(t *testing.T) {
	commits := []core.Commit{
		{SHA: "newer", Author: "alice", Subject: "ship it", Time: time.Unix(200, 0)},
		{SHA: "older", Author: "bob", Subject: "earlier", Time: time.Unix(100, 0)},
	}
	events := core.Events{
		"newer": {{Stage: "ci", Status: "passed", Time: time.Unix(250, 0)}},
		"older": {
			{Stage: "ci", Status: "passed", Time: time.Unix(110, 0)},
			{Stage: "deploy", Status: "passed", Time: time.Unix(150, 0)},
		},
	}

	snap := core.BuildSnapshot(commits, events)
	if len(snap.Commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(snap.Commits))
	}
	// Order preserved (newest-first).
	if snap.Commits[0].SHA != "newer" || snap.Commits[1].SHA != "older" {
		t.Errorf("expected order [newer, older], got [%s, %s]", snap.Commits[0].SHA, snap.Commits[1].SHA)
	}
	// Metadata propagated.
	if snap.Commits[0].Author != "alice" || snap.Commits[1].Subject != "earlier" {
		t.Errorf("metadata not propagated: %+v", snap.Commits)
	}
	// Events joined by SHA.
	if len(snap.Commits[0].Events) != 1 || snap.Commits[0].Events[0].Stage != "ci" {
		t.Errorf("newer's events not joined: %+v", snap.Commits[0].Events)
	}
	if len(snap.Commits[1].Events) != 2 {
		t.Errorf("older expected 2 events, got %d", len(snap.Commits[1].Events))
	}
}

func TestBuildSnapshot_EventsForUnknownSHAs_AreDropped(t *testing.T) {
	commits := []core.Commit{{SHA: "a", Time: time.Unix(100, 0)}}
	events := core.Events{
		"a":          {{Stage: "ci", Status: "passed", Time: time.Unix(110, 0)}},
		"not-loaded": {{Stage: "ci", Status: "failed", Time: time.Unix(120, 0)}},
	}

	snap := core.BuildSnapshot(commits, events)
	if len(snap.Commits) != 1 {
		t.Fatalf("expected 1 commit (events for missing SHAs dropped), got %d", len(snap.Commits))
	}
	if len(snap.Commits[0].Events) != 1 {
		t.Errorf("expected only 'a's event, got %+v", snap.Commits[0].Events)
	}
}

func TestBuildSnapshot_CommitWithNoEvents_HasEmptyEvents(t *testing.T) {
	commits := []core.Commit{{SHA: "lonely", Time: time.Unix(100, 0)}}
	snap := core.BuildSnapshot(commits, core.Events{})
	if len(snap.Commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(snap.Commits))
	}
	if len(snap.Commits[0].Events) != 0 {
		t.Errorf("expected no events for orphan commit, got %v", snap.Commits[0].Events)
	}
}

func TestDeriveView_PopulatesAllFields(t *testing.T) {
	// A minimal snapshot exercising both groups (Head + Deployed) and weekly
	// stats. The point isn't to cover every edge case — those live with the
	// individual function tests — just to confirm DeriveView wires the four
	// derived shapes together.
	commitTime := time.Date(2026, 5, 11, 9, 0, 0, 0, time.UTC) // ISO week 19
	deployTime := commitTime.Add(time.Hour)
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: "head", Author: "alice", Subject: "wip", Time: commitTime.Add(time.Minute)},
		{SHA: "deployed", Author: "alice", Subject: "shipped", Time: commitTime,
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: commitTime.Add(10 * time.Minute)},
				{Stage: "deploy", Status: "passed", Time: deployTime},
			}},
	}}

	view := core.DeriveView(snap, core.DefaultLeadTimeMode)
	if &view.Snapshot != &view.Snapshot { /* keeps the linter happy */
	}
	if len(view.Groups.Head) != 1 || view.Groups.Head[0].SHA != "head" {
		t.Errorf("Head: expected ['head'], got %+v", view.Groups.Head)
	}
	if len(view.Groups.Deployed) != 1 {
		t.Fatalf("Deployed: expected 1 batch, got %d", len(view.Groups.Deployed))
	}
	if len(view.Weekly) != 1 {
		t.Errorf("Weekly: expected 1 week stat, got %d", len(view.Weekly))
	}
	if view.Header.Deploy != "passed" {
		t.Errorf("Header.Deploy: expected 'passed', got %q", view.Header.Deploy)
	}
	if view.Header.CI != "passed" {
		t.Errorf("Header.CI: expected 'passed', got %q", view.Header.CI)
	}
}
