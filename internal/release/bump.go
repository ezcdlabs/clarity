// Package release derives the next semantic-version tag from the Conventional
// Commit messages landed since the previous tag, driving the auto-release job:
// feat → minor, fix → patch, a `!` marker or BREAKING CHANGE footer → major,
// everything else → no release.
package release

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Bump is a semantic-version bump level. The zero value, BumpNone, means no
// release-worthy change was found.
type Bump int

const (
	BumpNone Bump = iota
	BumpPatch
	BumpMinor
	BumpMajor
)

func (b Bump) String() string {
	switch b {
	case BumpPatch:
		return "patch"
	case BumpMinor:
		return "minor"
	case BumpMajor:
		return "major"
	default:
		return "none"
	}
}

// subjectRe matches a Conventional Commit subject line, capturing the type and
// an optional `!` breaking marker. The scope (parenthesised) is allowed but
// not captured.
var subjectRe = regexp.MustCompile(`^(?P<type>[a-zA-Z]+)(?:\([^)]*\))?(?P<bang>!)?:`)

// breakingFooterRe matches a BREAKING CHANGE / BREAKING-CHANGE footer, which
// must start its own line — a passing mention mid-sentence does not count.
var breakingFooterRe = regexp.MustCompile(`(?m)^BREAKING[ -]CHANGE:`)

// BumpFor returns the highest bump implied by any of the given commit messages.
// Each message may be multi-line (subject + body/footers).
func BumpFor(messages []string) Bump {
	highest := BumpNone
	for _, msg := range messages {
		if b := bumpForOne(msg); b > highest {
			highest = b
		}
	}
	return highest
}

func bumpForOne(msg string) Bump {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return BumpNone
	}
	subject := msg
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		subject = msg[:i]
	}

	m := subjectRe.FindStringSubmatch(subject)
	if m == nil {
		return BumpNone // not a conventional commit
	}
	commitType := strings.ToLower(m[subjectRe.SubexpIndex("type")])
	bang := m[subjectRe.SubexpIndex("bang")] != ""

	if bang || breakingFooterRe.MatchString(msg) {
		return BumpMajor
	}
	switch commitType {
	case "feat":
		return BumpMinor
	case "fix":
		return BumpPatch
	default:
		return BumpNone
	}
}

var semverTagRe = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)

// Next returns the next version tag given the latest semver tag and a bump.
// An empty latest means there is no prior tag, so the base is v0.0.0. Returns
// "" when bump is BumpNone, and an error when latest is non-empty but not a
// valid vX.Y.Z tag.
func Next(latest string, b Bump) (string, error) {
	if b == BumpNone {
		return "", nil
	}
	major, minor, patch := 0, 0, 0
	if latest != "" {
		m := semverTagRe.FindStringSubmatch(latest)
		if m == nil {
			return "", fmt.Errorf("latest tag %q is not a valid semver tag (vX.Y.Z)", latest)
		}
		major, _ = strconv.Atoi(m[1])
		minor, _ = strconv.Atoi(m[2])
		patch, _ = strconv.Atoi(m[3])
	}
	switch b {
	case BumpMajor:
		major, minor, patch = major+1, 0, 0
	case BumpMinor:
		minor, patch = minor+1, 0
	case BumpPatch:
		patch++
	}
	return fmt.Sprintf("v%d.%d.%d", major, minor, patch), nil
}
