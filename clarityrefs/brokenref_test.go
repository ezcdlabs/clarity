package clarityrefs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// breakEventsRef points the local events ref at a well-formed object id that
// is not in the store, which is what an evicted shared object cache leaves
// behind: the ref survives in the workspace, the objects it names do not.
//
// Written directly rather than through `git update-ref`, which refuses to
// point a ref at a nonexistent object — the state has to be manufactured the
// way reality manufactures it.
func breakEventsRef(t *testing.T, repoPath string) {
	t.Helper()
	path := filepath.Join(repoPath, ".git", "refs", "clarity", "events")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	const absent = "1111111111111111111111111111111111111111\n"
	if err := os.WriteFile(path, []byte(absent), 0o644); err != nil {
		t.Fatalf("writing broken ref: %v", err)
	}
}

// TestReadAllEvents_UnresolvableRefIsAnError pins the difference between a
// repo that has never reported and one whose events ref cannot be resolved.
//
// They are opposite situations that look alike at the point of failure: the
// first is every repo before its first report, the second means the history
// is there but unreadable. Reporting the second as the first renders an empty
// dashboard with no diagnostic at all — quieter than the "object not found"
// this whole change set exists to eliminate, and reachable by exactly the
// route that motivated it, a shared object cache evicted between jobs.
func TestReadAllEvents_UnresolvableRefIsAnError(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	ev := clarityrefs.Event{Stage: "ci", Status: "passed", Time: time.Unix(1744120134, 0)}
	if err := clarityrefs.WriteEvent(clone.Path, "origin", fakeSHA, ev); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	breakEventsRef(t, clone.Path)

	all, err := clarityrefs.ReadAllEvents(clone.Path)
	if err == nil {
		t.Fatalf("an unresolvable events ref must not read as 'never reported'; got %d entries, nil error", len(all))
	}
	if !strings.Contains(err.Error(), clarityrefs.EventsRef) {
		t.Errorf("error should name the ref it could not resolve, got: %v", err)
	}
}

// TestReadAllEvents_AbsentRefIsNotAnError is the other half, and the reason
// the distinction has to be made carefully rather than by failing on
// anything unusual.
func TestReadAllEvents_AbsentRefIsNotAnError(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	all, err := clarityrefs.ReadAllEvents(clone.Path)
	if err != nil {
		t.Fatalf("a repo that has never reported is not an error, got: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected no events, got %d", len(all))
	}
}
