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
// first contact: trust the key it is shown, write it down, and refuse anything
// different afterwards. That does not protect the very first fetch, which
// nothing but a key the user types in ever could. It does mean a host whose key
// changes under you is reported rather than silently accepted, which is the
// case that actually happens.
package hostkeys

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Store is a known_hosts file in the app's own data directory.
type Store struct {
	path string
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
		if errors.As(err, &mismatch) && len(mismatch.Want) == 0 {
			// Never seen this host. Trust it once, and write it down so that
			// every connection after this one is checked.
			return s.remember(hostname, key)
		}
		if errors.As(err, &mismatch) {
			return fmt.Errorf(
				"the host key for %s has changed — it is not the key this device "+
					"trusted before, so the connection was refused",
				knownhosts.Normalize(hostname),
			)
		}
		return err
	}
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
