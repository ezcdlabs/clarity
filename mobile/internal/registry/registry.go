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

	"github.com/ezcdlabs/clarity/mobile/internal/remote"
)

// Entry is one tracked repository.
type Entry struct {
	// ID is derived from the URL, so adding the same repository twice is the
	// same repository rather than a duplicate with its own store.
	ID     string `json:"id"`
	URL    string `json:"url"`
	Branch string `json:"branch"`
	// Alias is a rename, and it lives on this device only — nothing is written
	// to the host, and another device showing the same repository shows the
	// name the URL gives it. Empty means no rename.
	Alias string `json:"alias,omitempty"`

	// Status is the last view's verdict, kept so a switcher can say how every
	// repository is doing without opening each one.
	//
	// A pointer, and absent until something has actually been read: "nobody
	// has fetched this yet" is a different thing from "nothing was reported",
	// and a value type could not tell them apart.
	Status *Status `json:"status,omitempty"`
}

// Status is what the last view said, in the vocabulary the events use.
type Status struct {
	CI     string       `json:"ci"`
	Deploy string       `json:"deploy"`
	Flows  []FlowStatus `json:"flows,omitempty"`
}

// FlowStatus is one deploy target's verdict, in the order the tabs show them.
type FlowStatus struct {
	Name   string `json:"name"`
	Deploy string `json:"deploy"`
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
	// Ordered by the name a list will show, which is derived rather than
	// stored — the registry keeps facts, and what a repository is called is
	// read off its URL every time so a change to that rule reaches entries
	// already added.
	sort.Slice(entries, func(i, j int) bool {
		a, b := remote.Parse(entries[i].URL), remote.Parse(entries[j].URL)
		if a.Name != b.Name {
			return a.Name < b.Name
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
// SetAlias renames a repository on this device, or clears the rename when the
// name is blank. Clearing has to be possible: "go back to the real name" is
// the other half of renaming.
func (r *Registry) SetAlias(id, alias string) error {
	return r.update(id, func(e *Entry) { e.Alias = strings.TrimSpace(alias) })
}

// SetBranch changes which branch a repository watches, keeping its id, its
// local store and its rename. Blank means main, as adding one does.
func (r *Registry) SetBranch(id, branch string) error {
	return r.update(id, func(e *Entry) {
		if branch = strings.TrimSpace(branch); branch == "" {
			branch = "main"
		}
		e.Branch = branch
		// What it last said is about the old branch, and saying it about the
		// new one would be a verdict nothing has reached yet.
		e.Status = nil
	})
}

// SetStatus records what the last view said about a repository.
func (r *Registry) SetStatus(id string, s Status) error {
	return r.update(id, func(e *Entry) { e.Status = &s })
}

// update applies a change to one entry and writes the registry back.
func (r *Registry) update(id string, apply func(*Entry)) error {
	entries, err := r.List()
	if err != nil {
		return err
	}
	for i := range entries {
		if entries[i].ID == id {
			apply(&entries[i])
			return r.save(entries)
		}
	}
	// Quietly creating an entry from a stale id would turn a rename into an
	// add, which is not what anybody asked for.
	return fmt.Errorf("no repository with id %q", id)
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
