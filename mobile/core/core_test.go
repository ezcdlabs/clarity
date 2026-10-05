package core_test

import (
	"fmt"
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
	if err := syncOK(t, c, id); err != nil {
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
	for _, sec := range view.Flows[0].Sections {
		if sec.Kind != v1.SectionKind_SECTION_KIND_DEPLOYED {
			continue
		}
		for _, b := range sec.Batches {
			if len(b.Commits) > 0 {
				deployed = b.Commits[0]
			}
		}
	}
	if deployed == nil {
		t.Fatalf("the deployed commit did not survive the round trip: %+v", &view)
	}
	if deployed.Subject != "feat: first" {
		t.Errorf("subject = %q", deployed.Subject)
	}
	if !deployed.LeadTimeKnown || deployed.LeadTime == "" {
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
	// An untracked id is a programming error on the caller's side, not an
	// outcome the UI draws — so it still comes back as an error rather than as
	// a result.
	if _, err := c.Sync("nosuchrepo", 50, 5); err == nil {
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
		for _, sec := range f.Sections {
			if len(sec.Commits) > 0 || len(sec.Batches) > 0 {
				t.Error("commits appeared before anything was fetched")
			}
		}
	}
}

// TestClient_PayloadsAreNeverEmpty guards the one shape gomobile cannot carry.
//
// Its fromSlice turns any zero-length []byte into a null jbyteArray on Android
// and a nil NSData on iOS. An empty protobuf encoding is zero bytes — which is
// exactly what a message with no fields set produces — so a legal payload
// arrives as null and the decoder throws. An empty repository list is the first
// thing a new install asks for, so this is the common case, not an edge one.
func TestClient_PayloadsAreNeverEmpty(t *testing.T) {
	client, err := mobilecore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := client.ListRepos()
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) == 0 {
		t.Fatal("an empty repository list encodes to zero bytes, which arrives as null")
	}

	var list v1.RepoList
	if err := proto.Unmarshal(encoded, &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list.Repos) != 0 {
		t.Errorf("expected no repositories, got %d", len(list.Repos))
	}
	if list.GeneratedUnixSeconds == 0 {
		t.Error("nothing was set, so the payload is only non-empty by luck")
	}
}

// TestElapsed covers the formatter the clients tick with.
//
// A running timer has to be recomputed on the device every second, and the
// alternative to exposing this was reimplementing the rule in Kotlin and in
// Swift — two more places for "3m 29s" to drift into "3:29".
func TestElapsed(t *testing.T) {
	tests := []struct {
		name    string
		seconds int64
		want    string
	}{
		{"zero is still a duration", 0, "0s"},
		{"seconds", 45, "45s"},
		{"minutes and seconds", 209, "3m 29s"},
		{"hours", 53101, "14h 45m 01s"},
		// A phone's clock and a CI host's clock disagree. Counting backwards
		// from a deploy that has not happened yet reads as a bug.
		{"a clock skew reads as nothing elapsed", -30, "0s"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := mobilecore.Elapsed(tc.seconds); got != tc.want {
				t.Errorf("Elapsed(%d) = %q, want %q", tc.seconds, got, tc.want)
			}
		})
	}
}

// TestClient_ViewRemembersWhatItSaw is what lets a switcher show how every
// repository is doing without opening each one. Reading a view is the only
// moment the answer is known, so it is the moment to write it down.
func TestClient_ViewRemembersWhatItSaw(t *testing.T) {
	remoteRepo := gittest.NewRemote(t)
	clone := remoteRepo.NewClone(t)
	clone.WriteFile("f.txt", "one")
	clone.CommitAll("feat: first")
	clone.Push("main")
	head := clone.LogBranch("main")[0].Hash
	if err := clarityrefs.WriteEvent(clone.Path, "origin", head, clarityrefs.Event{
		Stage: "ci", Status: "passed", Time: time.Now(),
	}); err != nil {
		t.Fatalf("seed event: %v", err)
	}

	client, err := mobilecore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id, err := client.AddRepo("file://"+remoteRepo.URL(), "main")
	if err != nil {
		t.Fatal(err)
	}

	before := repoByID(t, client, id)
	if before.Ci != v1.Status_STATUS_UNSPECIFIED {
		t.Errorf("a never-read repository already has a verdict: %v", before.Ci)
	}

	if err := syncOK(t, client, id); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if _, err := client.View(id, 50); err != nil {
		t.Fatalf("view: %v", err)
	}

	after := repoByID(t, client, id)
	if after.Ci != v1.Status_STATUS_PASSED {
		t.Errorf("cached ci = %v, want PASSED", after.Ci)
	}
	if len(after.Flows) == 0 {
		t.Error("no flow verdicts cached; the switcher has nothing to draw per target")
	}
}

