package clarityrefs_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

const fakeSHA = "0123456789abcdef0123456789abcdef01234567"
const fakeSHA2 = "fedcba9876543210fedcba9876543210fedcba98"

// TestReadEvents_ReturnsEmpty_WhenRefMissing verifies that a fresh repo with no
// events ref returns no events and no error.
func TestReadEvents_ReturnsEmpty_WhenRefMissing(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	events, err := clarityrefs.ReadEvents(clone.Path, fakeSHA)
	if err != nil {
		t.Fatalf("ReadEvents on empty repo should not error, got: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events, got %d", len(events))
	}
}

// TestReadAllEvents_ReturnsEmpty_WhenRefMissing verifies the same for ReadAllEvents.
func TestReadAllEvents_ReturnsEmpty_WhenRefMissing(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	events, err := clarityrefs.ReadAllEvents(clone.Path)
	if err != nil {
		t.Fatalf("ReadAllEvents on empty repo should not error, got: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(events))
	}
}

// TestWriteEvent_CreatesFileOnRemote verifies that WriteEvent produces a file
// on the events ref of the remote.
func TestWriteEvent_CreatesFileOnRemote(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	ev := clarityrefs.Event{
		Stage:  "ci",
		Status: "passed",
		Time:   time.Unix(1744120134, 0),
	}
	if err := clarityrefs.WriteEvent(clone.Path, "origin", fakeSHA, ev); err != nil {
		t.Fatalf("WriteEvent failed: %v", err)
	}

	refs := remote.ListRefs()
	found := false
	for _, r := range refs {
		if r == "refs/clarity/events" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected refs/clarity/events on remote, got: %v", refs)
	}
}

// TestWriteEvent_BypassesPrePushHook verifies that WriteEvent's push to
// refs/clarity/events is not blocked by a user-installed pre-push hook in
// the clone. The events ref is internal bookkeeping; users' pre-push hooks
// (typically gating real code pushes) shouldn't apply to it.
func TestWriteEvent_BypassesPrePushHook(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Pre-push hooks rely on a POSIX shebang; the equivalent on Windows
		// would need a .bat or interpreter on PATH. Skip rather than skew
		// the test for the rare Windows-host case.
		t.Skip("pre-push hook scripting is POSIX-only")
	}
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	hookPath := filepath.Join(clone.Path, ".git", "hooks", "pre-push")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("install pre-push hook: %v", err)
	}

	ev := clarityrefs.Event{
		Stage:  "ci",
		Status: "passed",
		Time:   time.Unix(1744120134, 0),
	}
	if err := clarityrefs.WriteEvent(clone.Path, "origin", fakeSHA, ev); err != nil {
		t.Fatalf("WriteEvent must not be blocked by a pre-push hook, got: %v", err)
	}
}

// TestWriteEvent_ReadRoundTrip verifies that an event written can be read back
// with all fields intact.
func TestWriteEvent_ReadRoundTrip(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	ev := clarityrefs.Event{
		Stage:  "deploy",
		Status: "passed",
		Time:   time.Unix(1744120200, 0),
		CI: map[string]string{
			"system":  "github-actions",
			"run_id":  "12345",
			"run_url": "https://example.test/runs/12345",
			"actor":   "alice",
		},
	}
	if err := clarityrefs.WriteEvent(clone.Path, "origin", fakeSHA, ev); err != nil {
		t.Fatalf("WriteEvent failed: %v", err)
	}

	// Fresh clone — reads the events ref freshly from the remote.
	reader := remote.NewClone(t)
	if err := fetchEventsRef(reader.Path); err != nil {
		t.Fatalf("fetch events ref: %v", err)
	}

	got, err := clarityrefs.ReadEvents(reader.Path, fakeSHA)
	if err != nil {
		t.Fatalf("ReadEvents failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Stage != ev.Stage || got[0].Status != ev.Status {
		t.Errorf("expected %+v, got %+v", ev, got[0])
	}
	if !got[0].Time.Equal(ev.Time) {
		t.Errorf("expected time %v, got %v", ev.Time, got[0].Time)
	}
	for k, v := range ev.CI {
		if got[0].CI[k] != v {
			t.Errorf("CI[%q]: expected %q, got %q", k, v, got[0].CI[k])
		}
	}
}

// TestReadEvents_SortsByTimestamp verifies events for a given SHA are returned
// in ascending timestamp order regardless of write order.
func TestReadEvents_SortsByTimestamp(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	// Write out of order.
	times := []int64{1744120300, 1744120100, 1744120200}
	for _, ts := range times {
		ev := clarityrefs.Event{
			Stage:  "ci",
			Status: "passed",
			Time:   time.Unix(ts, 0),
		}
		if err := clarityrefs.WriteEvent(clone.Path, "origin", fakeSHA, ev); err != nil {
			t.Fatalf("WriteEvent: %v", err)
		}
	}

	if err := fetchEventsRef(clone.Path); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	got, err := clarityrefs.ReadEvents(clone.Path, fakeSHA)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Time.After(got[i].Time) {
			t.Errorf("events not sorted ascending: index %d (%v) is after index %d (%v)",
				i-1, got[i-1].Time, i, got[i].Time)
		}
	}
}

