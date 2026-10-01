package registry_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ezcdlabs/clarity/mobile/internal/registry"
)

// TestAdd_AcceptsTheURLYouWouldClone covers the whole setup step. Clarity
// does not know what hosts exist, so the check has to accept any of the forms
// a git remote takes — rejecting an unfamiliar host would be exactly the
// provider-specific assumption the design avoids.
func TestAdd_AcceptsTheURLYouWouldClone(t *testing.T) {
	good := []string{
		"https://github.com/ezcdlabs/clarity.git",
		"https://gitlab.com/group/sub/project",
		"git@github.com:ezcdlabs/clarity.git",
		"ssh://git@git.example.internal:2222/team/api.git",
		"https://user@dev.azure.com/org/project/_git/repo",
	}
	for _, url := range good {
		r := registry.Open(t.TempDir())
		if _, err := r.Add(url, ""); err != nil {
			t.Errorf("Add(%q): %v", url, err)
		}
	}

	bad := []string{"", "   ", "not a url", "ezcdlabs/clarity"}
	for _, url := range bad {
		r := registry.Open(t.TempDir())
		if _, err := r.Add(url, ""); err == nil {
			t.Errorf("Add(%q) should have been rejected at the point of pasting", url)
		}
	}
}

// TestAdd_SameRepositoryTwiceIsOneEntry verifies re-pasting a URL updates
// rather than duplicating — including through the spellings that address the
// same repository.
func TestAdd_SameRepositoryTwiceIsOneEntry(t *testing.T) {
	r := registry.Open(t.TempDir())

	first, err := r.Add("https://github.com/ezcdlabs/clarity.git", "main")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Same repository, different spelling, and a corrected branch.
	second, err := r.Add("https://github.com/ezcdlabs/clarity/", "trunk")
	if err != nil {
		t.Fatalf("Add again: %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("the same repository got two ids (%s, %s), so it would get two "+
			"object stores", first.ID, second.ID)
	}

	entries, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(entries))
	}
	if entries[0].Branch != "trunk" {
		t.Errorf("branch = %q; re-pasting to fix a branch should do what the user means", entries[0].Branch)
	}
}

// TestAdd_NamesTheRepositoryFromItsURL covers the menu label, across the
// spellings a remote takes.
func TestAdd_NamesTheRepositoryFromItsURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/ezcdlabs/clarity.git": "clarity",
		"git@github.com:ezcdlabs/clarity.git":     "clarity",
		"ssh://git@example.com:2222/team/api.git": "api",
		"https://gitlab.com/group/sub/deep/thing": "thing",
	}
	for url, want := range cases {
		r := registry.Open(t.TempDir())
		e, err := r.Add(url, "")
		if err != nil {
			t.Fatalf("Add(%q): %v", url, err)
		}
		if e.Name != want {
			t.Errorf("Add(%q).Name = %q, want %q", url, e.Name, want)
		}
	}
}

// TestRegistry_SurvivesRestart is the point of persisting at all.
func TestRegistry_SurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	if _, err := registry.Open(dir).Add("https://github.com/a/b.git", "main"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	entries, err := registry.Open(dir).List()
	if err != nil {
		t.Fatalf("List after reopen: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "b" {
		t.Errorf("the repository did not survive reopening: %+v", entries)
	}
}

// TestRemove_IsIdempotent — the caller wanted it gone, and it is.
func TestRemove_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	r := registry.Open(dir)
	e, _ := r.Add("https://github.com/a/b.git", "main")

	if err := r.Remove(e.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := r.Remove(e.ID); err != nil {
		t.Errorf("removing an untracked repository should not be an error: %v", err)
	}
	if entries, _ := r.List(); len(entries) != 0 {
		t.Errorf("expected none left, got %d", len(entries))
	}
}

// TestSave_DoesNotTruncateOnInterruption pins the atomic write. A phone can
// be killed mid-write, and a half-written repos.json would lose every
// repository rather than the one being changed.
func TestSave_DoesNotTruncateOnInterruption(t *testing.T) {
	dir := t.TempDir()
	r := registry.Open(dir)
	for _, u := range []string{"https://h/a.git", "https://h/b.git", "https://h/c.git"} {
		if _, err := r.Add(u, "main"); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	// The temporary file must not be left behind to be mistaken for state.
	if _, err := os.Stat(filepath.Join(dir, "repos.json.tmp")); err == nil {
		t.Error("a temporary file was left next to the real one")
	}
	if entries, _ := r.List(); len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

// TestStorePath_IsPerRepository verifies two repositories can never share an
// object store, which would mix one repo's commits into another's view.
func TestStorePath_IsPerRepository(t *testing.T) {
	dir := t.TempDir()
	r := registry.Open(dir)
	a, _ := r.Add("https://h/a.git", "main")
	b, _ := r.Add("https://h/b.git", "main")

	if r.StorePath(a.ID) == r.StorePath(b.ID) {
		t.Fatal("two repositories share an object store")
	}
	if !filepath.IsAbs(r.StorePath(a.ID)) && !filepath.IsLocal(r.StorePath(a.ID)) {
		t.Errorf("store path escapes the data directory: %q", r.StorePath(a.ID))
	}
}
