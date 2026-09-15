package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/core"
)

// loadDeploys writes a .ezcd.json containing the given clarity section and
// returns the parsed deploy flows.
func loadDeploys(t *testing.T, body string) ([]core.Flow, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".ezcd.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write .ezcd.json: %v", err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, err
	}
	return cfg.Deploys(), nil
}

// TestDeploys_Parsing covers the wire shapes the `deploys` key accepts. The
// shorthand exists because the overwhelmingly common case is a flow whose name
// is its only target, and making that a one-liner keeps the config honest for
// repos that never renamed anything.
func TestDeploys_Parsing(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []core.Flow
	}{
		{
			name: "absent section means no declared flows",
			body: `{}`,
			want: nil,
		},
		{
			name: "clarity section without deploys means no declared flows",
			body: `{"clarity": {"leadTime": "pipeline"}}`,
			want: nil,
		},
		{
			name: "bare strings are shorthand for name-is-its-own-target",
			body: `{"clarity": {"deploys": ["web", "ios"]}}`,
			want: []core.Flow{
				{Name: "web", Targets: []string{"web"}},
				{Name: "ios", Targets: []string{"ios"}},
			},
		},
		{
			name: "full form carries an explicit target list",
			body: `{"clarity": {"deploys": [{"name": "iOS", "targets": ["ios"]}]}}`,
			want: []core.Flow{{Name: "iOS", Targets: []string{"ios"}}},
		},
		{
			name: "empty-string target is the untargeted deploy",
			body: `{"clarity": {"deploys": [{"name": "web", "targets": ["", "web"]}]}}`,
			want: []core.Flow{{Name: "web", Targets: []string{"", "web"}}},
		},
		{
			name: "shapes can be mixed in one list",
			body: `{"clarity": {"deploys": ["android", {"name": "web", "targets": ["", "web"]}]}}`,
			want: []core.Flow{
				{Name: "android", Targets: []string{"android"}},
				{Name: "web", Targets: []string{"", "web"}},
			},
		},
		{
			name: "declaration order is preserved — it is the display order",
			body: `{"clarity": {"deploys": ["ios", "android", "web"]}}`,
			want: []core.Flow{
				{Name: "ios", Targets: []string{"ios"}},
				{Name: "android", Targets: []string{"android"}},
				{Name: "web", Targets: []string{"web"}},
			},
		},
		{
			name: "a flow with no targets listed defaults to its own name",
			body: `{"clarity": {"deploys": [{"name": "web"}]}}`,
			want: []core.Flow{{Name: "web", Targets: []string{"web"}}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := loadDeploys(t, c.body)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !flowsEqual(got, c.want) {
				t.Errorf("Deploys() = %+v, want %+v", got, c.want)
			}
		})
	}
}

// TestDeploys_Rejections covers the ambiguities that must fail at load rather
// than resolve arbitrarily at render time. Each of these would otherwise
// produce a config where "which flow does this event belong to?" or "which
// flow does --deploy=x select?" has more than one answer.
func TestDeploys_Rejections(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{
			name:    "two flows claiming the same target",
			body:    `{"clarity": {"deploys": [{"name": "a", "targets": ["ios"]}, {"name": "b", "targets": ["ios"]}]}}`,
			wantMsg: "ios",
		},
		{
			name:    "two flows claiming the untargeted deploy",
			body:    `{"clarity": {"deploys": [{"name": "a", "targets": [""]}, {"name": "b", "targets": [""]}]}}`,
			wantMsg: "untargeted",
		},
		{
			// Targets deliberately differ, so only the name check can reject
			// this. With a shared target the duplicate-target check fires
			// first and the assertion passes without the name check existing.
			name:    "duplicate flow names",
			body:    `{"clarity": {"deploys": [{"name": "web", "targets": ["a"]}, {"name": "web", "targets": ["b"]}]}}`,
			wantMsg: "declared twice",
		},
		{
			name:    "flow names differing only by case",
			body:    `{"clarity": {"deploys": [{"name": "web", "targets": ["a"]}, {"name": "Web", "targets": ["b"]}]}}`,
			wantMsg: "declared twice",
		},
		{
			name:    "an explicitly empty target list",
			body:    `{"clarity": {"deploys": [{"name": "web", "targets": []}]}}`,
			wantMsg: "lists no targets",
		},
		{
			name:    "the same target listed twice in one flow",
			body:    `{"clarity": {"deploys": [{"name": "web", "targets": ["web", "web"]}]}}`,
			wantMsg: "twice",
		},
		{
			name:    "a whitespace-only name",
			body:    `{"clarity": {"deploys": ["   "]}}`,
			wantMsg: "name",
		},
		{
			name:    "targets given as a string rather than a list",
			body:    `{"clarity": {"deploys": [{"name": "web", "targets": "web"}]}}`,
			wantMsg: "must be a list",
		},
		{
			name:    "a name colliding case-insensitively with another flow's target",
			body:    `{"clarity": {"deploys": [{"name": "web", "targets": ["a"]}, {"name": "legacy", "targets": ["Web"]}]}}`,
			wantMsg: "web",
		},
		{
			name: "a flow name colliding with another flow's target",
			// --deploy=web would be ambiguous: the flow named web, or the
			// flow owning the target web?
			body:    `{"clarity": {"deploys": [{"name": "web", "targets": ["frontend"]}, {"name": "legacy", "targets": ["web"]}]}}`,
			wantMsg: "web",
		},
		{
			name:    "a flow with no name",
			body:    `{"clarity": {"deploys": [{"targets": ["ios"]}]}}`,
			wantMsg: "name",
		},
		{
			name:    "an empty string in the shorthand form",
			body:    `{"clarity": {"deploys": [""]}}`,
			wantMsg: "name",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := loadDeploys(t, c.body)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), c.wantMsg) {
				t.Errorf("error %q does not mention %q", err, c.wantMsg)
			}
			// Every other error in this package names the config path it came
			// from; these must too, or a malformed entry in a long list is a
			// hunt.
			if !strings.Contains(err.Error(), "clarity.deploys") {
				t.Errorf("error %q does not name the config key", err)
			}
		})
	}
}

func flowsEqual(a, b []core.Flow) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || len(a[i].Targets) != len(b[i].Targets) {
			return false
		}
		for j := range a[i].Targets {
			if a[i].Targets[j] != b[i].Targets[j] {
				return false
			}
		}
	}
	return true
}

// TestDeploys_TargetsAreFoldedLikeNames closes the gap that made --deploy able
// to select the wrong deployable. The matcher folds case when resolving a
// target, so a config where two flows own "Web" and "web" would resolve
// --deploy=web to one flow while the deploy events sat in the other.
func TestDeploys_TargetsAreFoldedLikeNames(t *testing.T) {
	for _, body := range []string{
		`{"clarity": {"deploys": [{"name": "alpha", "targets": ["Web"]}, {"name": "beta", "targets": ["web"]}]}}`,
		`{"clarity": {"deploys": [{"name": "alpha", "targets": ["ios", "IOS"]}]}}`,
	} {
		if _, err := loadDeploys(t, body); err == nil {
			t.Errorf("accepted targets differing only by case: %s", body)
		}
	}
}
