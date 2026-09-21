package clarityrefs

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ezcdlabs/clarity/internal/gitenv"
	"github.com/go-git/go-git/v5/plumbing"
)

// Reading the events ref goes through git rather than through go-git's object
// storage, because git is the only thing that knows how this particular
// working copy was cloned.
//
// go-git resolves objects straight out of .git/objects. That is correct only
// for a self-contained clone, and CI checkouts routinely produce something
// else:
//
//   - a partial clone (--filter=blob:none) leaves blobs on the server to be
//     fetched lazily on access — go-git has no promisor support;
//   - a shared object store points .git/objects/info/alternates at a mirror
//     elsewhere on disk — go-git's alternates support cannot follow a path
//     outside the repository directory, because its filesystem abstraction
//     refuses to cross that boundary.
//
// Both leave every object perfectly readable by git and invisible to go-git,
// and both surfaced as a bare "object not found" against a ref that was
// present and correct. Rather than keep chasing clone shapes one at a time,
// reads ask git — which handles all of them, and any future variation,
// by construction.
//
// Writes still build objects through go-git: creating loose objects in the
// local store always works, and the subsequent `git push` reads them back
// with git's own rules.

// readEventsFiles returns every file on the local events ref as path →
// content, together with the commit the ref points at.
//
// An absent ref is not an error: it is what a repo looks like before its
// first report, and callers get an empty map and a zero hash. Reads operate
// on the local ref only — callers fetch first.
func readEventsFiles(repoPath string) (map[string][]byte, plumbing.Hash, error) {
	files := make(map[string][]byte)

	head, ok, err := resolveRef(repoPath, EventsRef)
	if err != nil {
		return nil, plumbing.ZeroHash, err
	}
	if !ok {
		return files, plumbing.ZeroHash, nil
	}

	paths, hashes, err := listTree(repoPath, EventsRef)
	if err != nil {
		return nil, plumbing.ZeroHash, err
	}
	if len(paths) == 0 {
		return files, head, nil
	}

	contents, err := catFileBatch(repoPath, hashes)
	if err != nil {
		return nil, plumbing.ZeroHash, err
	}
	for i, p := range paths {
		files[p] = contents[i]
	}
	return files, head, nil
}

// resolveRef returns the commit a ref points at. ok is false when the ref
// does not exist, which is a normal state rather than a failure.
func resolveRef(repoPath, ref string) (plumbing.Hash, bool, error) {
	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.Output()
	text := strings.TrimSpace(string(out))
	if err != nil {
		// `--quiet` makes an unknown ref exit non-zero with no output, which
		// is how an unreported repo looks. Anything that also printed is a
		// real failure worth surfacing.
		if text == "" {
			return plumbing.ZeroHash, false, nil
		}
		return plumbing.ZeroHash, false, fmt.Errorf("resolve %s: %w: %s", ref, err, text)
	}
	if text == "" {
		return plumbing.ZeroHash, false, nil
	}
	return plumbing.NewHash(text), true, nil
}

// listTree returns the path and blob hash of every file reachable from ref,
// recursively. Paths are NUL-terminated so that a filename containing a
// newline or a quote cannot be misread; -z also stops git from quoting
// non-ASCII paths.
func listTree(repoPath, ref string) (paths []string, hashes []string, err error) {
	cmd := exec.Command("git", "ls-tree", "-r", "-z", ref)
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("list %s: %w: %s", ref, err, strings.TrimSpace(stderr.String()))
	}

	for _, entry := range strings.Split(string(out), "\x00") {
		if entry == "" {
			continue
		}
		// "<mode> SP <type> SP <object> TAB <path>"
		meta, path, found := strings.Cut(entry, "\t")
		if !found {
			return nil, nil, fmt.Errorf("list %s: malformed entry %q", ref, entry)
		}
		fields := strings.Fields(meta)
		if len(fields) != 3 {
			return nil, nil, fmt.Errorf("list %s: malformed entry %q", ref, entry)
		}
		if fields[1] != "blob" {
			continue // submodule or other non-file entry; events are all blobs
		}
		paths = append(paths, path)
		hashes = append(hashes, fields[2])
	}
	return paths, hashes, nil
}

// catFileBatch returns the contents of the given objects, in the order asked
// for. One `git cat-file --batch` process streams them all: an events ref can
// hold thousands of files, and a process per object would dominate the cost.
//
// In a partial clone this is also where any object still held only on the
// server is lazily fetched, which is git's job and not something a direct
// object-store read can do.
func catFileBatch(repoPath string, hashes []string) ([][]byte, error) {
	cmd := exec.Command("git", "cat-file", "--batch")
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdin = strings.NewReader(strings.Join(hashes, "\n") + "\n")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	out, readErr := readBatch(bufio.NewReaderSize(stdout, 64*1024), len(hashes))
	// Drain whatever is left so git never blocks writing into a full pipe,
	// which would deadlock Wait.
	_, _ = io.Copy(io.Discard, stdout)
	waitErr := cmd.Wait()

	// git's own diagnosis matters more than the parse failure it caused —
	// a refused lazy fetch, for instance, ends the stream mid-object and
	// reads back as a bare EOF without it.
	if detail := strings.TrimSpace(stderr.String()); detail != "" {
		if readErr != nil {
			return nil, fmt.Errorf("read objects: %w: %s", readErr, detail)
		}
		if waitErr != nil {
			return nil, fmt.Errorf("read objects: %w: %s", waitErr, detail)
		}
	}
	if readErr != nil {
		return nil, fmt.Errorf("read objects: %w", readErr)
	}
	if waitErr != nil {
		return nil, fmt.Errorf("read objects: %w", waitErr)
	}
	return out, nil
}

// readBatch parses `git cat-file --batch` output: for each object a header
// line "<sha> SP <type> SP <size>", then exactly <size> bytes, then a newline.
func readBatch(r *bufio.Reader, want int) ([][]byte, error) {
	out := make([][]byte, 0, want)
	for i := 0; i < want; i++ {
		header, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("object %d of %d: %w", i+1, want, err)
		}
		header = strings.TrimSuffix(header, "\n")

		fields := strings.Fields(header)
		if len(fields) == 2 && fields[1] == "missing" {
			// The hash came straight out of ls-tree, so the tree references
			// an object this repo cannot produce — held only on the server
			// with lazy fetching refused, or a genuinely damaged store.
			return nil, fmt.Errorf(
				"object %s is named by %s but cannot be read from this repository",
				fields[0], EventsRef)
		}
		if len(fields) != 3 {
			return nil, fmt.Errorf("unexpected response %q", header)
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("unexpected size in %q", header)
		}

		buf := make([]byte, size)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, fmt.Errorf("object %s: %w", fields[0], err)
		}
		if _, err := r.Discard(1); err != nil { // trailing newline
			return nil, fmt.Errorf("object %s: %w", fields[0], err)
		}
		out = append(out, buf)
	}
	return out, nil
}

// commitTree returns the tree a commit points at. ok is false when the commit
// cannot be resolved, which callers treat as "cannot tell" rather than as a
// failure.
func commitTree(repoPath string, commit plumbing.Hash) (plumbing.Hash, bool) {
	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", commit.String()+"^{tree}")
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.Output()
	if err != nil {
		return plumbing.ZeroHash, false
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return plumbing.ZeroHash, false
	}
	return plumbing.NewHash(text), true
}
