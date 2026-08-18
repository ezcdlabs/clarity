package clarityrefs

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
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
		// Observed from GitHub Actions: two jobs pushing at once, and the
		// loser's compare-and-swap is refused by the *remote*, which reports
		// it as a lock failure naming both ref values. Note "[remote
		// rejected]" — the plain "[rejected]" match never sees this line.
		{"exit status 1\nTo https://github.com/Rafiki-Works/app-v2\n" +
			" ! [remote rejected] refs/clarity/events -> refs/clarity/events " +
			"(cannot lock ref 'refs/clarity/events': is at a9f4824bf9165cde14f8ffd67bfd1c7129321039 " +
			"but expected 1668810d1e2f4448d940486d2718decdeeece9b4)\n" +
			"error: failed to push some refs to 'https://github.com/Rafiki-Works/app-v2'", true},
		// A remote rejection that is NOT a lost race must keep routing to its
		// own recovery: the fsck path deletes the local ref before retrying,
		// which a plain fast-forward retry would not do.
		{"exit status 1\nremote: fatal: fsck error in packed object\n" +
			" ! [remote rejected] refs/clarity/events -> refs/clarity/events (unpacker error)", false},
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

// TestIsMissingLocalObject documents what `git push` reports when a gc has
// pruned objects this write created but had not yet made reachable. Each case
// is a message observed from real git, one per object type the write
// produces: blobs, the subtrees above them, and the commit itself.
//
// The wording differs per type and none of it mentions gc, so the match is on
// the shape git uses for an unreadable object — a diagnostic naming a raw
// object id. When a new variant turns up at runtime, add the case here first.
func TestIsMissingLocalObject(t *testing.T) {
	const push = "push events ref: exit status 1\n%s\n" +
		"error: remote unpack failed: eof before pack header was fully read\n" +
		" ! [rejected] refs/clarity/events -> refs/clarity/events (unpacker error)"

	cases := []struct {
		name     string
		msg      string
		expected bool
	}{
		{
			name:     "blob pruned",
			msg:      fmt.Sprintf(push, "fatal: unable to read 1e0cc372331e3a39d4fc68004e6aac0bded258ad"),
			expected: true,
		},
		{
			name:     "tree pruned",
			msg:      fmt.Sprintf(push, "fatal: bad tree object 5a8315cd2bc01c22ef5f057ff3c9fff67103da66"),
			expected: true,
		},
		{
			name:     "commit pruned",
			msg:      fmt.Sprintf(push, "fatal: bad object 648fb6396609169bb455f6939d8796366f570cc2"),
			expected: true,
		},
		{
			name:     "sha256 object ids",
			msg:      "fatal: bad object " + strings.Repeat("a", 64),
			expected: true,
		},
		// Rejections handled by their own recovery must not be captured here.
		{
			name:     "fast-forward rejection",
			msg:      "exit status 1\n ! [rejected] refs/clarity/events -> refs/clarity/events (non-fast-forward)",
			expected: false,
		},
		{
			name:     "remote fsck rejection",
			msg:      "remote: error: object abc: fullPathname: contains full pathnames\nremote: fatal: fsck error in packed object",
			expected: false,
		},
		// "unable to read" appears in unrelated git diagnostics; without an
		// object id it is not this failure.
		{
			name:     "unable to read without an object id",
			msg:      "fatal: unable to read current working directory: Permission denied",
			expected: false,
		},
		{
			name:     "authentication failure",
			msg:      "fatal: could not read Username for 'https://github.com': No such device or address",
			expected: false,
		},
		{
			name:     "connection refused",
			msg:      "connection refused",
			expected: false,
		},
	}

	for _, c := range cases {
		if got := isMissingLocalObject(fmt.Errorf("%s", c.msg)); got != c.expected {
			t.Errorf("%s: isMissingLocalObject(%q) = %v, want %v",
				c.name, c.msg, got, c.expected)
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
