package clarityrefs

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/internal/gittest"
)

// headRev resolves HEAD in the clone.
func headRev(t *testing.T, repoPath string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// fetchScopeRef pulls the ref back into the clone, since writes push to the
// remote rather than updating the local ref in place.
func fetchScopeRef(t *testing.T, repoPath string) {
	t.Helper()
	cmd := exec.Command("git", "fetch", "origin", "+"+EventsRef+":"+EventsRef)
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fetch events ref: %v\n%s", err, out)
	}
}

// TestScope_RoundTrip pins the on-disk shape. Candidacy is a property of the
// commit, not an outcome of an attempt, so it carries no stage and no status —
// which is the whole reason it doesn't live in the events tree.
func TestScope_RoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		scope    Scope
		wantJSON string
	}{
		{
			name:     "affected",
			scope:    Scope{Target: "ios", Affected: true, Time: time.Unix(1744120134, 0)},
			wantJSON: `{"target":"ios","affected":true,"ts":1744120134}`,
		},
		{
			name:     "unaffected",
			scope:    Scope{Target: "android", Affected: false, Time: time.Unix(1744120134, 0)},
			wantJSON: `{"target":"android","affected":false,"ts":1744120134}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := c.scope.marshal()
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(data) != c.wantJSON {
				t.Errorf("marshal = %s, want %s", data, c.wantJSON)
			}
			got, err := unmarshalScope(data)
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got != c.scope {
				t.Errorf("round-tripped %+v, want %+v", got, c.scope)
			}
		})
	}
}

// "affected" must serialise even when false. It is the difference between "CI
// said this commit doesn't touch android" and "nobody has said anything",
// which are different answers and drive different lead times.
func TestScope_AffectedFalseIsNotOmitted(t *testing.T) {
	data, err := Scope{Target: "ios", Affected: false, Time: time.Unix(1, 0)}.marshal()
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["affected"]; !ok {
		t.Errorf("affected:false was omitted, making it indistinguishable from silence: %s", data)
	}
}

// Two candidacy records for the same commit and instant must not collide, or
// one target's answer silently overwrites another's.
func TestScope_TargetsDoNotCollide(t *testing.T) {
	at := time.Unix(1744120134, 0)
	hash := func(s Scope) string {
		data, err := s.marshal()
		if err != nil {
			t.Fatal(err)
		}
		return contentHash(data)
	}

	ios := hash(Scope{Target: "ios", Affected: true, Time: at})
	android := hash(Scope{Target: "android", Affected: true, Time: at})
	iosNot := hash(Scope{Target: "ios", Affected: false, Time: at})

	if ios == android {
		t.Error("two targets hash alike — one would overwrite the other")
	}
	if ios == iosNot {
		t.Error("affected and unaffected hash alike for the same target")
	}
}

// TestScope_IsInvisibleToEventReaders is the forward-compatibility promise
// that justified a separate tree: a repo adopting candidacy must stay readable
// by every clarity released before it existed. Both event readers filter on
// the events/ prefix, so this pins that they keep doing so.
func TestScope_IsInvisibleToEventReaders(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "x")
	clone.CommitAll("one")
	clone.Push("main")
	sha := headRev(t, clone.Path)

	if err := WriteEvent(clone.Path, "origin", sha, Event{
		Stage: "deploy", Status: "passed", Time: time.Unix(100, 0), Target: "ios",
	}); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}
	if err := WriteScope(clone.Path, "origin", sha, Scope{
		Target: "android", Affected: false, Time: time.Unix(101, 0),
	}); err != nil {
		t.Fatalf("WriteScope: %v", err)
	}
	fetchScopeRef(t, clone.Path)

	events, err := ReadEvents(clone.Path, sha)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("candidacy leaked into ReadEvents: got %d events, want 1: %+v", len(events), events)
	}
	if events[0].Stage != "deploy" {
		t.Errorf("unexpected event: %+v", events[0])
	}

	all, err := ReadAllEvents(clone.Path)
	if err != nil {
		t.Fatalf("ReadAllEvents: %v", err)
	}
	if len(all[sha]) != 1 {
		t.Errorf("candidacy leaked into ReadAllEvents under the commit: %+v", all[sha])
	}
	// A leak doesn't necessarily land under the right SHA — a scope path
	// parsed as an event path yields "scope" as its commit — so check the
	// whole map, not just this commit's slice.
	if len(all) != 1 {
		t.Errorf("ReadAllEvents returned %d commits, want 1: %v", len(all), keysOf(all))
	}
	for commit, evs := range all {
		for _, e := range evs {
			if e.Stage == "" {
				t.Errorf("ReadAllEvents returned a stage-less record under %q — "+
					"candidacy is being parsed as a pipeline event: %+v", commit, e)
			}
		}
	}

	scope, err := ReadAllScope(clone.Path)
	if err != nil {
		t.Fatalf("ReadAllScope: %v", err)
	}
	if len(scope[sha]) != 1 || scope[sha][0].Target != "android" || scope[sha][0].Affected {
		t.Errorf("candidacy did not round-trip through the ref: %+v", scope[sha])
	}
}

func keysOf(m map[string][]Event) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestScope_RequiredFields is the read-side half of "false is not absence".
// The writer always emits `affected`, but a record can also arrive truncated,
// hand-edited, or from a foreign tool — and an event JSON is structurally
// valid scope JSON, so without these checks a strayed event reads back as a
// confident "unaffected" for the untargeted deploy.
func TestScope_RequiredFields(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{name: "no affected field", json: `{"target":"ios","ts":100}`},
		{name: "no target field", json: `{"affected":false,"ts":100}`},
		{name: "empty object", json: `{}`},
		{name: "an event that strayed into the scope tree",
			json: `{"stage":"deploy","status":"passed","ts":100,"target":"ios"}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, err := unmarshalScope([]byte(c.json)); err == nil {
				t.Errorf("accepted %s as %+v", c.json, got)
			}
		})
	}
}

