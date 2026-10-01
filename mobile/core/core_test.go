package core_test

import (
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/gittest"
	mobilecore "github.com/ezcdlabs/clarity/mobile/core"
	v1 "github.com/ezcdlabs/clarity/proto/gen/go/clarityv1"
	"google.golang.org/protobuf/proto"
)

// TestClient_AddSyncView walks the whole chain an app performs: track a
// repository by URL, fetch it, and decode a view — with no working tree, no
// git binary, and no provider API.
func TestClient_AddSyncView(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("f.txt", "one")
	clone.CommitAll("feat: first")
	clone.Push("main")
	head := clone.LogBranch("main")[0].Hash
	// After the commit was authored, not before: a deploy that predates its
	// commit is a negative interval, which core excludes — so an event in the
	// past would have produced a view with no lead time and proved nothing.
	if err := clarityrefs.WriteEvent(clone.Path, "origin", head, clarityrefs.Event{
		Stage: "deploy", Status: "passed", Time: time.Now().Add(30 * time.Second),
	}); err != nil {
		t.Fatalf("seed event: %v", err)
	}

	c, err := mobilecore.New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// file:// rather than a bare path: a filesystem path is a valid git remote
	// but can never be reachable from a phone, so the registry rejects one at
	// the point of pasting. The test uses the form a user could actually type.
	id, err := c.AddRepo("file://"+remote.URL(), "main")
	if err != nil {
		t.Fatalf("AddRepo: %v", err)
	}
	if err := c.Sync(id, 50, 30); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	raw, err := c.View(id, 50)
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	var view v1.View
	if err := proto.Unmarshal(raw, &view); err != nil {
		t.Fatalf("the bytes crossing the bridge did not decode: %v", err)
	}
	if len(view.Flows) == 0 {
		t.Fatal("no flows in the view")
	}

	var deployed *v1.Commit
	for _, g := range view.Flows[0].Groups {
		if g.Kind == v1.GroupKind_GROUP_KIND_DEPLOYED && len(g.Commits) > 0 {
			deployed = g.Commits[0]
		}
	}
	if deployed == nil {
		t.Fatalf("the deployed commit did not survive the round trip: %+v", &view)
	}
	if deployed.Subject != "feat: first" {
		t.Errorf("subject = %q", deployed.Subject)
	}
	if !deployed.HasLeadTime || deployed.LeadTime == "" {
		t.Error("a shipped commit should carry its lead time across the bridge")
	}
}

// TestClient_ListReposRoundTrips covers the menu the app opens on.
func TestClient_ListReposRoundTrips(t *testing.T) {
	c, err := mobilecore.New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.AddRepo("https://github.com/ezcdlabs/clarity.git", "main"); err != nil {
		t.Fatalf("AddRepo: %v", err)
	}
	if _, err := c.AddRepo("git@github.com:other/api.git", ""); err != nil {
		t.Fatalf("AddRepo: %v", err)
	}

	raw, err := c.ListRepos()
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	var list v1.RepoList
	if err := proto.Unmarshal(raw, &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list.Repos) != 2 {
		t.Fatalf("expected 2 repositories, got %d", len(list.Repos))
	}
	for _, r := range list.Repos {
		if r.Id == "" || r.Name == "" || r.Url == "" || r.Branch == "" {
			t.Errorf("incomplete summary: %+v", r)
		}
	}
}

// TestClient_PublicKeyIsStable pins that the key the user pasted into their
// host keeps working across client instances — a new Client per app launch
// must not mean a new identity.
func TestClient_PublicKeyIsStable(t *testing.T) {
	dir := t.TempDir()
	c1, _ := mobilecore.New(dir)
	if c1.HasKey() {
		t.Error("a fresh data directory should hold no key")
	}
	first, err := c1.PublicKey("phone")
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}

	c2, _ := mobilecore.New(dir)
	second, err := c2.PublicKey("phone")
	if err != nil {
		t.Fatalf("PublicKey after restart: %v", err)
	}
	if first != second {
		t.Error("the identity changed between launches; whatever the user pasted " +
			"into their host would stop working")
	}
	if !c2.HasKey() {
		t.Error("HasKey should report the generated identity")
	}
}

// TestClient_UnknownRepoIsAnError covers an app holding a stale id — after a
// removal, or a restore onto a device that never had it.
func TestClient_UnknownRepoIsAnError(t *testing.T) {
	c, _ := mobilecore.New(t.TempDir())
	if err := c.Sync("nosuchrepo", 50, 5); err == nil {
		t.Error("syncing an untracked repository should fail")
	}
	if _, err := c.View("nosuchrepo", 50); err == nil {
		t.Error("viewing an untracked repository should fail")
	}
}

// TestClient_ViewWithoutSyncIsEmptyNotAnError covers the first frame: an app
// can render a repository it has only just added, before the fetch lands.
func TestClient_ViewWithoutSyncIsEmptyNotAnError(t *testing.T) {
	c, _ := mobilecore.New(t.TempDir())
	id, err := c.AddRepo("https://github.com/ezcdlabs/clarity.git", "main")
	if err != nil {
		t.Fatalf("AddRepo: %v", err)
	}
	raw, err := c.View(id, 50)
	if err != nil {
		t.Fatalf("a view before the first sync should be empty, not an error: %v", err)
	}
	var view v1.View
	if err := proto.Unmarshal(raw, &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, f := range view.Flows {
		for _, g := range f.Groups {
			if len(g.Commits) > 0 {
				t.Error("commits appeared before anything was fetched")
			}
		}
	}
}