// TestReadAllEvents_GroupsBySHA verifies ReadAllEvents returns a map keyed by
// SHA with all events for each SHA.
func TestReadAllEvents_GroupsBySHA(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	for _, ev := range []struct {
		sha   string
		stage string
	}{
		{fakeSHA, "ci"},
		{fakeSHA, "deploy"},
		{fakeSHA2, "ci"},
	} {
		e := clarityrefs.Event{Stage: ev.stage, Status: "passed", Time: time.Now()}
		if err := clarityrefs.WriteEvent(clone.Path, "origin", ev.sha, e); err != nil {
			t.Fatalf("WriteEvent: %v", err)
		}
	}

	if err := fetchEventsRef(clone.Path); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	all, err := clarityrefs.ReadAllEvents(clone.Path)
	if err != nil {
		t.Fatalf("ReadAllEvents: %v", err)
	}
	if len(all[fakeSHA]) != 2 {
		t.Errorf("expected 2 events for fakeSHA, got %d", len(all[fakeSHA]))
	}
	if len(all[fakeSHA2]) != 1 {
		t.Errorf("expected 1 event for fakeSHA2, got %d", len(all[fakeSHA2]))
	}
}

// TestWriteEvent_FilenameIsContentAddressed verifies that writing the same
// event twice produces only ONE file on the events ref. Content-addressed
// filenames are what make a backfill re-run idempotent: identical input
// collapses into the same tree entry rather than accumulating duplicates.
func TestWriteEvent_FilenameIsContentAddressed(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	ev := clarityrefs.Event{Stage: "ci", Status: "passed", Time: time.Unix(1744120134, 0)}
	if err := clarityrefs.WriteEvent(clone.Path, "origin", fakeSHA, ev); err != nil {
		t.Fatalf("WriteEvent (first): %v", err)
	}
	if err := clarityrefs.WriteEvent(clone.Path, "origin", fakeSHA, ev); err != nil {
		t.Fatalf("WriteEvent (second): %v", err)
	}

	if err := fetchEventsRef(clone.Path); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	got, err := clarityrefs.ReadEvents(clone.Path, fakeSHA)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("writing the same event twice should produce 1 file, got %d", len(got))
	}
}

// TestWriteEvent_DifferentEventsKeepDistinctFiles verifies that the content-
// addressed naming still preserves genuinely-different events (different
// status) under the same SHA — distinct content → distinct hash → distinct
// filename.
func TestWriteEvent_DifferentEventsKeepDistinctFiles(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	for _, ev := range []clarityrefs.Event{
		{Stage: "ci", Status: "started", Time: time.Unix(1744120000, 0)},
		{Stage: "ci", Status: "passed", Time: time.Unix(1744120134, 0)},
	} {
		if err := clarityrefs.WriteEvent(clone.Path, "origin", fakeSHA, ev); err != nil {
			t.Fatalf("WriteEvent: %v", err)
		}
	}

	if err := fetchEventsRef(clone.Path); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	got, err := clarityrefs.ReadEvents(clone.Path, fakeSHA)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("two distinct events should produce 2 files, got %d", len(got))
	}
}

