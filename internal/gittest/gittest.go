// Package gittest provides test helpers for creating real on-disk git
// repositories. It is intended for use in tests only.
package gittest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// PassingTestCommand returns a shell command string that always exits 0.
func PassingTestCommand() string {
	if runtime.GOOS == "windows" {
		return "cmd /c exit 0"
	}
	return "true"
}

// RunOnceTestCommand returns a shell command that succeeds the first time it is
// run but fails on any subsequent run. flagFile must be a path that does not
// exist before the test starts; each invocation checks for the file and either
// creates it (succeeds) or fails because it already exists.
func RunOnceTestCommand(flagFile string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`cmd /c if exist %q (exit 1) else (type nul > %q)`, flagFile, flagFile)
	}
	return fmt.Sprintf(`test ! -f %q && touch %q`, flagFile, flagFile)
}

// FailingTestCommand returns a shell command string that always exits non-zero.
func FailingTestCommand() string {
	if runtime.GOOS == "windows" {
		return "cmd /c exit 1"
	}
	return "false"
}

// Commit is a minimal representation of a git commit.
type Commit struct {
	Hash    string
	Message string
}

// Remote is a bare git repository acting as the shared remote.
type Remote struct {
	Path string
}

// Clone is a working git clone of a Remote.
type Clone struct {
	Path string
	t    *testing.T
}

// NewRemote creates a temporary bare git repository with an initial commit on
// main. It is cleaned up automatically when the test ends.
func NewRemote(t *testing.T) *Remote {
	t.Helper()
	dir := t.TempDir()

	run(t, dir, "git", "init", "--bare", "--initial-branch=main")
	run(t, dir, "git", "config", "user.email", "test@example.com")
	run(t, dir, "git", "config", "user.name", "Test")

	// A bare repo has no working tree, so we seed the initial commit via a
	// temporary clone.
	seedDir := t.TempDir()
	run(t, seedDir, "git", "clone", dir, ".")
	run(t, seedDir, "git", "config", "user.email", "test@example.com")
	run(t, seedDir, "git", "config", "user.name", "Test")
	writeFile(t, seedDir, ".gitkeep", "")
	run(t, seedDir, "git", "add", ".")
	run(t, seedDir, "git", "commit", "-m", "initial commit")
	run(t, seedDir, "git", "push", "origin", "main")

	return &Remote{Path: dir}
}

// NewClone creates a working clone of the remote in a temp directory.
func (r *Remote) NewClone(t *testing.T) *Clone {
	t.Helper()
	dir := t.TempDir()

	run(t, dir, "git", "clone", r.Path, ".")
	run(t, dir, "git", "config", "user.email", "test@example.com")
	run(t, dir, "git", "config", "user.name", "Test")

	return &Clone{Path: dir, t: t}
}

// LogBranch returns commits on the given branch or ref, newest first.
func (r *Remote) LogBranch(branch string) []Commit {
	return logBranch(r.Path, branch)
}

// ListRefs returns all ref names present in the remote.
func (r *Remote) ListRefs() []string {
	out := runOutput(nil, r.Path, "git", "for-each-ref", "--format=%(refname)")
	var refs []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			refs = append(refs, line)
		}
	}
	return refs
}

// ReadFileAtRef returns the contents of a file in the tree of a given ref.
// Returns ("", false) if the file does not exist at that ref.
func (r *Remote) ReadFileAtRef(ref, filePath string) (string, bool) {
	cmd := exec.Command("git", "show", ref+":"+filePath)
	cmd.Dir = r.Path
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return string(out), true
}

// DeleteRef deletes a ref from the remote.
func (r *Remote) DeleteRef(ref string) {
	runOutput(nil, r.Path, "git", "update-ref", "-d", ref)
}

// WriteFile creates (or overwrites) a file relative to the clone root,
// creating parent directories as needed.
func (c *Clone) WriteFile(relPath, content string) {
	c.t.Helper()
	writeFile(c.t, c.Path, relPath, content)
}

// CommitAll stages all changes and creates a commit with the given message.
func (c *Clone) CommitAll(message string) {
	c.t.Helper()
	run(c.t, c.Path, "git", "add", ".")
	run(c.t, c.Path, "git", "commit", "-m", message)
}

// Push pushes the current HEAD to the given branch on origin.
func (c *Clone) Push(branch string) {
	c.t.Helper()
	run(c.t, c.Path, "git", "push", "origin", "HEAD:"+branch)
}

// PushRef pushes a specific local ref to a specific remote ref.
func (c *Clone) PushRef(localRef, remoteRef string) {
	c.t.Helper()
	run(c.t, c.Path, "git", "push", "origin", localRef+":"+remoteRef)
}

// Fetch fetches all refs from origin.
func (c *Clone) Fetch() {
	c.t.Helper()
	run(c.t, c.Path, "git", "fetch", "--all")
}

// LogBranch returns commits on the given branch or ref, newest first.
func (c *Clone) LogBranch(branch string) []Commit {
	return logBranch(c.Path, branch)
}

// CurrentBranch returns the short name of the current branch, or "" if HEAD is detached.
func (c *Clone) CurrentBranch() string {
	out := runOutput(nil, c.Path, "git", "symbolic-ref", "--short", "HEAD")
	return strings.TrimSpace(out)
}

// --- helpers -----------------------------------------------------------------

func logBranch(repoPath, branch string) []Commit {
	out := runOutput(nil, repoPath, "git", "log", branch, "--format=%H %s")
	var commits []Commit
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		// Only accept lines where the first field looks like a git hash (40 hex chars).
		if len(parts) == 2 && len(parts[0]) == 40 {
			commits = append(commits, Commit{Hash: parts[0], Message: parts[1]})
		}
	}
	return commits
}

func writeFile(t *testing.T, base, relPath, content string) {
	full := filepath.Join(base, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		if t != nil {
			t.Fatalf("writeFile mkdir: %v", err)
		}
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		if t != nil {
			t.Fatalf("writeFile: %v", err)
		}
	}
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		if t != nil {
			t.Fatalf("git command %v failed: %v\n%s", args, err, out)
		}
	}
}

func runOutput(t *testing.T, dir string, args ...string) string {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil && t != nil {
		t.Fatalf("git command %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}
