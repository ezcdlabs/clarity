// Package gittest provides test helpers for creating real on-disk git
// repositories. It is intended for use in tests only.
package gittest

import (
	"fmt"
	"io/fs"
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

// Remote is a git repository acting as the shared remote in a test. Two
// concrete backends construct it:
//
//   - NewRemote (this file) — bare repo on the local filesystem; Path is
//     both the on-disk location AND the clone URL.
//   - NewSSHRemote (gittest_ssh.go, build tag "ssh") — Docker-hosted SSH
//     server; URL is "ssh://git@localhost:<mapped-port>/repo.git"; Path
//     is empty because the bare repo lives inside the container.
//
// Callers that need to drive git client commands should use URL(); callers
// that need to inspect repo internals directly (file reads, log walks)
// must use Path and are inherently local-only.
type Remote struct {
	// Path is the on-disk bare repo directory for local backends, empty
	// for SSH backends.
	Path string
	// url is the clone URL — either the same as Path (local) or an
	// ssh://... form (SSH). Unexported because callers should go through
	// URL() rather than constructing one.
	url string
}

// URL returns the clone URL for the remote. For local-backed remotes this
// is the bare repo path; for SSH-backed remotes this is the ssh:// URL.
// Either form is valid input to `git clone`.
func (r *Remote) URL() string { return r.url }

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

	return &Remote{Path: dir, url: dir}
}

// NewClone creates a working clone of the remote in a temp directory.
// Uses URL() so it works the same for local-file and SSH backends.
func (r *Remote) NewClone(t *testing.T) *Clone {
	t.Helper()
	dir := t.TempDir()

	run(t, dir, "git", "clone", r.URL(), ".")
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

// NewBloblessClone creates a blobless partial clone (`--filter=blob:none`) of
// the remote: the shape a checkout action produces when it passes a filter,
// as Blacksmith's checkout does when sparse checkout is enabled.
//
// Such a clone has `remote.origin.promisor=true`, so a later `git fetch` of
// any ref brings its commits and trees but leaves the blobs on the server, to
// be fetched lazily on first access. Plain `git` handles that transparently;
// a library reading the object store directly does not, which is the shape
// this helper exists to reproduce.
//
// The filter is only honoured over a real transport — `git clone --filter`
// against a local path silently ignores it ("--filter is ignored in local
// clones") — so this clones over file:// and enables uploadpack.allowfilter
// on the remote. SSH-backed remotes have no on-disk Path to serve, so the
// test is skipped there rather than silently producing a full clone.
func (r *Remote) NewBloblessClone(t *testing.T) *Clone {
	t.Helper()
	if r.Path == "" {
		t.Skip("blobless clone requires a local on-disk remote")
	}
	run(t, r.Path, "git", "config", "uploadpack.allowfilter", "true")

	dir := t.TempDir()
	run(t, dir, "git", "clone", "--filter=blob:none", fileURL(r.Path), ".")
	run(t, dir, "git", "config", "user.email", "test@example.com")
	run(t, dir, "git", "config", "user.name", "Test")

	if got := runOutput(t, dir, "git", "config", "--get", "remote.origin.promisor"); strings.TrimSpace(got) != "true" {
		t.Fatalf("clone is not a promisor/partial clone (remote.origin.promisor=%q); the filter was ignored", strings.TrimSpace(got))
	}
	return &Clone{Path: dir, t: t}
}

// fileURL converts an absolute on-disk path into a file:// URL, which is what
// forces git to use a real transport instead of the local-clone shortcut.
func fileURL(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // Windows drive letters: file:///C:/...
	}
	return "file://" + p
}

// SetRemoteURL repoints a remote at a different URL, so a test can make the
// remote unreachable without touching the rest of the repo.
func SetRemoteURL(t *testing.T, repoPath, remote, url string) {
	t.Helper()
	run(t, repoPath, "git", "remote", "set-url", remote, url)
}

// NewSharedObjectClone creates a working copy whose objects live in a separate
// mirror repository, reached through .git/objects/info/alternates.
//
// This is how CI caches avoid re-downloading a repo on every run — Blacksmith's
// checkout action keeps a mirror between jobs and points the workspace at it,
// and does not dissociate by default. The workspace itself ends up holding
// essentially no objects, so anything reading .git/objects directly sees an
// empty store while git itself reads the repo perfectly.
//
// The returned clone is verified to hold no objects of its own, so a test
// using it cannot quietly pass against a self-contained store.
func (r *Remote) NewSharedObjectClone(t *testing.T) *Clone {
	t.Helper()
	if r.Path == "" {
		t.Skip("shared object store requires a local on-disk remote")
	}

	mirror := filepath.Join(t.TempDir(), "mirror.git")
	run(t, t.TempDir(), "git", "clone", "--mirror", r.URL(), mirror)

	dir := t.TempDir()
	run(t, dir, "git", "init", "--initial-branch=main")
	run(t, dir, "git", "config", "user.email", "test@example.com")
	run(t, dir, "git", "config", "user.name", "Test")

	info := filepath.Join(dir, ".git", "objects", "info")
	if err := os.MkdirAll(info, 0o755); err != nil {
		t.Fatalf("creating objects/info: %v", err)
	}
	alt := filepath.Join(mirror, "objects")
	if err := os.WriteFile(filepath.Join(info, "alternates"), []byte(alt+"\n"), 0o644); err != nil {
		t.Fatalf("writing alternates: %v", err)
	}

	run(t, dir, "git", "remote", "add", "origin", r.URL())
	run(t, dir, "git", "fetch", "origin", "main")
	run(t, dir, "git", "checkout", "-B", "main", "FETCH_HEAD")

	if n := countObjects(t, dir); n != 0 {
		t.Fatalf("workspace holds %d objects of its own; the alternate is not being relied on", n)
	}
	return &Clone{Path: dir, t: t}
}

// countObjects returns how many object files the repo holds locally, ignoring
// the bookkeeping under objects/info.
func countObjects(t *testing.T, repoPath string) int {
	t.Helper()
	root := filepath.Join(repoPath, ".git", "objects")
	n := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "info" {
				return filepath.SkipDir
			}
			return nil
		}
		n++
		return nil
	})
	if err != nil {
		t.Fatalf("counting objects: %v", err)
	}
	return n
}

// NewShallowClone creates a clone truncated to the most recent commit
// (`--depth=1`), which is what actions/checkout produces by default and
// therefore the shape most CI repositories actually have.
//
// A shallow clone records a graft boundary in .git/shallow: the oldest commit
// it holds claims parents that are deliberately absent. git honours the graft
// and stops there; a walker that does not will follow the parent pointer into
// an object that was never downloaded.
//
// Depth is only honoured over a real transport, so this clones over file://.
func (r *Remote) NewShallowClone(t *testing.T) *Clone {
	t.Helper()
	if r.Path == "" {
		t.Skip("shallow clone requires a local on-disk remote")
	}

	dir := t.TempDir()
	run(t, dir, "git", "clone", "--depth=1", fileURL(r.Path), ".")
	run(t, dir, "git", "config", "user.email", "test@example.com")
	run(t, dir, "git", "config", "user.name", "Test")

	if _, err := os.Stat(filepath.Join(dir, ".git", "shallow")); err != nil {
		t.Fatalf("clone is not shallow (no .git/shallow); --depth was ignored: %v", err)
	}
	return &Clone{Path: dir, t: t}
}
