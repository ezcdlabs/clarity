package acceptance_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
)

// revertPairSnapshot is the shape the noop status exists for: a commit and
// its revert, both genuinely touching a target's inputs, whose cumulative
// diff is empty. The pipeline ran its deploy stage, compared the build
// output against production, found them already equal, and reported
// `deploy noop` on the tip.
func revertPairSnapshot() core.Snapshot {
	return core.Snapshot{
		RepoName: "clarity",
		Commits: []core.CommitView{
			{
				SHA:    "ccc3333333333333333333333333333333333333",
				Author: "carol", Subject: `revert "break the checkout"`, Time: time.Unix(2000, 0),
				Events: []clarityrefs.Event{
					{Stage: "ci", Status: "passed", Time: time.Unix(2100, 0)},
					{Stage: "deploy", Status: "noop", Time: time.Unix(2200, 0)},
				},
			},
			{
				SHA:    "bbb2222222222222222222222222222222222222",
				Author: "carol", Subject: "break the checkout", Time: time.Unix(1900, 0),
				Events: []clarityrefs.Event{
					{Stage: "ci", Status: "passed", Time: time.Unix(1950, 0)},
				},
			},
			{
				SHA:    "aaa1111111111111111111111111111111111111",
				Author: "alice", Subject: "ship the checkout", Time: time.Unix(1000, 0),
				Events: []clarityrefs.Event{
					{Stage: "ci", Status: "passed", Time: time.Unix(1100, 0)},
					{Stage: "deploy", Status: "passed", Time: time.Unix(1200, 0)},
				},
			},
		},
	}
}

// The whole point of the status, end to end: the revert pair neither sits in
// CI Passed dragging the lead time up, nor claims a deploy that never
// happened. It completes — as a no-op.
func TestNoopDeploy_CompletesDeliveryWithoutCountingADeploy(t *testing.T) {
	out := renderDiscovered(t, revertPairSnapshot())

	// The pair left CI Passed: the no-op batch renders in Deployed with its
	// own subheader, above the deploy that shipped the current bytes.
	if !strings.Contains(out, "deploy no-op") {
		t.Fatalf("expected a 'deploy no-op' batch subheader, got:\n%s", out)
	}
	ciPassed := out[strings.Index(out, "CI Passed"):strings.Index(out, "Deployed")]
	if strings.Contains(ciPassed, "carol") {
		t.Errorf("the revert pair is still waiting in CI Passed:\n%s", out)
	}

	// The deploy badge answers "is production current?" — after a verified
	// no-op it is.
	header := strings.SplitN(out, "\n", 2)[0]
	if !strings.Contains(header, "deploy: ✓ passed") {
		t.Errorf("expected a green deploy badge after a no-op, got header: %q", header)
	}

	// The live marker is a state claim, and the no-op is the freshest
	// verified statement of it — so the top batch holds the marker, and the
	// revert pair reads as live rather than as waiting above the live line.
	liveAt := strings.Index(out, "(live)")
	if liveAt == -1 {
		t.Fatalf("expected a live batch, got:\n%s", out)
	}
	noopAt := strings.Index(out, "deploy no-op")
	if liveAt < noopAt || !strings.Contains(out[noopAt:liveAt], "no-op") {
		t.Errorf("expected the live marker on the no-op batch's own line:\n%s", out)
	}

	// One deploy, not two: the week line counts alice's ship alone. All three
	// events land in 1970-W01, so the current-week header says nothing — the
	// week divider logic is pinned elsewhere; here the count is the point.
	if strings.Contains(out, "2 deploys") {
		t.Errorf("a no-op inflated the deploy count:\n%s", out)
	}
	if !strings.Contains(out, "1 deploy") {
		t.Errorf("expected the real deploy still counted once, got:\n%s", out)
	}
}
