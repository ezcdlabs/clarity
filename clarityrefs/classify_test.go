package clarityrefs

import (
	"fmt"
	"testing"
)

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