// TestClient_RenameIsLocalAndReversible covers the rename sheet. The alias sits
// beside the derived name rather than replacing it, so clearing it can put the
// real name back.
func TestClient_RenameIsLocalAndReversible(t *testing.T) {
	client, err := mobilecore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id, err := client.AddRepo("git@github.com:acme/web-platform.git", "main")
	if err != nil {
		t.Fatal(err)
	}

	got := repoByID(t, client, id)
	if got.Name != "web-platform" || got.Namespace != "acme" || got.Host != "github.com" {
		t.Fatalf("labels = %q / %q / %q", got.Namespace, got.Name, got.Host)
	}

	if err := client.Rename(id, "The Platform"); err != nil {
		t.Fatal(err)
	}
	got = repoByID(t, client, id)
	if got.Alias != "The Platform" {
		t.Errorf("alias = %q", got.Alias)
	}
	if got.Name != "web-platform" {
		t.Errorf("the derived name was overwritten by the rename: %q", got.Name)
	}

	if err := client.Rename(id, ""); err != nil {
		t.Fatal(err)
	}
	if got = repoByID(t, client, id); got.Alias != "" {
		t.Errorf("clearing the rename left %q", got.Alias)
	}
}

func repoByID(t *testing.T, client *mobilecore.Client, id string) *v1.RepoSummary {
	t.Helper()
	raw, err := client.ListRepos()
	if err != nil {
		t.Fatal(err)
	}
	var list v1.RepoList
	if err := proto.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	for _, r := range list.Repos {
		if r.Id == id {
			return r
		}
	}
	t.Fatalf("no repository %q in the list", id)
	return nil
}

// TestClient_SyncSucceedsOverAnUnauthenticatedRemote keeps the ordinary path
// honest: a file remote needs no key and no host, and must still come back OK.
func TestClient_SyncSucceedsOverAnUnauthenticatedRemote(t *testing.T) {
	remoteRepo := gittest.NewRemote(t)
	clone := remoteRepo.NewClone(t)
	clone.WriteFile("f.txt", "one")
	clone.CommitAll("feat: first")
	clone.Push("main")

	client, err := mobilecore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id, err := client.AddRepo("file://"+remoteRepo.URL(), "main")
	if err != nil {
		t.Fatal(err)
	}

	raw, err := client.Sync(id, 0, 60)
	if err != nil {
		t.Fatal(err)
	}
	var got v1.SyncResult
	if err := proto.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Outcome != v1.Outcome_OUTCOME_OK {
		t.Fatalf("outcome = %v, message %q", got.Outcome, got.Message)
	}
	if got.HostKey != nil {
		t.Error("a file remote has no host key to ask about")
	}
}

// syncOK fetches and insists the outcome was success, so a test that only
// cares about what came back cannot pass on a fetch that quietly refused.
func syncOK(t *testing.T, c *mobilecore.Client, id string) error {
	t.Helper()
	raw, err := c.Sync(id, 0, 60)
	if err != nil {
		return err
	}
	var got v1.SyncResult
	if err := proto.Unmarshal(raw, &got); err != nil {
		return err
	}
	if got.Outcome != v1.Outcome_OUTCOME_OK {
		return fmt.Errorf("outcome %v: %s", got.Outcome, got.Message)
	}
	return nil
}

// TestClient_Metrics walks the weekly aggregates across the bridge.
//
// A separate read from View and over a much larger commit window, because this
// answers "are we getting better?" rather than "is main green right now?" —
// the same reason `git clarity metrics` never polls.
func TestClient_Metrics(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("f.txt", "one")
	clone.CommitAll("feat: first")
	clone.Push("main")
	head := clone.LogBranch("main")[0].Hash
	if err := clarityrefs.WriteEvent(clone.Path, "origin", head, clarityrefs.Event{
		Stage: "deploy", Status: "passed", Time: time.Now().Add(30 * time.Second),
	}); err != nil {
		t.Fatalf("seed event: %v", err)
	}

	c, err := mobilecore.New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	id, err := c.AddRepo("file://"+remote.URL(), "main")
	if err != nil {
		t.Fatalf("AddRepo: %v", err)
	}
	if err := syncOK(t, c, id); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	raw, err := c.Metrics(id, 2000, 12)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	var metrics v1.Metrics
	if err := proto.Unmarshal(raw, &metrics); err != nil {
		t.Fatalf("the bytes crossing the bridge did not decode: %v", err)
	}
	if len(metrics.Flows) == 0 {
		t.Fatalf("no flows: %+v", &metrics)
	}
	flow := metrics.Flows[0]
	if len(flow.Weeks) == 0 {
		t.Fatalf("no weeks, so there is nothing to plot: %+v", flow)
	}
	if flow.Weeks[0].Deploys != 1 {
		t.Errorf("deploys = %d, want the one that shipped", flow.Weeks[0].Deploys)
	}
	if flow.Axis == nil || len(flow.Axis.Ticks) != 5 {
		t.Errorf("axis = %+v, want a scale with five labelled ticks", flow.Axis)
	}
	if metrics.GeneratedUnixSeconds == 0 {
		// Load-bearing: a message with no field set encodes to zero bytes, and
		// gomobile turns a zero-length slice into a null array on both
		// platforms.
		t.Error("the payload is unstamped, so an empty one could not cross")
	}
}

