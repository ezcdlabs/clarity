package clarityrefs

import "testing"

// TestRedactURL covers stripping credentials out of a remote URL before it
// reaches an error message.
//
// Embedding a token in the remote is a common CI pattern — GitHub's
// x-access-token, GitLab's CI_JOB_TOKEN, and plenty of hand-rolled runners do
// it — and `git remote get-url` hands it back verbatim. git redacts
// credentials from its own diagnostics; anything clarity prints has to do the
// same, or a failed report writes a live token into a build log.
//
// Both halves are secret-bearing: the password obviously, and the username
// too, because a bare `https://<token>@host/...` puts the token there.
func TestRedactURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no credentials", "https://github.com/o/r.git", "https://github.com/o/r.git"},
		{"token as password", "https://x-access-token:ghs_SECRET@github.com/o/r.git", "https://github.com/o/r.git"},
		{"token as username", "https://ghp_SECRET@github.com/o/r.git", "https://github.com/o/r.git"},
		{"empty password still stripped", "https://user:@github.com/o/r.git", "https://github.com/o/r.git"},
		{"port preserved", "https://u:p@example.com:8443/o/r.git", "https://example.com:8443/o/r.git"},
		{"ssh url", "ssh://git@github.com/o/r.git", "ssh://github.com/o/r.git"},
		{"scp-like", "git@github.com:o/r.git", "github.com:o/r.git"},
		{"scp-like with token", "ghp_SECRET@github.com:o/r.git", "github.com:o/r.git"},
		{"local path", "/srv/git/repo.git", "/srv/git/repo.git"},
		{"local path with at sign", "/srv/git/we@ird.git", "/srv/git/we@ird.git"},
		{"file url", "file:///srv/git/repo.git", "file:///srv/git/repo.git"},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := redactURL(tc.in)
			if got != tc.want {
				t.Errorf("redactURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestRedactURL_NeverLeaksASecret is the property the table is really
// asserting: whatever the shape, nothing that looked like a credential
// survives.
func TestRedactURL_NeverLeaksASecret(t *testing.T) {
	const secret = "ghs_SUPERSECRETTOKEN"
	urls := []string{
		"https://x-access-token:" + secret + "@github.com/o/r.git",
		"https://" + secret + "@github.com/o/r.git",
		"https://" + secret + ":" + secret + "@github.com:443/o/r.git",
		"ssh://" + secret + "@github.com/o/r.git",
		secret + "@github.com:o/r.git",
		"http://u:" + secret + "@127.0.0.1:1/o/r.git",
	}
	for _, u := range urls {
		if got := redactURL(u); containsSecret(got, secret) {
			t.Errorf("redactURL(%q) leaked the credential: %q", u, got)
		}
	}
}

func containsSecret(s, secret string) bool {
	return len(secret) > 0 && len(s) >= len(secret) && (func() bool {
		for i := 0; i+len(secret) <= len(s); i++ {
			if s[i:i+len(secret)] == secret {
				return true
			}
		}
		return false
	})()
}
