// Package clarityrefs is the public Go API for clarity's events ref
// (refs/clarity/events). It exposes the Event type plus read and write
// operations against an on-disk git repository.
package clarityrefs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/ezcdlabs/clarity/internal/gitenv"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// EventsRef is the canonical ref under which clarity stores per-commit
// pipeline events.
const EventsRef = "refs/clarity/events"

// Event is a single pipeline event for a commit. Stage, Status and Time are
// the stable core schema. CI is opportunistic metadata captured from the
// environment that produced the event; it may be empty.
type Event struct {
	Stage  string
	Status string
	Time   time.Time
	CI     map[string]string
}

// eventJSON is the on-disk shape (timestamp serialised as Unix seconds).
type eventJSON struct {
	Stage  string            `json:"stage"`
	Status string            `json:"status"`
	Ts     int64             `json:"ts"`
	CI     map[string]string `json:"ci,omitempty"`
}

func (e Event) marshal() ([]byte, error) {
	return json.Marshal(eventJSON{
		Stage:  e.Stage,
		Status: e.Status,
		Ts:     e.Time.Unix(),
		CI:     e.CI,
	})
}

func unmarshalEvent(data []byte) (Event, error) {
	var ej eventJSON
	if err := json.Unmarshal(data, &ej); err != nil {
		return Event{}, err
	}
	return Event{
		Stage:  ej.Stage,
		Status: ej.Status,
		Time:   time.Unix(ej.Ts, 0),
		CI:     ej.CI,
	}, nil
}

// ReadEvents returns events for a single commit SHA, sorted by ascending
// timestamp. Reads only the local events ref; callers (typically the watcher)
// are responsible for fetching first. Returns an empty slice when the events
// ref is not yet present locally.
func ReadEvents(repoPath, sha string) ([]Event, error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, err
	}
	tree, err := loadEventsTree(repo)
	if err != nil || tree == nil {
		return nil, err
	}
	prefix := "events/" + sha + "/"
	var events []Event
	err = tree.Files().ForEach(func(f *object.File) error {
		if !strings.HasPrefix(f.Name, prefix) || !strings.HasSuffix(f.Name, ".json") {
			return nil
		}
		content, err := f.Contents()
		if err != nil {
			return err
		}
		ev, err := unmarshalEvent([]byte(content))
		if err != nil {
			return err
		}
		events = append(events, ev)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Time.Before(events[j].Time) })
	return events, nil
}

// ReadAllEvents returns events for every commit that has any, keyed by SHA.
// Each per-SHA slice is sorted by ascending timestamp.
func ReadAllEvents(repoPath string) (map[string][]Event, error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]Event)
	tree, err := loadEventsTree(repo)
	if err != nil || tree == nil {
		return out, err
	}
	err = tree.Files().ForEach(func(f *object.File) error {
		if !strings.HasPrefix(f.Name, "events/") || !strings.HasSuffix(f.Name, ".json") {
			return nil
		}
		rest := strings.TrimPrefix(f.Name, "events/")
		slash := strings.IndexByte(rest, '/')
		if slash < 0 {
			return nil
		}
		sha := rest[:slash]
		content, err := f.Contents()
		if err != nil {
			return err
		}
		ev, err := unmarshalEvent([]byte(content))
		if err != nil {
			return err
		}
		out[sha] = append(out[sha], ev)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for sha, evs := range out {
		sort.SliceStable(evs, func(i, j int) bool { return evs[i].Time.Before(evs[j].Time) })
		out[sha] = evs
	}
	return out, nil
}

// WriteEvent appends a single event for sha to the events ref and pushes it
// to remote. If two writers race the loser fetches the winner's commit and
// retries — the unique per-event filename guarantees no event is lost.
func WriteEvent(repoPath, remote, sha string, event Event) error {
	if remote == "" {
		remote = "origin"
	}
	suffix, err := shortID()
	if err != nil {
		return fmt.Errorf("generate event id: %w", err)
	}
	filename := fmt.Sprintf("%d-%s.json", event.Time.Unix(), suffix)
	data, err := event.marshal()
	if err != nil {
		return err
	}
	filePath := "events/" + sha + "/" + filename
	message := fmt.Sprintf("report: %s %s %s", sha, event.Stage, event.Status)

	return updateEventsRef(repoPath, remote, func(files map[string][]byte) {
		files[filePath] = data
	}, message)
}

// --- internal: optimistic push loop ------------------------------------------

func updateEventsRef(repoPath, remote string, mutate func(map[string][]byte), message string) error {
	defer deleteLocalEventsRef(repoPath)
	for {
		_ = fetchEventsRef(repoPath, remote) // may not exist yet; that's fine

		repo, err := gogit.PlainOpen(repoPath)
		if err != nil {
			return fmt.Errorf("open repo: %w", err)
		}

		files, parentHash, err := readEventsRefFiles(repo)
		if err != nil {
			return fmt.Errorf("read events ref: %w", err)
		}

		mutate(files)

		treeHash, err := buildTree(repo, files)
		if err != nil {
			return fmt.Errorf("build tree: %w", err)
		}

		sig := &object.Signature{Name: "clarity", Email: "clarity@local", When: time.Now()}
		commit := &object.Commit{
			Author:    *sig,
			Committer: *sig,
			Message:   message,
			TreeHash:  treeHash,
		}
		if parentHash != plumbing.ZeroHash {
			commit.ParentHashes = []plumbing.Hash{parentHash}
		}

		enc := repo.Storer.NewEncodedObject()
		if err := commit.Encode(enc); err != nil {
			return fmt.Errorf("encode commit: %w", err)
		}
		commitHash, err := repo.Storer.SetEncodedObject(enc)
		if err != nil {
			return fmt.Errorf("store commit: %w", err)
		}

		ref := plumbing.NewHashReference(plumbing.ReferenceName(EventsRef), commitHash)
		if err := repo.Storer.SetReference(ref); err != nil {
			return fmt.Errorf("set local ref: %w", err)
		}

		pushErr := pushEventsRef(repoPath, remote)
		if pushErr == nil {
			return nil
		}
		if isFastForwardRejected(pushErr) {
			continue
		}
		if isBrokenObjectError(pushErr) {
			_ = deleteLocalEventsRef(repoPath)
			continue
		}
		return fmt.Errorf("push events ref: %w", pushErr)
	}
}

func loadEventsTree(repo *gogit.Repository) (*object.Tree, error) {
	ref, err := repo.Reference(plumbing.ReferenceName(EventsRef), true)
	if err != nil {
		return nil, nil // no events ref yet
	}
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}
	return commit.Tree()
}

