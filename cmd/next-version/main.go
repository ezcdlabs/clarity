// Command next-version prints the next semantic-version tag implied by the
// Conventional Commit messages landed since the most recent semver tag, or
// nothing at all when no release-worthy change is present. It is a CI helper
// (not part of the shipped git-clarity binary, like cmd/demo) driving the
// auto-release workflow; the bump logic itself lives in internal/release and
// is unit-tested there.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/ezcdlabs/clarity/internal/release"
)

func main() {
	latest, err := latestTag()
	if err != nil {
		fmt.Fprintln(os.Stderr, "next-version:", err)
		os.Exit(1)
	}

	messages, err := commitMessagesSince(latest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "next-version:", err)
		os.Exit(1)
	}

	next, err := release.Next(latest, release.BumpFor(messages))
	if err != nil {
		fmt.Fprintln(os.Stderr, "next-version:", err)
		os.Exit(1)
	}

	// Empty output (no newline) signals "no release"; the workflow gates on it.
	if next != "" {
		fmt.Println(next)
	}
}

// latestTag returns the highest vX.Y.Z tag, or "" when the repo has none yet.
func latestTag() (string, error) {
	out, err := exec.Command("git", "tag", "--sort=-v:refname").Output()
	if err != nil {
		return "", fmt.Errorf("git tag: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if semverTag.MatchString(line) {
			return line, nil
		}
	}
	return "", nil
}

var semverTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

// commitMessagesSince returns the full message of every commit reachable from
// HEAD but not from latest. When latest is empty (first release) it returns
// the full history. Messages are NUL-separated so multi-line bodies survive.
func commitMessagesSince(latest string) ([]string, error) {
	args := []string{"log", "-z", "--format=%B"}
	if latest != "" {
		args = append(args, latest+"..HEAD")
	}
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}
	var msgs []string
	for _, m := range strings.Split(string(out), "\x00") {
		if strings.TrimSpace(m) != "" {
			msgs = append(msgs, m)
		}
	}
	return msgs, nil
}
