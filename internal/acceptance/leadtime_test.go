// Package acceptance_test wires the real composition together — config file,
// lens, renderers — and asserts on what a user would actually see.
//
// It exists because a configurable behaviour can be correct in every unit and
// still not reach the screen. `clarity.leadTime` shipped that way: core
// computed all three modes correctly and had tests proving it, while both
// renderers quietly recomputed their own groupings from the raw snapshot and
// threw the configured View away. Every layer passed. The feature did nothing.
//
// So these tests start from an .ezcd.json on disk and finish at rendered
// output, crossing every seam in between.
package acceptance_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/adapters/plain"
	"github.com/ezcdlabs/clarity/internal/adapters/tui"
	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/core"
)

// fakeSource emits one snapshot and closes, standing in for git or the
// GitHub API so the test exercises the wiring rather than the I/O.
type fakeSource struct{ snap core.Snapshot }

func (f *fakeSource) Watch(ctx context.Context) <-chan core.Snapshot {
	ch := make(chan core.Snapshot, 1)
	ch <- f.snap
	close(ch)
	return ch
}

// batchedPushSnapshot is the shape lead time modes exist to handle: two
// commits shipped by one deploy, where only the pushed head carries pipeline
// events. Both reach production; only one was ever seen by CI.
func batchedPushSnapshot() core.Snapshot {
	authored := time.Unix(1000, 0)
	ciStarted := time.Unix(1200, 0)
	deployed := time.Unix(1500, 0)

	return core.Snapshot{
		RepoName: "clarity",
		Commits: []core.CommitView{
			{
				SHA: "aaa1111111111111111111111111111111111111",
				Author: "alice", Subject: "pushed head", Time: authored,
				Events: []clarityrefs.Event{
					{Stage: "ci", Status: "started", Time: ciStarted},
					{Stage: "ci", Status: "passed", Time: time.Unix(1300, 0)},
					{Stage: "deploy", Status: "passed", Time: deployed},
				},
			},
			{
				SHA: "bbb2222222222222222222222222222222222222",
				Author: "bob", Subject: "swept along", Time: time.Unix(900, 0),
				// No events: CI ran on the head, not on this commit.
			},
		},
	}
}

// elapsedPattern matches the compact durations FormatElapsed renders, which
// is how a lead time appears on a row: "5m 00s", "1h 30m", "45s".
var elapsedPattern = regexp.MustCompile(`\d+[hms]`)

// ansiPattern matches the colour escapes the TUI emits. They must be stripped
// before looking for a duration: "\x1b[90m" ends in digits-then-m and matches
// elapsedPattern on its own, which quietly turns every TUI assertion into a
// pass.
var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

// hasLeadTime reports whether a rendered row carries a duration in its
// right-hand column.
func hasLeadTime(row string) bool {
	return elapsedPattern.MatchString(ansiPattern.ReplaceAllString(row, ""))
}

// rowFor returns the rendered line for a commit subject.
func rowFor(t *testing.T, out, subject string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, subject) {
			return line
		}
	}
	t.Fatalf("no row rendered for %q in:\n%s", subject, out)
	return ""
}