// TestWriteEvents_BatchIsIdempotent verifies that re-running an identical
// batch creates no new commit and no duplicate event files. This is what
// lets users safely re-run a backfill after a partial / interrupted run.
func TestWriteEvents_BatchIsIdempotent(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	batch := map[string][]clarityrefs.Event{
		fakeSHA: {
			{Stage: "ci", Status: "passed", Time: time.Unix(1744120134, 0)},
			{Stage: "deploy", Status: "passed", Time: time.Unix(1744120200, 0)},
		},
		fakeSHA2: {
			{Stage: "ci", Status: "passed", Time: time.Unix(1744120300, 0)},
		},
	}

	if err := clarityrefs.WriteEvents(clone.Path, "origin", batch); err != nil {
		t.Fatalf("first WriteEvents: %v", err)
	}
	first := len(remote.LogBranch("refs/clarity/events"))
	if first != 1 {
		t.Fatalf("expected 1 commit after first batch, got %d", first)
	}

	if err := clarityrefs.WriteEvents(clone.Path, "origin", batch); err != nil {
		t.Fatalf("second WriteEvents: %v", err)
	}
	second := len(remote.LogBranch("refs/clarity/events"))
	if second != first {
		t.Errorf("identical re-run should not add a commit, got %d -> %d", first, second)
	}

	if err := fetchEventsRef(clone.Path); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	all, err := clarityrefs.ReadAllEvents(clone.Path)
	if err != nil {
		t.Fatalf("ReadAllEvents: %v", err)
	}
	if len(all[fakeSHA]) != 2 {
		t.Errorf("expected 2 events for fakeSHA, got %d", len(all[fakeSHA]))
	}
	if len(all[fakeSHA2]) != 1 {
		t.Errorf("expected 1 event for fakeSHA2, got %d", len(all[fakeSHA2]))
	}
}

// TestWriteEvents_BatchesIntoSingleCommit verifies that WriteEvents writes an
// arbitrary number of events spanning multiple commit SHAs in a SINGLE commit
// + push on the events ref. This is the property that lets the backfill
// generator amortise the fetch/push round-trip cost across many events.
func TestWriteEvents_BatchesIntoSingleCommit(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	batch := map[string][]clarityrefs.Event{
		fakeSHA: {
			{Stage: "ci", Status: "started", Time: time.Unix(1744120000, 0)},
			{Stage: "ci", Status: "passed", Time: time.Unix(1744120134, 0)},
		},
		fakeSHA2: {
			{Stage: "ci", Status: "passed", Time: time.Unix(1744120200, 0)},
			{Stage: "deploy", Status: "passed", Time: time.Unix(1744120300, 0)},
		},
	}

	if err := clarityrefs.WriteEvents(clone.Path, "origin", batch); err != nil {
		t.Fatalf("WriteEvents failed: %v", err)
	}

	// Exactly one commit should land on the events ref — the whole point of
	// the batch primitive is to amortise the push round-trip.
	commits := remote.LogBranch("refs/clarity/events")
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit on events ref, got %d", len(commits))
	}

	if err := fetchEventsRef(clone.Path); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	all, err := clarityrefs.ReadAllEvents(clone.Path)
	if err != nil {
		t.Fatalf("ReadAllEvents: %v", err)
	}
	if len(all[fakeSHA]) != 2 {
		t.Errorf("expected 2 events for fakeSHA, got %d", len(all[fakeSHA]))
	}
	if len(all[fakeSHA2]) != 2 {
		t.Errorf("expected 2 events for fakeSHA2, got %d", len(all[fakeSHA2]))
	}
}

// TestWriteEvents_EmptyBatchIsNoop verifies that WriteEvents with no events
// does nothing and does not error — important for callers that may filter
// down to an empty input.
func TestWriteEvents_EmptyBatchIsNoop(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	if err := clarityrefs.WriteEvents(clone.Path, "origin", nil); err != nil {
		t.Fatalf("WriteEvents(nil) should not error, got: %v", err)
	}
	for _, r := range remote.ListRefs() {
		if r == "refs/clarity/events" {
			t.Errorf("empty batch should not create the events ref, but it exists")
		}
	}
}

// TestWriteEvent_FilenameFormat verifies event filenames match
// "<unix-ts>-<short-id>.json".
func TestWriteEvent_FilenameFormat(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	ev := clarityrefs.Event{Stage: "ci", Status: "passed", Time: time.Unix(1744120134, 0)}
	if err := clarityrefs.WriteEvent(clone.Path, "origin", fakeSHA, ev); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}

	cmd := exec.Command("git", "ls-tree", "-r", "refs/clarity/events")
	cmd.Dir = remote.Path
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git ls-tree: %v\n%s", err, out)
	}

	pattern := regexp.MustCompile(`events/[0-9a-f]{40}/\d+-[0-9a-f]+\.json$`)
	found := false
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if tab := strings.IndexByte(line, '\t'); tab >= 0 {
			path := line[tab+1:]
			if pattern.MatchString(path) {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected an event file matching %s, got:\n%s", pattern, out)
	}
}

