package clarityrefs

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
	"testing"
)

// TestIsTransientObjectWriteError documents the loose-object write race
// reported from GitHub Actions:
//
//	error: build tree: rename .git/objects/pack/tmp_obj_1741553381
//	  .git/objects/9f/9edc86...: no such file or directory
//
// go-git writes a loose object by streaming it to .git/objects/pack/tmp_obj_*
// and renaming it into .git/objects/<xx>/<rest>. go-billy's Rename MkdirAlls
// the destination's parent first — but a concurrent `git gc` rmdirs empty
// .git/objects/<xx> directories, and if it lands in the window between the
// MkdirAll and the rename(2) the rename fails ENOENT. The same applies if gc
// prunes the tmp_obj_* source. Both are transient: the retry re-creates the
// directory and writes a fresh temp file.
//
// Matching is on the typed *os.LinkError rather than the message so a genuine
// "no such file or directory" from an unrelated op can't be swallowed.
func TestIsTransientObjectWriteError(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name: "gc rmdir'd the destination object dir",
			err: &os.LinkError{
				Op:  "rename",
				Old: ".git/objects/pack/tmp_obj_1741553381",
				New: ".git/objects/9f/9edc8673b331befd2adda3eadb62effde0fbe9",
				Err: syscall.ENOENT,
			},
			expected: true,
		},
		{
			name: "wrapped by buildTree's error context",
			err: fmt.Errorf("build tree: %w", &os.LinkError{
				Op:  "rename",
				Old: ".git/objects/pack/tmp_obj_2499351975",
				New: ".git/objects/48/376f1f3c9c349bf33bb095c37cb854e4d60248",
				Err: syscall.ENOENT,
			}),
			expected: true,
		},
		{
			// A rename that failed for a non-transient reason must surface.
			name: "rename across devices is not retryable",
			err: &os.LinkError{
				Op: "rename", Old: "a", New: "b", Err: syscall.EXDEV,
			},
			expected: false,
		},
		{
			name: "permission denied is not retryable",
			err: &os.LinkError{
				Op: "rename", Old: "a", New: "b", Err: syscall.EACCES,
			},
			expected: false,
		},
		{
			// ENOENT from an unrelated operation is a real missing file.
			name: "open of a missing file is not a write race",
			err: &os.PathError{
				Op: "open", Path: ".git/config", Err: syscall.ENOENT,
			},
			expected: false,
		},
		{
			name:     "plain fs.ErrNotExist is not a write race",
			err:      fs.ErrNotExist,
			expected: false,
		},
		{
			name:     "unrelated error",
			err:      fmt.Errorf("connection refused"),
			expected: false,
		},
		{
			name:     "nil",
			err:      nil,
			expected: false,
		},
	}

	for _, c := range cases {
		if got := isTransientObjectWriteError(c.err); got != c.expected {
			t.Errorf("%s: isTransientObjectWriteError(%v) = %v, want %v",
				c.name, c.err, got, c.expected)
		}
	}
}

// TestIsFastForwardRejected documents all known retryable push rejection
// messages from go-git and the git CLI. When a new message is discovered at
// runtime, add a failing case here first, then extend isFastForwardRejected.
func TestIsFastForwardRejected(t *testing.T) {
	cases := []struct {
		msg      string
		expected bool
	}{
		// known retryable messages
		{"non-fast-forward update", true},
		{"failed to update ref", true},
		{"reference already exists", true},
		{"incorrect old value provided", true},
		// wrapping should still match
		{"push events ref: command error on refs/clarity/events: reference already exists", true},
		{"push events ref: command error on refs/clarity/events: incorrect old value provided", true},
		// git CLI rejection messages (git push --porcelain output)
		{"exit status 1\n ! [rejected]        refs/clarity/events -> refs/clarity/events (fetch first)", true},
		{"exit status 1\n ! [rejected]        refs/clarity/events -> refs/clarity/events (non-fast-forward)", true},
		// non-retryable errors must not be swallowed
		{"authentication required", false},
		{"connection refused", false},
		{"repository not found", false},
	}

	for _, c := range cases {
		got := isFastForwardRejected(fmt.Errorf("%s", c.msg))
		if got != c.expected {
			t.Errorf("isFastForwardRejected(%q) = %v, want %v", c.msg, got, c.expected)
		}
	}
}

// TestIsPackfileNotFound documents the transient go-git read error that
// occurs when a concurrent git gc/repack deletes a packfile out from under an
// in-progress read. The fetched objects are still reachable on the remote, so
// the read is retried after re-fetching into a freshly-packed object store.
func TestIsPackfileNotFound(t *testing.T) {
	cases := []struct {
		msg      string
		expected bool
	}{
		// go-git's dotgit.ErrPackfileNotFound, bare and wrapped
		{"packfile not found", true},
		{"object not found: packfile not found", true},
		{"read events ref: packfile not found", true},
		// unrelated errors must not be treated as a transient pack race
		{"object not found", false},
		{"reference not found", false},
		{"non-fast-forward update", false},
		{"connection refused", false},
	}
	for _, c := range cases {
		got := isPackfileNotFound(fmt.Errorf("%s", c.msg))
		if got != c.expected {
			t.Errorf("isPackfileNotFound(%q) = %v, want %v", c.msg, got, c.expected)
		}
	}
}

// TestIsBrokenObjectError documents the remote fsck error messages that
// indicate the local events ref contains invalid git objects. When such an
// error is received, the local ref must be deleted and the operation retried
// from scratch.
func TestIsBrokenObjectError(t *testing.T) {
	cases := []struct {
		msg      string
		expected bool
	}{
		// GitHub receive.fsckObjects rejection for flat-tree objects
		{"remote: error: object abc: fullPathname: contains full pathnames\nremote: fatal: fsck error in packed object", true},
		{"exit status 1\nremote: fatal: fsck error in packed object\nerror: remote unpack failed: index-pack failed", true},
		// Not broken-object errors
		{"[rejected]", false},
		{"non-fast-forward update", false},
		{"connection refused", false},
		{"authentication required", false},
	}
	for _, c := range cases {
		got := isBrokenObjectError(fmt.Errorf("%s", c.msg))
		if got != c.expected {
			t.Errorf("isBrokenObjectError(%q) = %v, want %v", c.msg, got, c.expected)
		}
	}
}
