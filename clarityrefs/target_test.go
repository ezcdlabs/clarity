package clarityrefs

import (
	"encoding/json"
	"testing"
	"time"
)

// TestEventTarget_RoundTrip covers the target field's on-disk contract: it
// serialises under "target", survives a round trip, and is omitted entirely
// when empty so an untargeted event's JSON is byte-identical to what older
// versions wrote.
func TestEventTarget_RoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		event    Event
		wantJSON string
	}{
		{
			name:     "untargeted event omits the field",
			event:    Event{Stage: "deploy", Status: "passed", Time: time.Unix(1744120134, 0)},
			wantJSON: `{"stage":"deploy","status":"passed","ts":1744120134}`,
		},
		{
			name:     "targeted event carries it",
			event:    Event{Stage: "deploy", Status: "passed", Time: time.Unix(1744120134, 0), Target: "ios"},
			wantJSON: `{"stage":"deploy","status":"passed","ts":1744120134,"target":"ios"}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := c.event.marshal()
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(data) != c.wantJSON {
				t.Errorf("marshal = %s, want %s", data, c.wantJSON)
			}

			got, err := unmarshalEvent(data)
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.Target != c.event.Target {
				t.Errorf("round-tripped Target = %q, want %q", got.Target, c.event.Target)
			}
		})
	}
}

// TestEventTarget_LegacyEventsReadAsUntargeted pins the forward-compat
// promise: an event written before targets existed has no "target" key, and
// must read back as the untargeted deploy rather than as anything else.
func TestEventTarget_LegacyEventsReadAsUntargeted(t *testing.T) {
	legacy := []byte(`{"stage":"deploy","status":"passed","ts":1744120134}`)

	got, err := unmarshalEvent(legacy)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Target != "" {
		t.Errorf("legacy event Target = %q, want empty", got.Target)
	}
}

// TestEventTarget_ChangesTheContentHash is what keeps two flows' events from
// colliding. Filenames are content-addressed, so a deploy to ios and a deploy
// to android at the same instant must hash differently or one silently
// overwrites the other.
func TestEventTarget_ChangesTheContentHash(t *testing.T) {
	at := time.Unix(1744120134, 0)
	ios := Event{Stage: "deploy", Status: "passed", Time: at, Target: "ios"}
	android := Event{Stage: "deploy", Status: "passed", Time: at, Target: "android"}
	untargeted := Event{Stage: "deploy", Status: "passed", Time: at}

	hashOf := func(e Event) string {
		data, err := e.marshal()
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return contentHash(data)
	}

	iosHash, androidHash, bareHash := hashOf(ios), hashOf(android), hashOf(untargeted)
	if iosHash == androidHash {
		t.Errorf("ios and android events hash identically (%s) — one would overwrite the other", iosHash)
	}
	if iosHash == bareHash {
		t.Errorf("targeted and untargeted events hash identically (%s)", iosHash)
	}

	// Same event twice still collapses — idempotency must survive the new field.
	if hashOf(ios) != iosHash {
		t.Error("hashing is not deterministic for the same targeted event")
	}
}

// TestEventTarget_UnknownFieldsStillIgnored guards the loosening the target
// field must not cause: an event carrying a key this build doesn't know about
// still parses rather than erroring.
func TestEventTarget_UnknownFieldsStillIgnored(t *testing.T) {
	future := []byte(`{"stage":"deploy","status":"passed","ts":1,"target":"ios","channel":"beta"}`)

	got, err := unmarshalEvent(future)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Target != "ios" {
		t.Errorf("Target = %q, want ios", got.Target)
	}

	// Sanity: the fixture really does carry an unknown key.
	var raw map[string]any
	if err := json.Unmarshal(future, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["channel"]; !ok {
		t.Fatal("fixture no longer carries an unknown field")
	}
}
