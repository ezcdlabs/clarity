package remote_test

import (
	"testing"

	"github.com/ezcdlabs/clarity/mobile/internal/remote"
)

// TestParse covers the naming rule in the Android handoff, which is pure git:
// the path after the host, minus a trailing .git, split on the last slash.
// There is deliberately no host detection — clarity does not know what GitHub
// is, and a rule that did would be wrong on the box in the cupboard.
func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		host      string
		namespace string
		repo      string
	}{
		{
			name:      "scp-like with a namespace",
			url:       "git@github.com:acme/web-platform.git",
			host:      "github.com",
			namespace: "acme",
			repo:      "web-platform",
		},
		{
			name:      "ssh url with a nested namespace",
			url:       "ssh://git@gitlab.com/acme/mobile/apps.git",
			host:      "gitlab.com",
			namespace: "acme/mobile",
			repo:      "apps",
		},
		{
			// An absolute path on the host. The leading slash is part of the
			// path, not a separator, and must not leave an empty first segment.
			name:      "absolute path on a private host",
			url:       "git@box.lan:/srv/git/infra.git",
			host:      "box.lan",
			namespace: "srv/git",
			repo:      "infra",
		},
		{
			name:      "no namespace at all",
			url:       "pi@nas.local:dotfiles",
			host:      "nas.local",
			namespace: "",
			repo:      "dotfiles",
		},
		{
			// A port is part of addressing, never part of the name.
			name:      "ssh url with a port",
			url:       "ssh://git@git.acme.dev:2222/team/thing.git",
			host:      "git.acme.dev",
			namespace: "team",
			repo:      "thing",
		},
		{
			name:      "no user in an ssh url",
			url:       "ssh://gitbox/one/two.git",
			host:      "gitbox",
			namespace: "one",
			repo:      "two",
		},
		{
			// Trailing slashes are punctuation, not an empty final segment.
			name:      "trailing slash",
			url:       "git@github.com:acme/thing/",
			host:      "github.com",
			namespace: "acme",
			repo:      "thing",
		},
		{
			// .git only comes off the end. A repository actually called
			// "thing.github" keeps its name.
			name:      "a dot in the name that is not a suffix",
			url:       "git@github.com:acme/thing.github",
			host:      "github.com",
			namespace: "acme",
			repo:      "thing.github",
		},
		{
			// Both at once. Trimming the slash has to happen before the .git
			// comes off, or the suffix is no longer at the end to be found.
			name:      "trailing slash after a .git",
			url:       "git@github.com:acme/thing.git/",
			host:      "github.com",
			namespace: "acme",
			repo:      "thing",
		},
		{
			// An email-shaped username. The last at-sign is the separator;
			// taking the first leaves half the username in the host.
			name:      "an at-sign inside the user",
			url:       "ssh://me@corp.com@git.acme.dev/team/thing.git",
			host:      "git.acme.dev",
			namespace: "team",
			repo:      "thing",
		},
		{
			name:      "https is addressed the same way",
			url:       "https://github.com/acme/web-platform.git",
			host:      "github.com",
			namespace: "acme",
			repo:      "web-platform",
		},
		{
			// Nothing recognisable. The whole string is the name rather than
			// an empty one: a wrong label beats a blank row.
			name:      "not a url at all",
			url:       "nonsense",
			host:      "",
			namespace: "",
			repo:      "nonsense",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := remote.Parse(tc.url)
			if got.Host != tc.host {
				t.Errorf("host = %q, want %q", got.Host, tc.host)
			}
			if got.Namespace != tc.namespace {
				t.Errorf("namespace = %q, want %q", got.Namespace, tc.namespace)
			}
			if got.Name != tc.repo {
				t.Errorf("name = %q, want %q", got.Name, tc.repo)
			}
		})
	}
}

// TestAnythingWithCharactersInItGetsAName guards the one thing a list row
// cannot survive. A URL that addresses nothing still has to print as
// something, so the whole string becomes the name rather than a blank row.
//
// A blank URL is not in this list on purpose: blank in, blank out is the
// honest answer, and the registry rejects one before it ever reaches here.
func TestAnythingWithCharactersInItGetsAName(t *testing.T) {
	for _, url := range []string{"git@host:", "ssh://host/", "/", ":", "git@host:.git", "   x   "} {
		if got := remote.Parse(url); got.Name == "" {
			t.Errorf("Parse(%q) produced no name at all", url)
		}
	}
}
