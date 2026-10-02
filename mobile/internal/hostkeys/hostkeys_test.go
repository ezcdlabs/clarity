package hostkeys

import (
	"crypto/ed25519"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestFirstSightOfAHostIsAcceptedAndRemembered(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "known_hosts"))

	if err := store.Callback()("github.com:22", addr(t), key(t, 1)); err != nil {
		t.Fatalf("first connection rejected: %v", err)
	}

	// Remembering is the whole point: an accepted key that is not written down
	// means every connection is a first connection, and the check below can
	// never fire.
	if err := store.Callback()("github.com:22", addr(t), key(t, 1)); err != nil {
		t.Fatalf("second connection to the same host rejected: %v", err)
	}
}

func TestAChangedHostKeyIsRejected(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "known_hosts"))
	if err := store.Callback()("github.com:22", addr(t), key(t, 1)); err != nil {
		t.Fatal(err)
	}

	err := store.Callback()("github.com:22", addr(t), key(t, 2))
	if err == nil {
		t.Fatal("a host presenting a different key was accepted")
	}
	// The message has to be readable on a phone, where there is no ssh manual
	// and no known_hosts to go and look at.
	if !strings.Contains(err.Error(), "github.com") {
		t.Errorf("the message does not name the host: %v", err)
	}
	if !strings.Contains(err.Error(), "changed") {
		t.Errorf("the message does not say the key changed: %v", err)
	}
}

func TestEachHostIsRememberedSeparately(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "known_hosts"))

	if err := store.Callback()("github.com:22", addr(t), key(t, 1)); err != nil {
		t.Fatal(err)
	}
	// A second host is unknown, not a mismatch — trusting the first must not
	// make the second one's key look wrong, or right.
	if err := store.Callback()("gitlab.com:22", addr(t), key(t, 2)); err != nil {
		t.Fatalf("a second host was rejected: %v", err)
	}
	if err := store.Callback()("github.com:22", addr(t), key(t, 2)); err == nil {
		t.Error("the first host was accepted with the second host's key")
	}
}

func TestTheFileIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	store := Open(path)
	if err := store.Callback()("github.com:22", addr(t), key(t, 1)); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("known_hosts is %o, want 600", perm)
	}
}

func TestADirectoryThatDoesNotExistYetIsCreated(t *testing.T) {
	// The first fetch can happen before anything else has written to the data
	// directory, so the store cannot assume somebody else made it.
	path := filepath.Join(t.TempDir(), "nested", "deeper", "known_hosts")
	store := Open(path)

	if err := store.Callback()("github.com:22", addr(t), key(t, 1)); err != nil {
		t.Fatalf("rejected because the directory was missing: %v", err)
	}
}

// --- helpers -------------------------------------------------------------

// key returns a stable distinct public key per seed, so a test can say "the
// same key" and "a different key" and mean it.
func key(t *testing.T, seed byte) ssh.PublicKey {
	t.Helper()
	raw := make([]byte, ed25519.SeedSize)
	for i := range raw {
		raw[i] = seed
	}
	pub, err := ssh.NewPublicKey(ed25519.NewKeyFromSeed(raw).Public())
	if err != nil {
		t.Fatal(err)
	}
	return pub
}

func addr(t *testing.T) net.Addr {
	t.Helper()
	return &net.TCPAddr{IP: net.ParseIP("140.82.121.3"), Port: 22}
}