// TestScope_WriteRequiresATarget — an empty target means the untargeted deploy
// by the Event convention, so a forgotten field would record a real claim
// about a real flow.
func TestScope_WriteRequiresATarget(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "x")
	clone.CommitAll("one")
	clone.Push("main")

	if err := WriteScope(clone.Path, "origin", headRev(t, clone.Path), Scope{}); err == nil {
		t.Error("a zero-value Scope was written")
	}
}

// TestScope_TwoTargetsSurviveTheSameInstant is the write-path half of the
// collision test. Hashing the content is only useful if the hash reaches the
// path — a fixed filename would silently keep one target's answer and discard
// the other.
func TestScope_TwoTargetsSurviveTheSameInstant(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "x")
	clone.CommitAll("one")
	clone.Push("main")
	sha := headRev(t, clone.Path)

	at := time.Unix(1744120134, 0)
	for _, target := range []string{"ios", "android"} {
		if err := WriteScope(clone.Path, "origin", sha, Scope{Target: target, Affected: false, Time: at}); err != nil {
			t.Fatalf("WriteScope(%s): %v", target, err)
		}
	}
	fetchScopeRef(t, clone.Path)

	got, err := ReadAllScope(clone.Path)
	if err != nil {
		t.Fatalf("ReadAllScope: %v", err)
	}
	if len(got[sha]) != 2 {
		t.Fatalf("want 2 records written in the same second, got %d: %+v", len(got[sha]), got[sha])
	}
}

// TestScope_ReadsAreSortedAndPrefixed pins the two properties a consumer
// depends on: records come back in time order (so "latest wins" is
// resolvable), and events never leak into them.
func TestScope_ReadsAreSortedAndPrefixed(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "x")
	clone.CommitAll("one")
	clone.Push("main")
	sha := headRev(t, clone.Path)

	// Written newest-first, and with timestamps whose decimal filenames sort
	// the opposite way to their chronology.
	for _, s := range []Scope{
		{Target: "ios", Affected: true, Time: time.Unix(1000000000, 0)},
		{Target: "android", Affected: false, Time: time.Unix(999999999, 0)},
	} {
		if err := WriteScope(clone.Path, "origin", sha, s); err != nil {
			t.Fatalf("WriteScope: %v", err)
		}
	}
	if err := WriteEvent(clone.Path, "origin", sha, Event{
		Stage: "deploy", Status: "passed", Time: time.Unix(5, 0), Target: "ios",
	}); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}
	fetchScopeRef(t, clone.Path)

	got, err := ReadAllScope(clone.Path)
	if err != nil {
		t.Fatalf("ReadAllScope: %v", err)
	}
	// Events must not leak in — and a leaked event lands under the key
	// "events", not under a real SHA, so the whole map has to be checked.
	if len(got) != 1 {
		t.Fatalf("ReadAllScope returned %d keys, want 1: %+v", len(got), got)
	}
	if len(got[sha]) != 2 {
		t.Fatalf("want 2 records, got %d: %+v", len(got[sha]), got[sha])
	}
	if !got[sha][0].Time.Before(got[sha][1].Time) {
		t.Errorf("records are not in time order: %+v", got[sha])
	}
}
