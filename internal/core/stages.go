package core

import (
	"sort"

	"github.com/ezcdlabs/clarity/clarityrefs"
)

// StageStatus is the most recent status for one stage on one commit.
type StageStatus struct {
	Stage  string
	Status string
	Time   timeRef
}

// timeRef is a minimal time wrapper so callers don't need to import time
// just to construct test data — they can build it via CollapseStages.
type timeRef struct{ unix int64 }

// CollapseStages returns the latest status per stage from the given event
// stream, in the order each stage's latest event was observed (chronological).
// This is the "render-time status collapse" described in README.md.
func CollapseStages(events []clarityrefs.Event) []StageStatus {
	latestByStage := map[string]clarityrefs.Event{}
	firstSeen := map[string]int64{}
	for _, e := range events {
		ts := e.Time.Unix()
		if cur, ok := latestByStage[e.Stage]; !ok || e.Time.After(cur.Time) {
			latestByStage[e.Stage] = e
		}
		if _, ok := firstSeen[e.Stage]; !ok {
			firstSeen[e.Stage] = ts
		}
	}

	out := make([]StageStatus, 0, len(latestByStage))
	for stage, e := range latestByStage {
		out = append(out, StageStatus{
			Stage:  stage,
			Status: e.Status,
			Time:   timeRef{unix: e.Time.Unix()},
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return firstSeen[out[i].Stage] < firstSeen[out[j].Stage]
	})
	return out
}

// OverallStatus reduces a commit's events to one of:
//   - "none"    no events
//   - "passed"  every stage's latest is passed or skipped
//   - "failed"  any stage's latest is failed
//   - "running" any stage's latest is started (and none failed)
//
// "failed" wins over "running"; "running" wins over "passed".
func OverallStatus(events []clarityrefs.Event) string {
	if len(events) == 0 {
		return "none"
	}
	stages := CollapseStages(events)
	hasRunning := false
	for _, s := range stages {
		switch s.Status {
		case "failed":
			return "failed"
		case "started":
			hasRunning = true
		}
	}
	if hasRunning {
		return "running"
	}
	return "passed"
}

// CIStatus returns the latest CI event's status for a commit, or "" if
// there are none. Used by per-row icon rendering to pick ✓ / ✗ / spinner.
func CIStatus(events []clarityrefs.Event) string {
	var latest clarityrefs.Event
	found := false
	for _, e := range events {
		if e.Stage != "ci" {
			continue
		}
		if !found || e.Time.After(latest.Time) {
			latest = e
			found = true
		}
	}
	if !found {
		return ""
	}
	return latest.Status
}

// CurrentStageStatus returns the latest *resolved* (passed or failed) event
// status for the given stage anywhere in the snapshot, or "" if no resolved
// events exist. "started" and "skipped" events are intentionally ignored so
// header badges hold their colour through transient retries instead of
// flickering to neutral every time a build kicks off.
func CurrentStageStatus(commits []CommitView, stage string) string {
	var latest clarityrefs.Event
	found := false
	for _, c := range commits {
		for _, e := range c.Events {
			if e.Stage != stage {
				continue
			}
			if e.Status != "passed" && e.Status != "failed" {
				continue
			}
			if !found || e.Time.After(latest.Time) {
				latest = e
				found = true
			}
		}
	}
	if !found {
		return ""
	}
	return latest.Status
}
