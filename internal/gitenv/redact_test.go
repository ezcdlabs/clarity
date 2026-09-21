package gitenv

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

		// url.Parse rejects these — an invalid percent-escape, a
		// non-numeric port, a control character, a space. The fallback has
		// to redact them rather than hand back the original.
		{"unparseable percent", "https://user:SECRET%@github.com/o/r.git", "https://github.com/o/r.git"},
		{"unparseable port", "https://user:SECRET@host:notaport/o/r.git", "https://host:notaport/o/r.git"},
		{"unparseable newline", "https://user:SECRET\n@github.com/o/r.git", "https://github.com/o/r.git"},
		{"unparseable space", "https://user:SECRET @github.com/o/r.git", "https://github.com/o/r.git"},

		// A relative path is not an scp target, and neither is a Windows
		// path whose colon precedes the "@". Mangling these loses the name
		// of the remote the user has to go and fix.
		{"relative path with at sign", "we@ird/relative/repo.git", "we@ird/relative/repo.git"},
		{"windows path with at sign", "C:\\repos\\we@ird", "C:\\repos\\we@ird"},
		{"file url", "file:///srv/git/repo.git", "file:///srv/git/repo.git"},
		{"ipv6 literal", "https://u:p@[::1]:443/x", "https://[::1]:443/x"},
		{"percent-encoded at in username", "https://user%40host:p@github.com/x", "https://github.com/x"},
		{"several at signs, last wins", "https://a@b@c@github.com/x", "https://github.com/x"},
		{"uppercase scheme", "HTTPS://u:p@github.com/x", "HTTPS://github.com/x"},
		{"git scheme", "git://u:p@github.com/x", "git://github.com/x"},
		{"at sign in the path only", "https://github.com/o/we@ird.git", "https://github.com/o/we@ird.git"},
		{"empty authority", "https:///x", "https:///x"},
		{"a bare dash", "-", "-"},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactURL(tc.in)
			if got != tc.want {
				t.Errorf("RedactURL(%q) = %q, want %q", tc.in, got, tc.want)
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
		if got := RedactURL(u); containsSecret(got, secret) {
			t.Errorf("RedactURL(%q) leaked the credential: %q", u, got)
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

// TestRedactOutput covers scrubbing credentials out of git's own diagnostics
// before clarity embeds them in an error.
//
// git redacts the password from its messages but prints the username in the
// clear — and `https://<token>@host/...` carries the token as the username,
// which is the most common CI shape there is. Because clarity runs git with
// GIT_TERMINAL_PROMPT=0, "could not read Password for
// 'http://<token>@host'" is precisely the failure a misconfigured runner
// produces, so this is the likely leak rather than an exotic one.
func TestRedactOutput(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"token as username in git's prompt error",
			"fatal: could not read Password for 'http://ghs_SECRET@127.0.0.1:8080': terminal prompts disabled",
			"fatal: could not read Password for 'http://127.0.0.1:8080': terminal prompts disabled",
		},
		{
			"user and password pair",
			"fatal: unable to access 'https://user:ghs_SECRET@github.com/o/r.git/': 403",
			"fatal: unable to access 'https://github.com/o/r.git/': 403",
		},
		{
			"several occurrences on several lines",
			"a https://ghs_SECRET@h/x\nb https://u:ghs_SECRET@h/y",
			"a https://h/x\nb https://h/y",
		},
		{
			"nothing to redact is left alone",
			"fatal: could not read Username for 'https://github.com': terminal prompts disabled",
			"fatal: could not read Username for 'https://github.com': terminal prompts disabled",
		},
		// The delimiter is the LAST "@" before the path, not the first.
		// An email-style username is the normal form on Azure DevOps,
		// Bitbucket and most self-hosted setups, and a password may simply
		// contain an "@" — stopping at the first one strips the username
		// and leaves the secret standing.
		{
			"email-style username, secret after the second at",
			"fatal: could not read Password for 'https://me@corp.com:ghs_SECRET@dev.azure.com/o/r'",
			"fatal: could not read Password for 'https://dev.azure.com/o/r'",
		},
		{
			"at sign inside the password",
			"fatal: could not read Password for 'https://user:p@ghs_SECRET@github.com/o/r.git'",
			"fatal: could not read Password for 'https://github.com/o/r.git'",
		},
		{
			"a credential carrying a raw newline",
			"fatal: 'https://u:ghs_SECRET\n@github.com/x'",
			"fatal: 'https://github.com/x'",
		},

		// The match must stop at a space, or it runs from a scheme to an
		// unrelated "@" and deletes the diagnostic instead of the secret.
		{
			"prose with an unrelated at-sign is untouched",
			"see https://example.com and mail bob@corp",
			"see https://example.com and mail bob@corp",
		},
		{
			"an at-sign on a later line is untouched",
			"fatal: https://github.com\n  remote: contact admin@x.com",
			"fatal: https://github.com\n  remote: contact admin@x.com",
		},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Redact(tc.in); got != tc.want {
				t.Errorf("Redact(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestRedactOutput_NeverLeaksASecret is the property the table encodes.
func TestRedactOutput_NeverLeaksASecret(t *testing.T) {
	const secret = "ghs_SUPERSECRETTOKEN"
	outputs := []string{
		"fatal: could not read Password for 'http://" + secret + "@h:1': terminal prompts disabled",
		"fatal: unable to access 'https://u:" + secret + "@github.com/o/r.git/': 403",
		"remote: rejected\nfatal: https://" + secret + "@github.com/o/r.git failed",
		"ssh://" + secret + "@github.com/o/r.git",
		"fatal: unable to access 'https://u:" + secret + "@[::1]:443/x': err",
		"a 'https://" + secret + "@h/x' and \"https://" + secret + "@h2/y\"",
		"line1 https://" + secret + "@h/x\nline2 https://" + secret + "@h2/y",
		"https://u:" + secret + "\n@github.com/x",
		"fatal: could not read Password for 'https://me@corp.com:" + secret + "@dev.azure.com/o/r'",
		"fatal: could not read Password for 'https://user:p@" + secret + "@github.com/o/r.git'",
		"fatal: unable to access 'https://a@b@c@" + secret + "@github.com/x'",
	}
	for _, o := range outputs {
		if got := Redact(o); containsSecret(got, secret) {
			t.Errorf("Redact(%q) leaked the credential: %q", o, got)
		}
	}
}
