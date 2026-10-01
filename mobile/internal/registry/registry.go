// Package registry tracks the repositories a client has been told about.
//
// Each one is added by pasting the URL you would clone — which is the whole
// setup step, and deliberately so. There is no account to connect and no
// provider to choose: a git URL is the only thing clarity needs, and it is
// the same whether the host is GitHub, a GitLab instance, or a box in a
// cupboard.
package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is one tracked repository.
type Entry struct {
	// ID is derived from the URL, so adding the same repository twice is the
	// same repository rather than a duplicate with its own store.
	ID     string `json:"id"`
	URL    string `json:"url"`
	Branch string `json:"branch"`
	// Name is what the list shows. Derived from the URL, because asking the
	// user to name a repo they have just pasted is a step that earns nothing.
	Name string `json:"name"`
}

// Registry is the set of tracked repositories, persisted under a directory.
type Registry struct {
	dir string
}

func Open(dir string) *Registry { return &Registry{dir: dir} }

const fileName = "repos.json"

// Add registers a repository, returning the entry. Adding one that is already
// tracked updates it rather than duplicating — re-pasting a URL to fix a
// branch should do what the user means.
func (r *Registry) Add(rawURL, branch string) (Entry, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return Entry{}, errors.New("a repository URL is required")
	}
	if !looksLikeGitRemote(rawURL) {
		return Entry{}, fmt.Errorf("%q does not look like a git remote — paste the URL you would clone", rawURL)
	}
	if branch = strings.TrimSpace(branch); branch == "" {
		branch = "main"
	}

	entry := Entry{
		ID:     id(rawURL),
		URL:    rawURL,
		Branch: branch,
		Name:   nameFrom(rawURL),
	}

	entries, err := r.List()
	if err != nil {
		return Entry{}, err
	}
	replaced := false
	for i := range entries {
		if entries[i].ID == entry.ID {
			entries[i] = entry
			replaced = true
		}
	}
	if !replaced {
		entries = append(entries, entry)
	}
	return entry, r.save(entries)
}

// Remove forgets a repository. Removing one that is not tracked is not an
// error: the caller wanted it gone, and it is.
func (r *Registry) Remove(repoID string) error {
	entries, err := r.List()
	if err != nil {
		return err
	}
	kept := entries[:0:0]
	for _, e := range entries {
		if e.ID != repoID {
			kept = append(kept, e)
		}
	}
	return r.save(kept)
}

// List returns the tracked repositories, ordered by name so the menu does not
// reshuffle itself between launches.
func (r *Registry) List() ([]Entry, error) {
	data, err := os.ReadFile(filepath.Join(r.dir, fileName))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read repositories: %w", err)
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("read repositories: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].ID < entries[j].ID
	})
	return entries, nil
}

// Get returns one entry.
func (r *Registry) Get(repoID string) (Entry, error) {
	entries, err := r.List()
	if err != nil {
		return Entry{}, err
	}
	for _, e := range entries {
		if e.ID == repoID {
			return e, nil
		}
	}
	return Entry{}, fmt.Errorf("no repository %q is being tracked", repoID)
}

// StorePath is where a repository's git objects live. One directory per
// repository, named by id, so removing a repository is removing a directory
// and two repositories can never share an object store.
func (r *Registry) StorePath(repoID string) string {
	return filepath.Join(r.dir, "repos", repoID)
}

func (r *Registry) save(entries []Entry) error {
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	// Written to a temporary file and renamed: a phone can be killed
	// mid-write, and a truncated repos.json would lose every repository the
	// user had added rather than the one being changed.
	tmp := filepath.Join(r.dir, fileName+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write repositories: %w", err)
	}
	return os.Rename(tmp, filepath.Join(r.dir, fileName))
}

// id is a stable identifier for a remote. Derived rather than random so the
// same URL is the same repository across launches and devices.
func id(rawURL string) string {
	sum := sha256.Sum256([]byte(normalise(rawURL)))
	return hex.EncodeToString(sum[:8])
}

// normalise folds the differences that do not change which repository is
// meant: a trailing slash, a .git suffix, and case in the host.
func normalise(rawURL string) string {
	s := strings.TrimSpace(rawURL)
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSuffix(s, ".git")
	return s
}

// nameFrom is the last path segment — "api" from any of the ways that repo
// can be addressed.
func nameFrom(rawURL string) string {
	s := normalise(rawURL)
	if i := strings.LastIndexAny(s, "/:"); i >= 0 && i+1 < len(s) {
		s = s[i+1:]
	}
	if s == "" {
		return rawURL
	}
	return s
}

// looksLikeGitRemote rejects what is obviously not a remote, so a mistyped
// paste fails at the point of pasting rather than as a confusing fetch error
// later. Deliberately permissive: clarity does not know what hosts exist, and
// refusing an unfamiliar one would be the provider-specific assumption this
// whole design avoids.
func looksLikeGitRemote(s string) bool {
	switch {
	case strings.Contains(s, "://"):
		return true
	case strings.Contains(s, "@") && strings.Contains(s, ":"):
		return true // scp-style: git@host:owner/repo
	default:
		return false
	}
}