// viewFor runs the full path a user's configuration takes: an .ezcd.json on
// disk, through config.Load, into the Lens, out as the View a renderer gets.
func viewFor(t *testing.T, leadTime string, snap core.Snapshot) core.View {
	t.Helper()

	dir := t.TempDir()
	body := `{"clarity": {}}`
	if leadTime != "" {
		body = `{"clarity": {"leadTime": "` + leadTime + `"}}`
	}
	if err := os.WriteFile(filepath.Join(dir, ".ezcd.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write .ezcd.json: %v", err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	lens := core.NewLens(&fakeSource{snap: snap}, cfg.LeadTimeMode())
	select {
	case v, ok := <-lens.Views(t.Context()):
		if !ok {
			t.Fatal("lens closed without emitting a view")
		}
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the lens to emit")
		return core.View{}
	}
}

// TestLeadTimeMode_ReachesRenderedRows is the test that would have caught the
// renderers ignoring their View. It asserts on the rendered row, not on the
// derived model, because the row is the thing the user reads.
func TestLeadTimeMode_ReachesRenderedRows(t *testing.T) {
	now := time.Unix(9999, 0)

	cases := []struct {
		leadTime string
		// does the commit with no events of its own show a lead time?
		wantSweptAlongTimed bool
		// what the pushed head's lead time reads
		wantHead string
	}{
		// Default: every deployed commit is timed from its authoring.
		{leadTime: "", wantSweptAlongTimed: true, wantHead: "8m 20s"},
		{leadTime: "all", wantSweptAlongTimed: true, wantHead: "8m 20s"},
		// Reported: the swept-along commit drops out, the head keeps its
		// authoring-to-deploy time.
		{leadTime: "reported", wantSweptAlongTimed: false, wantHead: "8m 20s"},
		// Pipeline: the head is timed from ci started instead.
		{leadTime: "pipeline", wantSweptAlongTimed: false, wantHead: "5m 00s"},
	}

	for _, c := range cases {
		name := c.leadTime
		if name == "" {
			name = "unset"
		}
		t.Run(name, func(t *testing.T) {
			view := viewFor(t, c.leadTime, batchedPushSnapshot())

			renderers := map[string]string{
				"tui":   tui.RenderSnapshot(view, 100, now, 0),
				"plain": plain.RenderSnapshot(view.Snapshot.RepoName, view, now, plain.Options{}),
			}

			for kind, out := range renderers {
				head := rowFor(t, out, "pushed head")
				if !strings.Contains(head, c.wantHead) {
					t.Errorf("%s: head row should show lead time %q, got: %q",
						kind, c.wantHead, head)
				}

				swept := rowFor(t, out, "swept along")
				gotTimed := hasLeadTime(swept)
				if gotTimed != c.wantSweptAlongTimed {
					verb := "should not"
					if c.wantSweptAlongTimed {
						verb = "should"
					}
					t.Errorf("%s: commit with no events %s show a lead time, got: %q",
						kind, verb, swept)
				}
			}
		})
	}
}

// TestLeadTimeMode_ReachesWeeklyAverage covers the other number the mode
// changes. The weekly summary is rendered from the same View, so it can drift
// away from the per-row values in exactly the same way.
func TestLeadTimeMode_ReachesWeeklyAverage(t *testing.T) {
	now := time.Unix(9999, 0)

	// all:      (500s + 600s) / 2 = 9m 10s   — both commits contribute
	// reported: 500s                = 8m 20s   — only the head
	// pipeline: 300s                = 5m 00s   — head, from ci started
	for leadTime, want := range map[string]string{
		"all":      "9m 10s",
		"reported": "8m 20s",
		"pipeline": "5m 00s",
	} {
		t.Run(leadTime, func(t *testing.T) {
			view := viewFor(t, leadTime, batchedPushSnapshot())

			out := plain.RenderSnapshot(view.Snapshot.RepoName, view, now, plain.Options{})
			if !strings.Contains(out, want) {
				t.Errorf("weekly average should read %q, got:\n%s", want, out)
			}
		})
	}
}

// TestLeadTimeMode_ExcludedCommitsStillAppear pins the half of the behaviour
// that isn't about the metric: a commit dropped from the average is still part
// of the history and must stay on screen.
func TestLeadTimeMode_ExcludedCommitsStillAppear(t *testing.T) {
	now := time.Unix(9999, 0)

	for _, leadTime := range []string{"all", "reported", "pipeline"} {
		view := viewFor(t, leadTime, batchedPushSnapshot())
		for kind, out := range map[string]string{
			"tui":   tui.RenderSnapshot(view, 100, now, 0),
			"plain": plain.RenderSnapshot(view.Snapshot.RepoName, view, now, plain.Options{}),
		} {
			if !strings.Contains(out, "swept along") {
				t.Errorf("%s/%s: excluded commit must still be listed, got:\n%s",
					leadTime, kind, out)
			}
		}
	}
}
