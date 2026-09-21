// Package gitopen opens repositories with go-git in a way that survives the
// object-store layouts CI checkouts produce.
package gitopen

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// gitDirName is the entry that marks a working copy, either as a directory
// or as a file pointing elsewhere.
const gitDirName = ".git"

// Repo opens the repository at path, following a shared object store even
// when it lives outside the repository directory.
//
// This is gogit.PlainOpen plus one option. PlainOpen builds its storage with
// no AlternatesFS, which leaves go-git resolving .git/objects/info/alternates
// against the repository's own directory — and its filesystem abstraction
// refuses to cross that boundary, so an absolute path to a mirror elsewhere
// on disk fails to resolve and every object in that mirror reads as "object
// not found".
//
// Shared object stores are how CI caches avoid re-downloading a repo on every
// run: the checkout writes an alternates file pointing at a mirror it keeps
// between jobs, and the workspace itself holds almost no objects. Rooting the
// alternates filesystem at / lets those paths resolve.
//
// Everything else matches PlainOpen exactly, including its handling of a
// bare repo and of a .git file, and its not enabling commondir.
func Repo(path string) (*gogit.Repository, error) {
	dot, wt, err := dotGitToOSFilesystems(path)
	if err != nil {
		return nil, err
	}
	if _, err := dot.Stat(""); err != nil {
		if os.IsNotExist(err) {
			return nil, gogit.ErrRepositoryNotExists
		}
		return nil, err
	}

	st := filesystem.NewStorageWithOptions(dot, cache.NewObjectLRUDefault(),
		filesystem.Options{AlternatesFS: osfs.New(string(filepath.Separator))})
	return gogit.Open(st, wt)
}

// dotGitToOSFilesystems locates the git directory and the working tree,
// mirroring go-git's own unexported version of this.
func dotGitToOSFilesystems(path string) (dot, wt billy.Filesystem, err error) {
	fs := osfs.New(path)

	fi, err := fs.Stat(gitDirName)
	if err != nil {
		if os.IsNotExist(err) {
			return fs, nil, nil // a bare repository
		}
		return nil, nil, err
	}
	if fi.IsDir() {
		dot, err = fs.Chroot(gitDirName)
		return dot, fs, err
	}

	// A .git file — a linked worktree or a submodule — names the real git
	// directory instead of being one.
	dot, err = dotGitFileToOSFilesystem(path, fs)
	if err != nil {
		return nil, nil, err
	}
	return dot, fs, nil
}

func dotGitFileToOSFilesystem(path string, fs billy.Filesystem) (billy.Filesystem, error) {
	f, err := fs.Open(gitDirName)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	const prefix = "gitdir: "
	line := string(b)
	if !strings.HasPrefix(line, prefix) {
		return nil, fmt.Errorf(".git file has no %q prefix", prefix)
	}
	gitdir := strings.TrimSpace(strings.Split(line[len(prefix):], "\n")[0])
	if filepath.IsAbs(gitdir) {
		return osfs.New(gitdir), nil
	}
	return osfs.New(fs.Join(path, gitdir)), nil
}
