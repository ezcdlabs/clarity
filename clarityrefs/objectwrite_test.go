package clarityrefs

import (
	"os"
	"syscall"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// flakyStorer fails the first failures calls to SetEncodedObject with err,
// then succeeds. Stands in for a concurrent `git gc` rmdir'ing the
// .git/objects/<xx> directory that go-billy just created.
type flakyStorer struct {
	storer.EncodedObjectStorer // nil — only SetEncodedObject is exercised
	failures                   int
	err                        error
	calls                      int
}

func (f *flakyStorer) SetEncodedObject(plumbing.EncodedObject) (plumbing.Hash, error) {
	f.calls++
	if f.calls <= f.failures {
		return plumbing.ZeroHash, f.err
	}
	return plumbing.NewHash("9f9edc8673b331befd2adda3eadb62effde0fbe9"), nil
}

func renameRace() error {
	return &os.LinkError{
		Op:  "rename",
		Old: ".git/objects/pack/tmp_obj_1741553381",
		New: ".git/objects/9f/9edc8673b331befd2adda3eadb62effde0fbe9",
		Err: syscall.ENOENT,
	}
}

// TestStoreObject_RetriesTransientRenameRace pins the fix for the GitHub
// Actions failure: a loose-object write that loses the race with a concurrent
// gc must be retried, not surfaced. The retry re-runs go-billy's
// MkdirAll+rename, which re-creates the pruned directory.
func TestStoreObject_RetriesTransientRenameRace(t *testing.T) {
	st := &flakyStorer{failures: 1, err: renameRace()}

	h, err := storeObject(st, nil)
	if err != nil {
		t.Fatalf("storeObject should have retried past the race, got: %v", err)
	}
	if h.IsZero() {
		t.Error("expected the hash from the successful retry, got zero")
	}
	if st.calls != 2 {
		t.Errorf("expected 1 failure + 1 retry = 2 calls, got %d", st.calls)
	}
}

// TestStoreObject_GivesUpAfterMaxAttempts stops a pathologically unlucky
// repo (or a genuinely broken object store reporting ENOENT) from looping
// forever. The final error must be the underlying one, not a synthetic
// "gave up" message that hides what actually failed.
func TestStoreObject_GivesUpAfterMaxAttempts(t *testing.T) {
	st := &flakyStorer{failures: objectWriteAttempts + 1, err: renameRace()}

	if _, err := storeObject(st, nil); err == nil {
		t.Fatal("expected an error once the attempt budget is exhausted")
	} else if !isTransientObjectWriteError(err) {
		t.Errorf("expected the underlying rename error to surface, got: %v", err)
	}
	if st.calls != objectWriteAttempts {
		t.Errorf("expected %d attempts, got %d", objectWriteAttempts, st.calls)
	}
}

// TestStoreObject_DoesNotRetryRealErrors guards the retry from swallowing
// genuine failures — a full disk or a read-only object store must fail on
// the first attempt rather than burning the whole attempt budget.
func TestStoreObject_DoesNotRetryRealErrors(t *testing.T) {
	st := &flakyStorer{
		failures: 1,
		err:      &os.LinkError{Op: "rename", Err: syscall.EACCES},
	}

	if _, err := storeObject(st, nil); err == nil {
		t.Fatal("expected the permission error to surface")
	}
	if st.calls != 1 {
		t.Errorf("expected no retry on a non-transient error, got %d calls", st.calls)
	}
}
