package core

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ezcdlabs/clarity/mobile/internal/hostkeys"
	v1 "github.com/ezcdlabs/clarity/proto/gen/go/clarityv1"
)

// TestOutcome covers the sorting a UI depends on, without a network.
//
// The earlier version of this reached for 127.0.0.1 and asserted only that the
// result was "not OK", which made it a test of whatever happened to be
// listening on the machine running it. Mutation testing found that out by
// deleting a message nothing checked.
func TestOutcome(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		want    v1.Outcome
		hostKey bool
		output  bool
	}{
		{
			name: "nothing went wrong",
			err:  nil,
			want: v1.Outcome_OUTCOME_OK,
		},
		{
			name:    "a host nobody has agreed to",
			err:     &hostkeys.UnknownHostError{Host: "git.acme.dev", Type: "ED25519", Fingerprint: "SHA256:abc"},
			want:    v1.Outcome_OUTCOME_HOST_KEY_UNKNOWN,
			hostKey: true,
		},
		{
			name:    "a host presenting something else",
			err:     &hostkeys.ChangedHostError{Host: "git.acme.dev", Type: "ED25519", Fingerprint: "SHA256:def"},
			want:    v1.Outcome_OUTCOME_HOST_KEY_CHANGED,
			hostKey: true,
		},
		{
			// The phrase ssh actually produces. The list of attempted methods
			// varies between versions; the phrase does not.
			name:   "the key was refused",
			err:    errors.New("ssh: handshake failed: ssh: unable to authenticate, attempted methods [none publickey]"),
			want:   v1.Outcome_OUTCOME_AUTH_DENIED,
			output: true,
		},
		{
			name:   "what git says when a key is not authorised",
			err:    errors.New("Permission denied (publickey)."),
			want:   v1.Outcome_OUTCOME_AUTH_DENIED,
			output: true,
		},
		{
			name: "anything else",
			err:  errors.New("dial tcp: network is unreachable"),
			want: v1.Outcome_OUTCOME_FAILED,
		},
		{
			// Wrapped, which is how it will actually arrive — through
			// gitsource and go-git before it reaches here.
			name:    "wrapped, and still recognised",
			err:     fmt.Errorf("fetch refs/clarity/events: %w", &hostkeys.UnknownHostError{Host: "h", Type: "ED25519", Fingerprint: "SHA256:x"}),
			want:    v1.Outcome_OUTCOME_HOST_KEY_UNKNOWN,
			hostKey: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := outcome(tc.err)
			if got.Outcome != tc.want {
				t.Errorf("outcome = %v, want %v", got.Outcome, tc.want)
			}
			if tc.want != v1.Outcome_OUTCOME_OK && got.Message == "" {
				t.Error("no message for the user to read")
			}
			if tc.hostKey && got.HostKey == nil {
				t.Error("no host key, so the dialog has no fingerprint to show")
			}
			if !tc.hostKey && got.HostKey != nil {
				t.Errorf("a host key appeared where there is none: %+v", got.HostKey)
			}
			if tc.output && got.GitOutput == "" {
				t.Error("no git output behind the disclosure")
			}
		})
	}
}
