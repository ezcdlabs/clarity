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

// TestRegistry_SurvivesRestart pins the only thing persistence owes: what the
// user gave us comes back. What a repository is *called* is not stored at all
// — it is read off the URL by mobile/internal/remote every time, so a change
// to that rule reaches entries that were added before it.
func TestRegistry_SurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	if _, err := registry.Open(dir).Add("https://github.com/a/b.git", "main"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	entries, err := registry.Open(dir).List()
	if err != nil {
		t.Fatalf("List after reopen: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("the repository did not survive reopening: %+v", entries)
	}
	if entries[0].URL != "https://github.com/a/b.git" || entries[0].Branch != "main" {
		t.Errorf("came back as %+v", entries[0])
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

// TestAlias_IsLocalAndClearable covers renaming. The alias lives on this
// device only — nothing is written to the host — and clearing it has to be
// possible, because "go back to the real name" is the other half of renaming.
func TestAlias_IsLocalAndClearable(t *testing.T) {
	r := registry.Open(t.TempDir())
	e, err := r.Add("git@github.com:acme/web-platform.git", "main")
	if err != nil {
		t.Fatal(err)
	}

	if err := r.SetAlias(e.ID, "  The Platform  "); err != nil {
		t.Fatal(err)
	}
	got, err := r.Get(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Trimmed, because a name with invisible edges is a name that sorts and
	// compares wrongly for a reason nobody can see.
	if got.Alias != "The Platform" {
		t.Errorf("alias = %q, want %q", got.Alias, "The Platform")
	}

	if err := r.SetAlias(e.ID, "   "); err != nil {
		t.Fatal(err)
	}
	got, _ = r.Get(e.ID)
	if got.Alias != "" {
		t.Errorf("a blank alias left %q behind instead of clearing", got.Alias)
	}
}

// TestStatus_IsRememberedAcrossRestart is what lets the switcher say how every
// repository is doing without opening each one. It is cached rather than
// derived, so it has to survive the process that cached it.
func TestStatus_IsRememberedAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	r := registry.Open(dir)
	e, err := r.Add("git@github.com:acme/monorepo.git", "main")
	if err != nil {
		t.Fatal(err)
	}

	want := registry.Status{
		CI:     "passed",
		Deploy: "failed",
		Flows: []registry.FlowStatus{
			{Name: "web", Deploy: "passed"},
			{Name: "ios", Deploy: "failed"},
		},
	}
	if err := r.SetStatus(e.ID, want); err != nil {
		t.Fatal(err)
	}

	got, err := registry.Open(dir).Get(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status == nil {
		t.Fatal("the cached status did not survive a restart")
	}
	if got.Status.CI != "passed" || got.Status.Deploy != "failed" {
		t.Errorf("status = %+v, want ci passed and deploy failed", got.Status)
	}
	if len(got.Status.Flows) != 2 || got.Status.Flows[1].Name != "ios" {
		t.Errorf("flows = %+v, want web then ios in order", got.Status.Flows)
	}
}

// TestStatus_IsAbsentUntilSomethingHasBeenRead pins the distinction a boolean
// could not carry: a repository nobody has fetched yet has no verdict, which
// is not the same as a verdict of "nothing reported".
func TestStatus_IsAbsentUntilSomethingHasBeenRead(t *testing.T) {
	r := registry.Open(t.TempDir())
	e, err := r.Add("git@github.com:acme/fresh.git", "main")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := r.Get(e.ID)
	if got.Status != nil {
		t.Errorf("a never-fetched repository already has a status: %+v", got.Status)
	}
}

// TestSetAlias_RejectsAnUnknownRepository keeps a rename from quietly creating
// an entry out of a stale id.
func TestSetAlias_RejectsAnUnknownRepository(t *testing.T) {
	r := registry.Open(t.TempDir())
	if err := r.SetAlias("nope", "x"); err == nil {
		t.Error("renaming a repository that is not tracked succeeded")
	}
}

// TestSetBranch_KeepsEverythingElse covers "Change branch…". The repository is
// the same repository — same id, same local store, same rename — so changing
// which branch it watches must not look like removing it and adding it back.
func TestSetBranch_KeepsEverythingElse(t *testing.T) {
	r := registry.Open(t.TempDir())
	e, err := r.Add("git@github.com:acme/web-platform.git", "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.SetAlias(e.ID, "The Platform"); err != nil {
		t.Fatal(err)
	}

	if err := r.SetBranch(e.ID, "  release  "); err != nil {
		t.Fatal(err)
	}
	got, err := r.Get(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Branch != "release" {
		t.Errorf("branch = %q", got.Branch)
	}
	if got.Alias != "The Platform" {
		t.Errorf("the rename was lost: %q", got.Alias)
	}
	if got.ID != e.ID {
		t.Errorf("the id changed, so the local store is orphaned")
	}
}

// TestSetBranch_BlankMeansMain matches what adding does, so the two ways of
// setting a branch agree about what an empty box means.
func TestSetBranch_BlankMeansMain(t *testing.T) {
	r := registry.Open(t.TempDir())
	e, _ := r.Add("git@github.com:acme/thing.git", "release")
	if err := r.SetBranch(e.ID, "   "); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Get(e.ID); got.Branch != "main" {
		t.Errorf("branch = %q, want main", got.Branch)
	}
}
