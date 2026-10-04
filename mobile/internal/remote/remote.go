// Package remote reads a git URL for the parts a UI needs to label it.
//
// Pure git, and deliberately no host detection. Clarity does not know what
// GitHub is: a rule that special-cased it would be wrong on a GitLab instance
// and wrong again on a box in a cupboard, which are exactly the cases this
// project exists to treat the same. Everything here comes from the shape of
// the URL itself.
//
// Doing it here rather than in each app is what keeps Android and iOS calling
// the same repository by the same name.
package remote

import "strings"

// Ref is what a URL says about itself.
type Ref struct {
	// Host is for the subtitle — "main · github.com" — and is never part of
	// the name.
	Host string
	// Namespace is everything before the last path segment, shown dimmed in
	// front of the name. Empty when the path is a single segment.
	Namespace string
	// Name is the last path segment, and the one thing that is never
	// abbreviated away.
	Name string
}

// Parse reads a clone URL. It never fails: anything unrecognisable becomes the
// name, because a wrong label beats a blank row in a list.
func Parse(url string) Ref {
	raw := strings.TrimSpace(url)
	host, path := split(raw)

	// A trailing slash is punctuation, not an empty final segment. The .git
	// comes off the end only — a repository called "thing.github" keeps it.
	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")

	ref := Ref{Host: host}
	if i := strings.LastIndex(path, "/"); i >= 0 {
		ref.Namespace, ref.Name = path[:i], path[i+1:]
	} else {
		ref.Name = path
	}

	if ref.Name == "" {
		// Nothing usable came out. Showing the URL is unhelpful but visible,
		// and visible is the half that matters in a list.
		ref.Name = raw
		ref.Namespace = ""
	}
	return ref
}

// split separates the host from the path it addresses.
//
// Two shapes, which is all git has: a scheme URL, where the path starts at the
// first slash after the authority, and the scp-like form, where it starts at
// the first colon. The scp form has no port — git does not accept one there —
// so a colon in it is always the separator.
func split(raw string) (host, path string) {
	if i := strings.Index(raw, "://"); i >= 0 {
		rest := raw[i+3:]
		authority, path := cut(rest, "/")
		return hostOf(authority), path
	}
	if i := strings.Index(raw, ":"); i >= 0 {
		return hostOf(raw[:i]), raw[i+1:]
	}
	return "", raw
}

// hostOf drops any user and any port, leaving the host itself.
func hostOf(authority string) string {
	if i := strings.LastIndex(authority, "@"); i >= 0 {
		authority = authority[i+1:]
	}
	if i := strings.Index(authority, ":"); i >= 0 {
		authority = authority[:i]
	}
	return authority
}

func cut(s, sep string) (before, after string) {
	if i := strings.Index(s, sep); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}
