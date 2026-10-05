package watch_test

import (
	"testing"

	"github.com/ezcdlabs/clarity/mobile/internal/registry"
	"github.com/ezcdlabs/clarity/mobile/internal/watch"
)

func status(ci string, flows ...registry.FlowStatus) registry.Status {
	return registry.Status{CI: ci, Flows: flows}
}

func flow(name, deploy string) registry.FlowStatus {
	return registry.FlowStatus{Name: name, Deploy: deploy}
}

// TestCompare_BreakingIsWorthSaying is the whole feature: a stage that was fine
// and is now failing.
func TestCompare_BreakingIsWorthSaying(t *testing.T) {
	before := status("passed")
	after := status("failed")

	got := watch.Compare(&before, after)

	if len(got) != 1 {
		t.Fatalf("got %+v, want one change", got)
	}
	if got[0].Stage != "CI" {
		t.Errorf("stage = %q, want CI", got[0].Stage)
	}
	if !got[0].Broke {
		t.Error("CI went from passed to failed and the change does not say it broke")
	}
}

// TestCompare_RecoveringIsWorthSayingToo — the other half of the story, and the
// one that says stop worrying.
func TestCompare_RecoveringIsWorthSayingToo(t *testing.T) {
	before := status("failed")
	after := status("passed")

	got := watch.Compare(&before, after)

	if len(got) != 1 || got[0].Broke {
		t.Fatalf("got %+v, want one recovery", got)
	}
}

// TestCompare_NothingChangedIsSilence. A notification that fires every quarter
// of an hour for as long as a build stays broken is one the user turns off,
// taking the useful ones with it.
func TestCompare_NothingChangedIsSilence(t *testing.T) {
	for _, s := range []string{"passed", "failed", "started", "none"} {
		before := status(s)
		after := status(s)
		if got := watch.Compare(&before, after); len(got) != 0 {
			t.Errorf("%s -> %s reported %+v", s, s, got)
		}
	}
}

// TestCompare_TheFirstCheckIsSilent. There is nothing to transition from, and
// announcing the state of a repository that was just added as news is wrong —
// it did not break, it was always like that.
func TestCompare_TheFirstCheckIsSilent(t *testing.T) {
	if got := watch.Compare(nil, status("failed")); len(got) != 0 {
		t.Errorf("a repository with no recorded verdict reported %+v", got)
	}
}

// TestCompare_InFlightIsNotAVerdict pins the states that are not an answer. A
// build that has started has not failed, and reporting the gap between a push
// and a green tick as a recovery would notify on every single commit.
func TestCompare_InFlightIsNotAVerdict(t *testing.T) {
	cases := []struct{ from, to string }{
		{"passed", "started"},
		{"started", "passed"},
		{"passed", "none"},
		{"none", "passed"},
		{"failed", "started"},
	}
	for _, c := range cases {
		before := status(c.from)
		if got := watch.Compare(&before, status(c.to)); len(got) != 0 {
			t.Errorf("%s -> %s reported %+v, want silence", c.from, c.to, got)
		}
	}
}

// TestCompare_AFailureThatSurvivesAnInFlightBuildIsNotNewsTwice covers the
// common shape of a broken pipeline: red, someone pushes a fix, it runs, it
// fails again. The second failure is the same failure as far as a reader is
// concerned — they were already told.
func TestCompare_AFailureThatSurvivesAnInFlightBuildIsNotNewsTwice(t *testing.T) {
	broken := status("failed")
	running := watch.Compare(&broken, status("started"))
	if len(running) != 0 {
		t.Fatalf("failed -> started reported %+v", running)
	}
	// The registry records what it saw, so the next comparison starts from
	// "started" rather than from "failed".
	started := status("started")
	if got := watch.Compare(&started, status("failed")); len(got) != 0 {
		t.Errorf("started -> failed reported %+v; the reader already knows it is red", got)
	}
}

// TestCompare_EachDeployTargetIsItsOwnStory. A repo that ships web and ios from
// one trunk has two pipelines, and "something went red" that does not say which
// is a notification you have to open the app to understand.
func TestCompare_EachDeployTargetIsItsOwnStory(t *testing.T) {
	before := status("passed", flow("web", "passed"), flow("ios", "passed"))
	after := status("passed", flow("web", "passed"), flow("ios", "failed"))

	got := watch.Compare(&before, after)

	if len(got) != 1 {
		t.Fatalf("got %+v, want only the ios change", got)
	}
	if got[0].Stage != "deploy to ios" {
		t.Errorf("stage = %q, want it to name the target", got[0].Stage)
	}
	if !got[0].Broke {
		t.Error("ios deploy went red and the change does not say so")
	}
}

// TestCompare_ATargetNobodyHasSeenBeforeIsNotABreak — a flow appearing for the
// first time has no previous verdict, exactly as a new repository does not.
func TestCompare_ATargetNobodyHasSeenBeforeIsNotABreak(t *testing.T) {
	before := status("passed", flow("web", "passed"))
	after := status("passed", flow("web", "passed"), flow("ios", "failed"))

	if got := watch.Compare(&before, after); len(got) != 0 {
		t.Errorf("a newly seen target reported %+v", got)
	}
}

// TestCompare_ReportsEverythingThatChanged. Two pipelines can break in the same
// quarter of an hour, and the one that is dropped is the one nobody hears about.
func TestCompare_ReportsEverythingThatChanged(t *testing.T) {
	before := status("passed", flow("web", "passed"), flow("ios", "passed"))
	after := status("failed", flow("web", "failed"), flow("ios", "passed"))

	got := watch.Compare(&before, after)

	if len(got) != 2 {
		t.Fatalf("got %+v, want both CI and web", got)
	}
}