func readEventsRefFiles(repo *gogit.Repository) (map[string][]byte, plumbing.Hash, error) {
	files := make(map[string][]byte)
	ref, err := repo.Reference(plumbing.ReferenceName(EventsRef), true)
	if err != nil {
		return files, plumbing.ZeroHash, nil
	}
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, plumbing.ZeroHash, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, plumbing.ZeroHash, err
	}
	err = tree.Files().ForEach(func(f *object.File) error {
		content, err := f.Contents()
		if err != nil {
			return err
		}
		files[f.Name] = []byte(content)
		return nil
	})
	if err != nil {
		return nil, plumbing.ZeroHash, err
	}
	return files, ref.Hash(), nil
}

// buildTree creates the blob objects and constructs a fully nested git tree.
// Per-directory subtrees (rather than flat entries with slashes in their name)
// are required to pass GitHub's receive.fsckObjects "fullPathname" check.
func buildTree(repo *gogit.Repository, files map[string][]byte) (plumbing.Hash, error) {
	blobs := make(map[string]plumbing.Hash, len(files))
	for p, content := range files {
		enc := repo.Storer.NewEncodedObject()
		enc.SetType(plumbing.BlobObject)
		w, err := enc.Writer()
		if err != nil {
			return plumbing.ZeroHash, err
		}
		if _, err := w.Write(content); err != nil {
			return plumbing.ZeroHash, err
		}
		w.Close()
		h, err := repo.Storer.SetEncodedObject(enc)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		blobs[p] = h
	}
	return buildNestedTree(repo, blobs, "")
}

func buildNestedTree(repo *gogit.Repository, blobs map[string]plumbing.Hash, prefix string) (plumbing.Hash, error) {
	dirs := make(map[string]struct{})
	var entries []object.TreeEntry

	for p, h := range blobs {
		rel := p
		if prefix != "" {
			if !strings.HasPrefix(p, prefix+"/") {
				continue
			}
			rel = p[len(prefix)+1:]
		}
		if idx := strings.IndexByte(rel, '/'); idx >= 0 {
			dirs[rel[:idx]] = struct{}{}
		} else {
			entries = append(entries, object.TreeEntry{
				Name: rel,
				Mode: 0100644,
				Hash: h,
			})
		}
	}

	for dir := range dirs {
		sub := dir
		if prefix != "" {
			sub = prefix + "/" + dir
		}
		h, err := buildNestedTree(repo, blobs, sub)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		entries = append(entries, object.TreeEntry{
			Name: dir,
			Mode: 0040000,
			Hash: h,
		})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	tree := &object.Tree{Entries: entries}
	enc := repo.Storer.NewEncodedObject()
	if err := tree.Encode(enc); err != nil {
		return plumbing.ZeroHash, err
	}
	return repo.Storer.SetEncodedObject(enc)
}

func fetchEventsRef(repoPath, remote string) error {
	cmd := exec.Command("git", "fetch", remote, "+"+EventsRef+":"+EventsRef)
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := string(out)
		if strings.Contains(msg, "couldn't find remote ref") {
			return nil
		}
		return fmt.Errorf("fetch events ref: %w\n%s", err, msg)
	}
	return nil
}

func pushEventsRef(repoPath, remote string) error {
	// --no-verify skips the user's pre-push hook. The events ref is internal
	// bookkeeping (event JSON files keyed by commit SHA), not user code, so
	// a hook gating real code pushes — tests, linters, etc. — has no business
	// inspecting it and shouldn't be able to block clarity from recording an
	// event.
	cmd := exec.Command("git", "push", "--no-verify", remote, EventsRef+":"+EventsRef)
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}
	return nil
}

func deleteLocalEventsRef(repoPath string) error {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return err
	}
	return repo.Storer.RemoveReference(plumbing.ReferenceName(EventsRef))
}

func isFastForwardRejected(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gogit.ErrNonFastForwardUpdate) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "non-fast-forward") ||
		strings.Contains(msg, "failed to update ref") ||
		strings.Contains(msg, "reference already exists") ||
		strings.Contains(msg, "incorrect old value provided") ||
		strings.Contains(msg, "[rejected]")
}

func isBrokenObjectError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "fullPathname") ||
		strings.Contains(msg, "fsck error")
}

func shortID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
