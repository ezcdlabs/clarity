package clarityrefs

import (
	"errors"
	"io/fs"
	"os"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// writeStorer returns the object store updateEventsRef writes through. A var
// so tests can wrap it in a store that fails the way a lost gc race does —
// the race itself is a window of nanoseconds between two syscalls inside
// go-billy, far too narrow to hit on demand from outside.
var writeStorer = func(repo *gogit.Repository) storer.EncodedObjectStorer {
	return repo.Storer
}

// objectWriteAttempts bounds how many times a single loose-object write is
// retried after losing a race with a concurrent gc. The race window is the
// few microseconds between go-billy's MkdirAll and its rename(2), so losing
// it five times running means something other than a gc is wrong — fail and
// surface the error rather than loop.
const objectWriteAttempts = 5

// storeObject writes enc to st, retrying writes that lost a race with a
// concurrent `git gc`.
//
// go-git stores a loose object by streaming it to .git/objects/pack/tmp_obj_*
// and renaming it to .git/objects/<xx>/<rest>; go-billy MkdirAlls the
// destination's parent directory immediately before the rename. `git gc`
// rmdirs empty .git/objects/<xx> directories as it packs, so a gc landing
// between those two syscalls makes the rename fail ENOENT — reported from a
// GitHub Actions deploy job as:
//
//	error: build tree: rename .git/objects/pack/tmp_obj_1741553381
//	  .git/objects/9f/9edc86...: no such file or directory
//
// Retrying re-runs MkdirAll and writes a fresh temp file, so it recovers from
// a pruned destination directory and a pruned source temp file alike. Git
// objects are content-addressed, so a retried write is idempotent — nothing
// is duplicated if the first attempt partially succeeded.
//
// clarity already sets gc.auto=0 on its own git invocations (see noAutoGC),
// which removes the most likely racer: the detached `gc --auto` that our own
// `git fetch` spawns one statement before the tree build. That is prevention
// for the gc we control; this retry covers the gc we don't — another job,
// another tool in the same workflow, or a repo with gc.auto configured.
func storeObject(st storer.EncodedObjectStorer, enc plumbing.EncodedObject) (plumbing.Hash, error) {
	var err error
	for range objectWriteAttempts {
		var h plumbing.Hash
		h, err = st.SetEncodedObject(enc)
		if err == nil {
			return h, nil
		}
		if !isTransientObjectWriteError(err) {
			return plumbing.ZeroHash, err
		}
	}
	// Budget exhausted — return the last race error rather than a synthetic
	// one, so the caller sees which path actually failed.
	return plumbing.ZeroHash, err
}

// isTransientObjectWriteError reports whether err is a loose-object write that
// lost a race with a concurrent gc, rather than a real write failure.
//
// Matching is on the typed *os.LinkError — a failed rename(2) whose cause is
// "does not exist" — not on the message. A missing file reported by any other
// operation is a genuine error and must not be retried into a timeout.
func isTransientObjectWriteError(err error) bool {
	var le *os.LinkError
	if !errors.As(err, &le) {
		return false
	}
	return le.Op == "rename" && errors.Is(le.Err, fs.ErrNotExist)
}
