// Package hostkeys verifies ssh host keys on a device that has no ssh client.
//
// go-git's default is to read ~/.ssh/known_hosts, which on Android and iOS does
// not exist — the fetch fails before it reaches the network with "unable to
// find any valid known_hosts file". The obvious shortcut is
// ssh.InsecureIgnoreHostKey, and it is the wrong one: a tool whose whole job is
// telling you the truth about your pipeline should not accept whatever answers
// on port 22.
//
// So the app keeps its own known_hosts and does what an ssh client does on
// first contact: stop, show the fingerprint, and go no further until somebody
// says yes. An earlier version of this trusted the first key silently, which
// is a prompt nobody can fail — this one refuses and hands the UI what a
// person needs in order to answer.
//
// Refusals are typed rather than worded, because the two cases are not the
// same: a host nobody has met is a question, and a host whose key has changed
// since last time is an alarm.
package hostkeys

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Store is a known_hosts file in the app's own data directory.
type Store struct {
	path string
	// pending holds what has been offered and not yet answered, keyed by the
	// fingerprint the user is looking at.
	pending sync.Map // fingerprint -> offer
}

// UnknownHostError is a host nobody has agreed to yet. It carries everything a
// prompt has to show, because a prompt that cannot show the fingerprint is one
// that can only be answered yes.
type UnknownHostError struct {
	Host        string
	Type        string // "ED25519", as a host publishes it
	Fingerprint string // "SHA256:…", as a host publishes it

	key ssh.PublicKey
}

// offer is one unanswered prompt: what to show, and what to write down.
//
// They are not the same string. knownhosts matches on "[host]:port" for a
// non-default port, while a person checking a fingerprint is checking it
// against a host. Writing down the displayed name produced an entry that never
// matched again.
type offer struct {
	addr string // the normalized form knownhosts matches on
	key  ssh.PublicKey
}

func (e *UnknownHostError) Error() string {
	return fmt.Sprintf("%s is not a host this device has trusted yet", e.Host)
}

// ChangedHostError is a host that has been agreed to before and is now
// presenting something else.
type ChangedHostError struct {
	Host        string
	Type        string
	Fingerprint string
}

func (e *ChangedHostError) Error() string {
	return fmt.Sprintf(
		"the host key for %s has changed — it is not the key this device "+
			"trusted before, so the connection was refused", e.Host)
}

// Open prepares a store at path. Nothing is read or created until a connection
// needs checking.
func Open(path string) *Store { return &Store{path: path} }

// Callback returns the host key check to hand to go-git.
func (s *Store) Callback() ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if err := s.ensureFile(); err != nil {
			return err
		}

		// Re-read on every connection rather than caching: a fetch is seconds
		// of network work, the file holds a handful of lines, and a cache here
		// would be a second copy of the truth.
		verify, err := knownhosts.New(s.path)
		if err != nil {
			return fmt.Errorf("read known hosts: %w", err)
		}

		err = verify(hostname, remote, key)
		if err == nil {
			return nil
		}

		var mismatch *knownhosts.KeyError
		if !errors.As(err, &mismatch) {
			return err
		}

		name := knownhosts.Normalize(hostname)
		if len(mismatch.Want) == 0 {
			// Never seen. Remember the key alongside the question, so saying
			// yes accepts the thing that was actually shown rather than
			// whatever answers next.
			s.pending.Store(fingerprint(key), offer{addr: name, key: key})
			return &UnknownHostError{
				Host:        hostFrom(name),
				Type:        keyType(key),
				Fingerprint: fingerprint(key),
			}
		}
		return &ChangedHostError{
			Host:        hostFrom(name),
			Type:        keyType(key),
			Fingerprint: fingerprint(key),
		}
	}
}

// Trust records a host key the user has agreed to.
//
// It takes the fingerprint rather than the key so that what gets written down
// is the thing the user was shown. Answering yes to one fingerprint must not
// accept a different key that happened to arrive in the same moment.
func (s *Store) Trust(host, fingerprint string) error {
	v, ok := s.pending.Load(fingerprint)
	if !ok {
		return fmt.Errorf("no host key with fingerprint %s was offered", fingerprint)
	}
	pending := v.(offer)
	if hostFrom(pending.addr) != host {
		return fmt.Errorf("that fingerprint was offered by %s, not %s", hostFrom(pending.addr), host)
	}
	if err := s.ensureFile(); err != nil {
		return err
	}
	// The address rather than the host: what is written down has to be the
	// thing knownhosts will match on next time.
	return s.remember(pending.addr, pending.key)
}

// keyType is the name a host publishes its key under — "ED25519" rather than
// ssh-ed25519, which is the wire name and not what anyone is comparing against.
func keyType(key ssh.PublicKey) string {
	return strings.ToUpper(strings.TrimPrefix(key.Type(), "ssh-"))
}

func fingerprint(key ssh.PublicKey) string { return ssh.FingerprintSHA256(key) }

// hostFrom drops the port knownhosts.Normalize adds, because a person checking
// a fingerprint is checking it against a host and not against a socket.
//
// Only the bracketed form needs handling: Normalize returns a bare host on the
// default port and "[host]:port" otherwise, so there is no third shape to
// unpick. A branch for one was here and was unreachable.
func hostFrom(normalized string) string {
	s := strings.TrimPrefix(normalized, "[")
	if i := strings.Index(s, "]:"); i >= 0 {
		return s[:i]
	}
	return s
}

func (s *Store) ensureFile() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create known hosts: %w", err)
	}
	return f.Close()
}

func (s *Store) remember(hostname string, key ssh.PublicKey) error {
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("record host key: %w", err)
	}
	defer f.Close()

	line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
	if _, err := fmt.Fprintln(f, line); err != nil {
		return fmt.Errorf("record host key: %w", err)
	}
	return nil
}