// TestClient_Metrics_WindowIsWeeksNotCommits pins what --weeks means here: how
// far back you can see must not depend on how busy the repository was.
func TestClient_Metrics_WindowIsWeeksNotCommits(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("f.txt", "one")
	clone.CommitAll("feat: first")
	clone.Push("main")
	head := clone.LogBranch("main")[0].Hash
	for _, offset := range []time.Duration{30 * time.Second, -8 * 24 * time.Hour} {
		if err := clarityrefs.WriteEvent(clone.Path, "origin", head, clarityrefs.Event{
			Stage: "deploy", Status: "passed", Time: time.Now().Add(offset),
		}); err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}

	c, err := mobilecore.New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	id, err := c.AddRepo("file://"+remote.URL(), "main")
	if err != nil {
		t.Fatalf("AddRepo: %v", err)
	}
	if err := syncOK(t, c, id); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	raw, err := c.Metrics(id, 2000, 1)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	var metrics v1.Metrics
	if err := proto.Unmarshal(raw, &metrics); err != nil {
		t.Fatal(err)
	}
	if n := len(metrics.Flows[0].Weeks); n != 1 {
		t.Errorf("asked for one week and got %d", n)
	}
}

// TestClient_Check walks a background check across the bridge: the first one
// records a baseline in silence, and a later one reports what moved.
func TestClient_Check(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("f.txt", "one")
	clone.CommitAll("feat: first")
	clone.Push("main")
	head := clone.LogBranch("main")[0].Hash
	// Increasing times, explicitly. Events are collapsed newest-wins and are
	// filed by the second they carry, so two written in the same second leave
	// which one is current up to the tie-break rather than to the test.
	at := time.Now().Add(-time.Hour)
	write := func(stage, status string) {
		at = at.Add(time.Minute)
		if err := clarityrefs.WriteEvent(clone.Path, "origin", head, clarityrefs.Event{
			Stage: stage, Status: status, Time: at,
		}); err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}
	write("ci", "passed")

	c, err := mobilecore.New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.AddRepo("file://"+remote.URL(), "main"); err != nil {
		t.Fatalf("AddRepo: %v", err)
	}

	first := check(t, c)
	if len(first.Changes) != 0 {
		t.Errorf("the first check announced %+v; there was nothing to move from", first.Changes)
	}
	if first.Checked != 1 {
		t.Errorf("checked = %d, want the one tracked repository", first.Checked)
	}

	// Nothing has happened since.
	if quiet := check(t, c); len(quiet.Changes) != 0 {
		t.Errorf("an unchanged repository announced %+v", quiet.Changes)
	}

	write("ci", "failed")
	broke := check(t, c)
	if len(broke.Changes) != 1 {
		t.Fatalf("got %+v, want the CI failure", broke.Changes)
	}
	if !broke.Changes[0].Broke || broke.Changes[0].Stage != "CI" {
		t.Errorf("change = %+v, want CI breaking", broke.Changes[0])
	}
	if broke.Changes[0].RepoName == "" {
		t.Error("the change does not say which repository, so a notification cannot either")
	}

	write("ci", "passed")
	fixed := check(t, c)
	if len(fixed.Changes) != 1 || fixed.Changes[0].Broke {
		t.Fatalf("got %+v, want the recovery", fixed.Changes)
	}
}

// TestClient_Check_AnUnreachableRepositoryIsCountedNotFatal. A check that
// quietly failed for every repository looks exactly like one that found nothing
// wrong, and the difference is the whole value of the feature.
func TestClient_Check_AnUnreachableRepositoryIsCountedNotFatal(t *testing.T) {
	c, err := mobilecore.New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.AddRepo("ssh://git@nowhere.invalid/acme/thing.git", "main"); err != nil {
		t.Fatalf("AddRepo: %v", err)
	}

	got := check(t, c)
	if got.Unreachable != 1 {
		t.Errorf("unreachable = %d, want the one that could not be fetched", got.Unreachable)
	}
	if len(got.Changes) != 0 {
		t.Errorf("a repository that could not be reached announced %+v", got.Changes)
	}
}

func check(t *testing.T, c *mobilecore.Client) *v1.Changes {
	t.Helper()
	raw, err := c.Check(30)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	var out v1.Changes
	if err := proto.Unmarshal(raw, &out); err != nil {
		t.Fatalf("the bytes crossing the bridge did not decode: %v", err)
	}
	return &out
}
