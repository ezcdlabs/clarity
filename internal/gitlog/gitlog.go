// Package gitlog is a small shared helper for the commit-walk step
// every local-git Source adapter needs. refsource and ghsource both
// read commits from the same local repository — the only difference is
// where they get their events from. This package owns the gogit details
// so neither adapter has to.
package gitlog

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ezcdlabs/clarity/internal/core"
	"github.com/ezcdlabs/clarity/internal/gitenv"
)

// DefaultLimit is the commit-count cap used when callers pass limit <= 0.
// Kept aligned with refsource's pre-extraction default so behaviour is
// unchanged.
const DefaultLimit = 50

// Resolve returns the commit cap Walk will actually apply for limit,
// substituting DefaultLimit for a non-positive one. Exported because
// callers record the cap alongside the snapshot they build: resolving it
// themselves keeps the number they report identical to the number the walk
// used, instead of a raw 0 the renderer would go on to quote back at the
// user as "--limit 0 reached".
func Resolve(limit int) int {
	if limit <= 0 {
		return DefaultLimit
	}
	return limit
}

// Walk walks up to limit commits starting from the given branch and
// returns them newest-first. The branch is resolved against the local
// repository: the remote-tracking ref (refs/remotes/origin/<branch>) is
// preferred because the Source's fetch step keeps it current; the local
// branch ref is the fallback. Callers are responsible for fetching
// before calling — Walk does not touch the network.
//
// The second return reports whether the history continues past the limit.
// It cannot be inferred from the returned slice — `len(commits) == limit`
// is equally true of a repo that ends there and one with a thousand more
// commits below the cut — so the walk steps one commit past the limit to
// find out. That commit is not included in the result; the extra step is
// what the renderers use to tell "this is all of it" apart from "your
// --limit stopped here".
func Walk(repoPath, branch string, limit int) ([]core.Commit, bool, error) {
	limit = Resolve(limit)

	ref, err := resolveBranch(repoPath, branch)
	if err != nil {
		return nil, false, err
	}

	// One past the limit, for the same reason the go-git walk stepped one
	// past it: len(commits) == limit is equally true of a history that ends
	// there and one with a thousand more commits below the cut.
	records, err := logRecords(repoPath, ref, limit+1)
	if err != nil {
		return nil, false, err
	}

	more := len(records) > limit
	if more {
		records = records[:limit]
	}

	commits := make([]core.Commit, 0, len(records))
	for _, r := range records {
		c, err := parseRecord(r)
		if err != nil {
			return nil, false, fmt.Errorf("walk log: %w", err)
		}
		commits = append(commits, c)
	}
	return commits, more, nil
}

// logRecords asks git for the commit list.
//
// Walking with go-git meant resolving each parent out of .git/objects
// directly, which assumes the repository holds its whole history. A shallow
// clone does not, by design: `--depth=1` — the actions/checkout default, and
// so the shape most CI repositories have — records a graft boundary in
// .git/shallow whose oldest commit claims parents that were never
// downloaded. git honours the graft and stops; go-git does not read
// .git/shallow and followed the pointer into a missing object, failing the
// entire walk.
//
// Asking git also makes the walk indifferent to the other layouts a checkout
// can produce — partial clones and shared object stores — for the same reason
// reading the events ref does.
//
// -z separates commits with NUL, so a subject can hold anything but the
// newline that %s already excludes; within a record the four fields are
// newline-separated.
func logRecords(repoPath, ref string, max int) ([]string, error) {
	cmd := exec.Command("git", "log", "-z",
		"--format=%H%n%an%n%aI%n%s", "-n", strconv.Itoa(max), ref)
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("walk log: %w: %s", err, gitenv.Redact(strings.TrimSpace(stderr.String())))
	}

	var records []string
	for _, r := range strings.Split(string(out), "\x00") {
		if strings.TrimSpace(r) != "" {
			records = append(records, r)
		}
	}
	return records, nil
}

// parseRecord turns one "<sha>\n<author>\n<iso date>\n<subject>" record into
// a core.Commit. The subject is allowed to be empty; nothing else is.
func parseRecord(record string) (core.Commit, error) {
	parts := strings.SplitN(record, "\n", 4)
	if len(parts) < 4 {
		return core.Commit{}, fmt.Errorf("malformed log record %q", record)
	}
	when, err := time.Parse(time.RFC3339, parts[2])
	if err != nil {
		return core.Commit{}, fmt.Errorf("commit %s: unparseable date %q: %w", parts[0], parts[2], err)
	}
	return core.Commit{
		SHA:     parts[0],
		Subject: parts[3],
		Author:  parts[1],
		Time:    when,
	}, nil
}

// resolveBranch picks the ref the walk starts from, preferring the
// remote-tracking branch — the Source's fetch step keeps it current — and
// falling back to the local branch.
func resolveBranch(repoPath, branch string) (string, error) {
	remote := "refs/remotes/origin/" + branch
	local := "refs/heads/" + branch
	for _, ref := range []string{remote, local} {
		if refExists(repoPath, ref) {
			return ref, nil
		}
	}
	return "", fmt.Errorf("no ref found for branch %q (tried %s and %s)",
		branch, remote, local)
}

func refExists(repoPath, ref string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
