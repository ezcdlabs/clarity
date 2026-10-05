package keys_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/mobile/internal/keys"
	"golang.org/x/crypto/ssh"
)

// TestEnsure_GeneratesOnceAndReuses verifies the identity is stable. A key
// that regenerated would silently stop matching what the user pasted into
// their host, and the failure would look like a permissions problem.
func TestEnsure_GeneratesOnceAndReuses(t *testing.T) {
	dir := t.TempDir()
	id := keys.Open(dir)

	if id.Exists() {
		t.Fatal("a fresh directory should hold no key")
	}
	first, err := id.Ensure()
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !id.Exists() {
		t.Error("Exists should report the key it just generated")
	}

	second, err := id.Ensure()
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if string(ssh.MarshalAuthorizedKey(first.PublicKey())) !=
		string(ssh.MarshalAuthorizedKey(second.PublicKey())) {
		t.Error("the key changed between calls; whatever the user pasted into " +
			"their host would stop working")
	}

	// And it survives a restart, which is the case that actually matters.
	reopened, err := keys.Open(dir).Ensure()
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if string(ssh.MarshalAuthorizedKey(reopened.PublicKey())) !=
		string(ssh.MarshalAuthorizedKey(first.PublicKey())) {
		t.Error("the key did not survive reopening the store")
	}
}

// TestPublicKey_IsPasteable covers the UX this exists for: the user copies
// one line into a git host.
func TestPublicKey_IsPasteable(t *testing.T) {
	line, err := keys.Open(t.TempDir()).PublicKey("clarity on my phone")
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	if !strings.HasPrefix(line, "ssh-ed25519 ") {
		t.Errorf("not an authorized_keys line: %q", line)
	}
	if strings.Contains(line, "\n") {
		t.Error("the line must paste as one line, with no trailing newline")
	}
	if !strings.HasSuffix(line, "clarity on my phone") {
		t.Errorf("the comment should identify the device: %q", line)
	}
	// It has to actually parse as a public key, not merely look like one.
	if _, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line)); err != nil {
		t.Errorf("a host would reject this line: %v", err)
	}
}

// TestEnsure_PrivateKeyIsNotReadableByOthers pins the file mode. This is the
// one secret on the device, and a phone's app sandbox is not a reason to be
// careless with it — the same code runs in tests, on desktops, and anywhere
// else the package is reused.
func TestEnsure_PrivateKeyIsNotReadableByOthers(t *testing.T) {
	dir := t.TempDir()
	if _, err := keys.Open(dir).Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "id_ed25519"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("private key mode is %04o; group and other must have nothing", perm)
	}
}

// TestFingerprint is what the UI shows beside a key without showing the key.
// The same SHA256 form a host publishes, so the two can be compared by eye.
func TestFingerprint(t *testing.T) {
	id := keys.Open(filepath.Join(t.TempDir(), "identity"))
	if _, err := id.Ensure(); err != nil {
		t.Fatal(err)
	}

	got, err := id.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "SHA256:") {
		t.Errorf("fingerprint = %q, want the SHA256 form", got)
	}
	// Stable: the same key has to produce the same string every time, or it is
	// not something anyone can compare against what their host shows.
	again, err := id.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if got != again {
		t.Errorf("two reads gave %q then %q", got, again)
	}
}

// TestFingerprintDoesNotCreateAKey keeps a label from being a side effect.
func TestFingerprintDoesNotCreateAKey(t *testing.T) {
	id := keys.Open(filepath.Join(t.TempDir(), "identity"))
	if _, err := id.Fingerprint(); err == nil {
		t.Error("asking for a fingerprint generated an identity")
	}
	if id.Exists() {
		t.Error("a key was written just by asking what it is called")
	}
}
