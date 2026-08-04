package clarityrefs

import (
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/internal/gittest"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// objectDir matches a loose-object fan-out directory, .git/objects/<xx>.
var objectDir = regexp.MustCompile(`^[0-9a-f]{2}$`)

// racingStorer fails every nth object write the way a lost gc race does, and
// succeeds on the immediate retry — matching what happens for real, where the
// retry re-runs go-billy's MkdirAll and re-creates the pruned directory.
type racingStorer struct {
	storer.EncodedObjectStorer
	every  int64
	writes atomic.Int64
	failed atomic.Int64
}

func (r *racingStorer) SetEncodedObject(obj plumbing.EncodedObject) (plumbing.Hash, error) {
	if r.writes.Add(1)%r.every == 0 {
		r.failed.Add(1)
		return plumbing.ZeroHash, &os.LinkError{
			Op:  "rename",
			Old: ".git/objects/pack/tmp_obj_1741553381",
			New: ".git/objects/9f/9edc8673b331befd2adda3eadb62effde0fbe9",
			Err: syscall.ENOENT,
		}
	}
	return r.EncodedObjectStorer.SetEncodedObject(obj)
}

// TestWriteEvents_SurvivesObjectWriteRace is the end-to-end regression test
// for the failure reported from a GitHub Actions deploy job:
//
//	error: build tree: rename .git/objects/pack/tmp_obj_1741553381
//	  .git/objects/9f/9edc86...: no such file or directory
//
// go-git stores a loose object by streaming it to .git/objects/pack/tmp_obj_*
// and renaming it into .git/objects/<xx>/<rest>; go-billy MkdirAlls the
// destination's parent immediately before the rename(2). `git gc` rmdirs
// empty .git/objects/<xx> directories, so a gc landing between those two
// syscalls makes the rename fail ENOENT. On a runner the racing gc is the
// detached `gc --auto` that `git fetch` spawns — which is why the fetch
// WriteEvents performs a few statements before building the tree is the
// trigger.
//
// The race is injected rather than provoked. Both halves of the mechanism
// were confirmed against real git first: `git gc` does rmdir an empty
// .git/objects/<xx> even at its default 2-week prune expiry, and looping it
// against go-git writes reproduces this exact error within a few hundred
// objects. What can't be done from outside is hitting the window on demand —
// it is the gap between two adjacent syscalls inside go-billy. Provoking it
// with a real gc means either a flaky test or `--prune=now`, which corrupts
// the in-flight write in a way auto-gc never does and so tests the wrong
// failure. Injecting keeps the test deterministic and covers what matters
// here: that every object write in the path retries, blobs, subtrees and
// commit alike.
func TestWriteEvents_SurvivesObjectWriteRace(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	// Fail often enough to hit blobs, subtrees and the commit object.
	race := &racingStorer{every: 3}
	original := writeStorer
	writeStorer = func(repo *gogit.Repository) storer.EncodedObjectStorer {
		race.EncodedObjectStorer = repo.Storer
		return race
	}
	t.Cleanup(func() { writeStorer = original })

	events := map[string][]Event{
		"0123456789abcdef0123456789abcdef01234567": {
			{Stage: "ci", Status: "started", Time: time.Unix(1744120000, 0)},
			{Stage: "ci", Status: "passed", Time: time.Unix(1744120134, 0)},
			{Stage: "deploy", Status: "passed", Time: time.Unix(1744120200, 0)},
		},
		"fedcba9876543210fedcba9876543210fedcba98": {
			{Stage: "ci", Status: "passed", Time: time.Unix(1744120300, 0)},
		},
	}

	if err := WriteEvents(clone.Path, "origin", events); err != nil {
		t.Fatalf("WriteEvents must survive the object write race, got: %v", err)
	}
	if race.failed.Load() == 0 {
		t.Fatal("no writes were made to fail — the test isn't exercising the race")
	}

	// Survived isn't enough: every event has to land. The events ref is
	// pushed and the local copy dropped, so read it back from origin.
	if err := fetchEventsRef(clone.Path, "origin"); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	got, err := ReadAllEvents(clone.Path)
	if err != nil {
		t.Fatalf("ReadAllEvents: %v", err)
	}
	for sha, want := range events {
		if len(got[sha]) != len(want) {
			t.Errorf("sha %s: got %d events, want %d", sha, len(got[sha]), len(want))
		}
	}
}

// looseObjects returns the ids of every loose object in the repository.
func looseObjects(t *testing.T, repoPath string) map[string]bool {
	t.Helper()
	objects := filepath.Join(repoPath, ".git", "objects")
	entries, err := os.ReadDir(objects)
	if err != nil {
		t.Fatalf("read objects dir: %v", err)
	}
	found := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() || !objectDir.MatchString(e.Name()) {
			continue
		}
		loose, err := os.ReadDir(filepath.Join(objects, e.Name()))
		if err != nil {
			continue
		}
		for _, o := range loose {
			found[e.Name()+o.Name()] = true
		}
	}
	return found
}