// TestWriteEvent_TreeHasNoFullPathnames verifies the events tree is built with
// proper nested sub-trees rather than flat entries with slashes — GitHub's
// receive.fsckObjects rejects the latter ("fullPathname" check).
func TestWriteEvent_TreeHasNoFullPathnames(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	ev := clarityrefs.Event{Stage: "ci", Status: "passed", Time: time.Unix(1744120134, 0)}
	if err := clarityrefs.WriteEvent(clone.Path, "origin", fakeSHA, ev); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}

	cmd := exec.Command("git", "ls-tree", "refs/clarity/events")
	cmd.Dir = remote.Path
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git ls-tree: %v\n%s", err, out)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		if tab := strings.IndexByte(line, '\t'); tab >= 0 {
			name := line[tab+1:]
			if strings.Contains(name, "/") {
				t.Errorf("root tree entry %q contains a slash — invalid tree object rejected by GitHub fsck", name)
			}
		}
	}
}

// TestWriteEvent_ConcurrentWrites_BothLand verifies that two clones writing
// events simultaneously both end up on the events ref (optimistic FF retry).
func TestWriteEvent_ConcurrentWrites_BothLand(t *testing.T) {
	remote := gittest.NewRemote(t)
	alice := remote.NewClone(t)
	bob := remote.NewClone(t)

	var wg sync.WaitGroup
	var aliceErr, bobErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		ev := clarityrefs.Event{Stage: "ci", Status: "passed", Time: time.Unix(1744120100, 0)}
		aliceErr = clarityrefs.WriteEvent(alice.Path, "origin", fakeSHA, ev)
	}()
	go func() {
		defer wg.Done()
		ev := clarityrefs.Event{Stage: "deploy", Status: "passed", Time: time.Unix(1744120200, 0)}
		bobErr = clarityrefs.WriteEvent(bob.Path, "origin", fakeSHA, ev)
	}()
	wg.Wait()

	if aliceErr != nil {
		t.Errorf("alice WriteEvent failed: %v", aliceErr)
	}
	if bobErr != nil {
		t.Errorf("bob WriteEvent failed: %v", bobErr)
	}

	reader := remote.NewClone(t)
	if err := fetchEventsRef(reader.Path); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	got, err := clarityrefs.ReadEvents(reader.Path, fakeSHA)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 events after concurrent writes, got %d: %+v", len(got), got)
	}
}

// TestEvent_JSONShape verifies the on-disk JSON shape matches the README:
// {"stage":..., "status":..., "ts":..., "ci":{...}}
func TestEvent_JSONShape(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	ev := clarityrefs.Event{
		Stage:  "ci",
		Status: "passed",
		Time:   time.Unix(1744120134, 0),
		CI:     map[string]string{"system": "github-actions"},
	}
	if err := clarityrefs.WriteEvent(clone.Path, "origin", fakeSHA, ev); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}

	cmd := exec.Command("git", "ls-tree", "-r", "refs/clarity/events")
	cmd.Dir = remote.Path
	out, _ := cmd.CombinedOutput()
	var path string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if tab := strings.IndexByte(line, '\t'); tab >= 0 {
			p := line[tab+1:]
			if strings.HasPrefix(p, "events/") && strings.HasSuffix(p, ".json") {
				path = p
				break
			}
		}
	}
	if path == "" {
		t.Fatal("could not find event file in tree")
	}

	content, ok := remote.ReadFileAtRef("refs/clarity/events", path)
	if !ok {
		t.Fatalf("could not read %s", path)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, content)
	}
	if parsed["stage"] != "ci" {
		t.Errorf("expected stage=ci, got %v", parsed["stage"])
	}
	if parsed["status"] != "passed" {
		t.Errorf("expected status=passed, got %v", parsed["status"])
	}
	if ts, ok := parsed["ts"].(float64); !ok || int64(ts) != 1744120134 {
		t.Errorf("expected ts=1744120134, got %v (%T)", parsed["ts"], parsed["ts"])
	}
	ci, _ := parsed["ci"].(map[string]any)
	if ci == nil || ci["system"] != "github-actions" {
		t.Errorf("expected ci.system=github-actions, got %v", parsed["ci"])
	}
}

// fetchEventsRef pulls the remote events ref into the clone for read-side tests.
// (The watcher / TUI normally does this via internal/refs.)
func fetchEventsRef(repoPath string) error {
	cmd := exec.Command("git", "fetch", "origin", "+refs/clarity/events:refs/clarity/events")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(out), "couldn't find remote ref") {
		return err
	}
	return nil
}
