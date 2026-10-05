package hostkeys

import (
	"crypto/ed25519"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestFirstSightOfAHostIsRefusedUntilItIsTrusted(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "known_hosts"))

	err := store.Callback()("github.com:22", addr(t), key(t, 1))
	if err == nil {
		t.Fatal("an unknown host was accepted without anybody being asked")
	}

	// The refusal has to carry what a person needs in order to answer. A
	// prompt that cannot show the fingerprint is a prompt that can only be
	// answered yes.
	var unknown *UnknownHostError
	if !errors.As(err, &unknown) {
		t.Fatalf("got %T, want an UnknownHostError the UI can act on", err)
	}
	if unknown.Host != "github.com" {
		t.Errorf("host = %q", unknown.Host)
	}
	if !strings.HasPrefix(unknown.Fingerprint, "SHA256:") {
		t.Errorf("fingerprint = %q, want the SHA256 form a host publishes", unknown.Fingerprint)
	}
	if unknown.Type != "ED25519" {
		t.Errorf("type = %q, want the name a host publishes", unknown.Type)
	}

	// Trusting it is what the dialog's accept button does.
	if err := store.Trust(unknown.Host, unknown.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if err := store.Callback()("github.com:22", addr(t), key(t, 1)); err != nil {
		t.Fatalf("the trusted host was still refused: %v", err)
	}
}

// TestTrustOnlyAcceptsTheKeyItWasShown is what stops the prompt being
// theatre. Answering yes to one fingerprint must not quietly accept a
// different key that arrives in the same moment.
func TestTrustOnlyAcceptsTheKeyItWasShown(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "known_hosts"))

	err := store.Callback()("github.com:22", addr(t), key(t, 1))
	var unknown *UnknownHostError
	if !errors.As(err, &unknown) {
		t.Fatalf("got %T", err)
	}
	if err := store.Trust(unknown.Host, "SHA256:something-else-entirely"); err == nil {
		t.Fatal("trusting a fingerprint the host never presented succeeded")
	}
	if err := store.Callback()("github.com:22", addr(t), key(t, 1)); err == nil {
		t.Error("the host is trusted despite the mismatch being refused")
	}
}

func TestAChangedHostKeyIsRejected(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "known_hosts"))
	trust(t, store, "github.com:22", key(t, 1))

	err := store.Callback()("github.com:22", addr(t), key(t, 2))
	if err == nil {
		t.Fatal("a host presenting a different key was accepted")
	}
	// A changed key is a different thing from an unseen one, and the UI shows
	// a different dialog for it.
	var changed *ChangedHostError
	if !errors.As(err, &changed) {
		t.Fatalf("got %T, want a ChangedHostError", err)
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

	trust(t, store, "github.com:22", key(t, 1))
	// A second host is unknown, not a mismatch — trusting the first must not
	// make the second one's key look wrong, or right.
	var unknown *UnknownHostError
	if !errors.As(store.Callback()("gitlab.com:22", addr(t), key(t, 2)), &unknown) {
		t.Fatal("a second host did not read as unknown")
	}
	trust(t, store, "gitlab.com:22", key(t, 2))
	if err := store.Callback()("github.com:22", addr(t), key(t, 2)); err == nil {
		t.Error("the first host was accepted with the second host's key")
	}
}

func TestTheFileIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	store := Open(path)
	trust(t, store, "github.com:22", key(t, 1))

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

	var unknown *UnknownHostError
	if !errors.As(store.Callback()("github.com:22", addr(t), key(t, 1)), &unknown) {
		t.Fatal("a missing directory stopped the host even being read as unknown")
	}
	trust(t, store, "github.com:22", key(t, 1))
}

// trust answers the prompt the way the dialog's accept button does.
func trust(t *testing.T, store *Store, hostname string, k ssh.PublicKey) {
	t.Helper()
	var unknown *UnknownHostError
	if !errors.As(store.Callback()(hostname, addr(t), k), &unknown) {
		t.Fatalf("%s was not reported as unknown", hostname)
	}
	if err := store.Trust(unknown.Host, unknown.Fingerprint); err != nil {
		t.Fatal(err)
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

// TestTrustRefusesAKeyOfferedBySomebodyElse closes the other half of the
// prompt. The fingerprint on screen belongs to a host, and answering yes has
// to accept it for that host and no other — otherwise a second connection
// racing the dialog could have its key written down under the wrong name.
func TestTrustRefusesAKeyOfferedBySomebodyElse(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "known_hosts"))

	var unknown *UnknownHostError
	if !errors.As(store.Callback()("github.com:22", addr(t), key(t, 1)), &unknown) {
		t.Fatal("the host was not reported as unknown")
	}
	if err := store.Trust("gitlab.com", unknown.Fingerprint); err == nil {
		t.Fatal("a key offered by github was trusted for gitlab")
	}
	if err := store.Callback()("gitlab.com:22", addr(t), key(t, 1)); err == nil {
		t.Error("gitlab ended up trusted anyway")
	}
}

// TestThePromptNamesTheHostWithoutThePort covers a host on a non-default port,
// which is where knownhosts starts bracketing the name. A person checking a
// fingerprint is checking it against a host, not against a socket, so the
// prompt has to say "git.acme.dev" and not "[git.acme.dev]:2222".
func TestThePromptNamesTheHostWithoutThePort(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "known_hosts"))

	var unknown *UnknownHostError
	if !errors.As(store.Callback()("git.acme.dev:2222", addr(t), key(t, 1)), &unknown) {
		t.Fatal("the host was not reported as unknown")
	}
	if unknown.Host != "git.acme.dev" {
		t.Errorf("host = %q, want it without the port or the brackets", unknown.Host)
	}

	// And trusting it still matches the same host next time, brackets or not.
	if err := store.Trust(unknown.Host, unknown.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if err := store.Callback()("git.acme.dev:2222", addr(t), key(t, 1)); err != nil {
		t.Errorf("the trusted host on a non-default port was refused: %v", err)
	}
}