// pruneNewLooseObjects deletes the loose objects that appeared since before
// was taken, emulating what `git gc --prune=now` does to objects a write has
// produced but not yet made reachable. Scoped to what the write created:
// pruning indiscriminately would take the repository's own history with it,
// which no gc would ever do.
func pruneNewLooseObjects(t *testing.T, repoPath string, before map[string]bool) int {
	t.Helper()
	objects := filepath.Join(repoPath, ".git", "objects")
	pruned := 0
	for id := range looseObjects(t, repoPath) {
		if before[id] {
			continue
		}
		if err := os.Remove(filepath.Join(objects, id[:2], id[2:])); err == nil {
			pruned++
		}
	}
	return pruned
}

// TestWriteEvents_RecoversWhenGCPrunesObjectsBeforePush covers the second way
// a concurrent gc breaks a write, found while building the retry above.
//
// The objects a write creates — blobs, subtrees, the commit — are unreachable
// until the events ref is moved to point at them. A gc pruning aggressively
// enough to collect objects that young deletes them in that window, and the
// push that follows fails locally while packing what it can no longer read.
// Retrying the object write can't help: by then the objects are gone.
//
// Unlike the rmdir race this needs `gc --prune=now` or a configured
// gc.pruneExpire — auto-gc's two-week default never collects an object this
// young — so it is a narrower exposure, but a real one for repos whose
// workflows run their own gc.
//
// Emptying the object store in the push seam reproduces it exactly: git
// itself produces the failure, so the recovery is driven by the real error
// rather than a guess at its wording.
func TestWriteEvents_RecoversWhenGCPrunesObjectsBeforePush(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	preexisting := looseObjects(t, clone.Path)
	original := pushEvents
	var once sync.Once
	pushEvents = func(repoPath, rem string) error {
		once.Do(func() {
			if pruneNewLooseObjects(t, repoPath, preexisting) == 0 {
				t.Error("nothing was pruned — the test isn't reproducing the failure")
			}
		})
		return original(repoPath, rem)
	}
	t.Cleanup(func() { pushEvents = original })

	events := map[string][]Event{
		"0123456789abcdef0123456789abcdef01234567": {
			{Stage: "ci", Status: "passed", Time: time.Unix(1744120134, 0)},
			{Stage: "deploy", Status: "passed", Time: time.Unix(1744120200, 0)},
		},
	}

	if err := WriteEvents(clone.Path, "origin", events); err != nil {
		t.Fatalf("WriteEvents must rebuild the pruned objects and retry, got: %v", err)
	}

	if err := fetchEventsRef(clone.Path, "origin"); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	got, err := ReadAllEvents(clone.Path)
	if err != nil {
		t.Fatalf("ReadAllEvents: %v", err)
	}
	for sha, want := range events {
		if len(got[sha]) != len(want) {
			t.Errorf("sha %s: got %d events, want %d", sha, len(got[sha]), len(want))
		}
	}
}
