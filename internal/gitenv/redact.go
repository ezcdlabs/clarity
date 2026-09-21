package gitenv

import (
	"regexp"
	"strings"
)

// Credentials must never reach a message clarity prints.
//
// Embedding a token in a remote is routine in CI — GitHub's x-access-token,
// GitLab's CI_JOB_TOKEN, hand-rolled runners — and `git remote get-url`
// returns whatever is configured, verbatim. It also applies
// url.<base>.insteadOf rewriting, so a repository whose own remote looks
// clean can still hand back a URL carrying a token injected by global
// config. Redaction is load-bearing even where the remote looks harmless.
//
// git is only half a safety net here: it redacts the *password* from its own
// diagnostics but prints the username in the clear, and
// `https://<token>@host/...` carries the token as the username. Since clarity
// runs git with GIT_TERMINAL_PROMPT=0, the message a misconfigured runner
// produces is exactly "could not read Password for 'http://<token>@host'".
// So both clarity's own text and git's output have to be scrubbed.

// credentialInURL matches the userinfo of any scheme-qualified URL: a scheme,
// "://", then everything up to an "@" that precedes the first "/".
//
// Applied to free text, so it must not run past the authority into the path —
// hence excluding "/" and quotes, which end an authority in practice.
//
// Newlines are deliberately allowed inside the userinfo: a URL carrying a raw
// newline there would otherwise slip through intact, and under-redacting
// costs a live credential. A literal space is not, because that is what stops
// a runaway match — without it, "see https://example.com and mail bob@corp"
// matches from the scheme all the way to an unrelated "@" and deletes the
// line. Prose has spaces; URLs do not. The length bound is a second stop.
var credentialInURL = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.\-]*://)[^/@'" ]{0,256}@`)

// Redact is exported because every package that shells out to git embeds
// git's output in its errors, and each one is a route to the same log.
func Redact(s string) string {
	return credentialInURL.ReplaceAllString(s, "$1")
}

// Both halves of the userinfo are treated as secret: the password obviously,
// and the username because a bare `https://<token>@host/...` puts the token
// there.
// RedactURL removes credentials from a remote URL.
func RedactURL(raw string) string {
	if raw == "" {
		return ""
	}

	// Deliberately textual rather than net/url. url.Parse rejects plenty of
	// URLs that git accepts — an invalid percent-escape, a non-numeric port,
	// a stray space or control character — and a parse-based redactor has to
	// decide what to do when it fails. Returning the original is a leak, and
	// falling back to scp-style handling silently does nothing, because the
	// "//" in a scheme guarantees a "/" before the "@". Splitting the
	// authority off by hand has neither failure mode.
	if i := strings.Index(raw, "://"); i >= 0 {
		scheme, rest := raw[:i+3], raw[i+3:]
		authority, path := rest, ""
		if slash := strings.IndexByte(rest, '/'); slash >= 0 {
			authority, path = rest[:slash], rest[slash:]
		}
		if at := strings.LastIndexByte(authority, '@'); at >= 0 {
			authority = authority[at+1:]
		}
		return scheme + authority + path
	}

	return redactScpLike(raw)
}

// redactScpLike strips a leading "user[:pass]@" from git's scp-style remote
// syntax, "[user@]host:path".
//
// A filesystem path is left alone, because "@" is legal in a directory name
// and mangling the URL would hide which remote the user has to go and fix.
// The distinguishing feature of scp syntax is a colon *after* the "@" that
// separates host from path, with no "/" before the "@" — "we@ird/repo.git"
// is a relative path, and "C:\repos\we@ird" is a Windows one whose colon
// comes earlier.
func redactScpLike(raw string) string {
	at := strings.IndexByte(raw, '@')
	if at < 0 {
		return raw
	}
	if slash := strings.IndexByte(raw, '/'); slash >= 0 && slash < at {
		return raw
	}
	if !strings.Contains(raw[at+1:], ":") {
		return raw
	}
	return raw[at+1:]
}
