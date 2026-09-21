package clarityrefs

import (
	"net/url"
	"strings"
)

// redactURL removes any credentials embedded in a remote URL.
//
// `git remote get-url` returns whatever is configured, verbatim. Putting a
// token in the remote is routine in CI — GitHub's x-access-token, GitLab's
// CI_JOB_TOKEN, hand-rolled runners — so a URL printed into an error message
// can put a live credential in a build log. git redacts credentials from its
// own diagnostics; so does clarity.
//
// Both the username and the password are treated as secret: `https://
// <token>@host/...` carries the token as the username, and that form is at
// least as common as the paired one.
func redactURL(raw string) string {
	if raw == "" {
		return ""
	}

	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			// Unparseable: fall through to the conservative textual strip
			// below rather than risk returning it untouched.
			return redactAuthority(raw)
		}
		if u.User == nil {
			return raw
		}
		u.User = nil
		return u.String()
	}

	return redactAuthority(raw)
}

// redactAuthority strips a leading "user[:pass]@" from a scp-style remote
// (git@host:path) or from a URL that would not parse.
//
// A local filesystem path is left alone: it has no authority, and a path may
// legitimately contain "@". The distinction is the "@" coming before the
// first "/" — in scp syntax the credential precedes the host, while in a path
// any "@" sits inside a directory name.
func redactAuthority(raw string) string {
	at := strings.IndexByte(raw, '@')
	if at < 0 {
		return raw
	}
	if slash := strings.IndexByte(raw, '/'); slash >= 0 && slash < at {
		return raw // "@" is inside a path segment, not a credential
	}
	return raw[at+1:]
}
