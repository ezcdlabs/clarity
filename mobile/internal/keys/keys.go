// Package keys manages the SSH identity a client uses to reach git hosts.
//
// The app generates its own keypair and shows the user the public half to
// paste into GitHub, GitLab, or whatever they host on. That keeps clarity
// host-agnostic — there is no OAuth app to register per provider, and no
// provider API to depend on — and it means the secret never leaves the
// device.
package keys

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// fileName is the private key on disk. The public half is derived rather than
// stored: two files that can disagree is one more thing to get wrong.
const fileName = "id_ed25519"

// Identity is the keypair this client authenticates with.
type Identity struct {
	dir string
}

// Open prepares an identity stored under dir. Nothing is generated until
// Ensure is called, so opening is cheap and side-effect free.
func Open(dir string) *Identity { return &Identity{dir: dir} }

// Ensure returns the existing key, generating one on first use.
//
// ed25519 rather than RSA: every host clarity targets has accepted it for
// years, the keys are short enough to paste without wrapping, and generation
// is instant on a phone where RSA-4096 is not.
func (i *Identity) Ensure() (ssh.Signer, error) {
	signer, err := i.load()
	if err == nil {
		return signer, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	pem, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		return nil, fmt.Errorf("encode key: %w", err)
	}

	if err := os.MkdirAll(i.dir, 0o700); err != nil {
		return nil, fmt.Errorf("create key directory: %w", err)
	}
	// 0600, and written before it is readable: a private key is the one file
	// here that must never be world-readable even briefly.
	if err := os.WriteFile(i.path(), encodePEM(pem), 0o600); err != nil {
		return nil, fmt.Errorf("write key: %w", err)
	}
	return i.load()
}

// Fingerprint is the SHA256 form a host publishes, for showing beside a key
// without showing the key — and for comparing by eye against what the host
// says.
//
// It does not create an identity. A label is not a reason to generate a
// keypair, and a caller asking what the key is called before there is one
// deserves to be told so.
func (i *Identity) Fingerprint() (string, error) {
	signer, err := i.load()
	if err != nil {
		return "", err
	}
	return ssh.FingerprintSHA256(signer.PublicKey()), nil
}

// PublicKey returns the authorized_keys line to paste into a git host,
// generating the key if this is the first time.
func (i *Identity) PublicKey(comment string) (string, error) {
	signer, err := i.Ensure()
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	if comment = strings.TrimSpace(comment); comment != "" {
		line += " " + comment
	}
	return line, nil
}

// Exists reports whether a key has been generated, without creating one.
// Lets a client show "add this key to your host" only when there is one.
func (i *Identity) Exists() bool {
	_, err := os.Stat(i.path())
	return err == nil
}

func (i *Identity) path() string { return filepath.Join(i.dir, fileName) }

func (i *Identity) load() (ssh.Signer, error) {
	data, err := os.ReadFile(i.path())
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("read key: %w", err)
	}
	return signer, nil
}

// encodePEM renders the PEM block ssh.MarshalPrivateKey returns.
func encodePEM(block *pem.Block) []byte { return pem.EncodeToMemory(block) }
