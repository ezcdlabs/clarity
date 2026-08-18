// Package clarityrefs is the public Go API for clarity's events ref
// (refs/clarity/events). It exposes the Event type plus read and write
// operations against an on-disk git repository.
package clarityrefs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ezcdlabs/clarity/internal/gitenv"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// EventsRef is the canonical ref under which clarity stores per-commit
// pipeline events.
const EventsRef = "refs/clarity/events"

// maxReadRetries bounds how many times updateEventsRef re-fetches after a
// transient "packfile not found" read error before giving up, so a genuinely
// unreadable repo fails fast instead of looping forever.
const maxReadRetries = 3

// maxWriteRetries bounds how many times updateEventsRef rebuilds and re-pushes
// after a concurrent gc pruned the objects it had just written, so a repo with
// a gc running continuously fails with the real error instead of looping.
const maxWriteRetries = 3

// noAutoGC disables git's background gc/maintenance for a single invocation.
// A `git fetch` otherwise spawns a detached `gc --auto` that can repack and
// delete packfiles out from under the go-git read that immediately follows,
// surfacing as dotgit.ErrPackfileNotFound. Prepended to every git command we
// run so our own fetch/push never triggers the racing repack.
var noAutoGC = []string{"-c", "gc.auto=0", "-c", "maintenance.auto=false"}

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
	data, err := event.marshal()
	if err != nil {
		return err
	}
	filename := fmt.Sprintf("%d-%s.json", event.Time.Unix(), contentHash(data))
	filePath := "events/" + sha + "/" + filename
	message := fmt.Sprintf("report: %s %s %s", sha, event.Stage, event.Status)

	return updateEventsRef(repoPath, remote, func(files map[string][]byte) {
		files[filePath] = data
	}, message)
}

// WriteEvents appends a batch of events to the events ref in ONE commit and
// ONE push, amortising the fetch/push round-trip across the whole batch.
// Intended for backfill / migration paths where N events are known up-front;
// live reporting should keep using WriteEvent so each event lands as its own
// audit-able commit. A nil or empty batch is a no-op.
func WriteEvents(repoPath, remote string, eventsBySHA map[string][]Event) error {
	if len(eventsBySHA) == 0 {
		return nil
	}
	if remote == "" {
		remote = "origin"
	}

	type prepared struct {
		path string
		data []byte
	}
	var prep []prepared
	total := 0
	for sha, events := range eventsBySHA {
		for _, ev := range events {
			data, err := ev.marshal()
			if err != nil {
				return err
			}
			filename := fmt.Sprintf("%d-%s.json", ev.Time.Unix(), contentHash(data))
			prep = append(prep, prepared{
				path: "events/" + sha + "/" + filename,
				data: data,
			})
			total++
		}
	}

	message := fmt.Sprintf("report: batch of %d events", total)
	return updateEventsRef(repoPath, remote, func(files map[string][]byte) {
		for _, p := range prep {
			files[p.path] = p.data
		}
	}, message)
}

// --- internal: optimistic push loop ------------------------------------------

func updateEventsRef(repoPath, remote string, mutate func(map[string][]byte), message string) error {
	defer deleteLocalEventsRef(repoPath)
	readRetries := 0
	writeRetries := 0
	for {
		_ = fetchEventsRef(repoPath, remote) // may not exist yet; that's fine

		repo, err := gogit.PlainOpen(repoPath)
		if err != nil {
			return fmt.Errorf("open repo: %w", err)
		}

		files, parentHash, err := readEventsRefFiles(repo)
		if err != nil {
			// A concurrent git gc/repack can delete a packfile out from under
			// go-git mid-read ("packfile not found"). The objects are still on
			// the remote, so drop the stale local ref and re-fetch into a
			// freshly-packed object store rather than failing the report.
			if isPackfileNotFound(err) && readRetries < maxReadRetries {
				readRetries++
				_ = deleteLocalEventsRef(repoPath)
				continue
			}
			return fmt.Errorf("read events ref: %w", err)
		}

		mutate(files)

		st := writeStorer(repo)
		treeHash, err := buildTree(st, files)
		if err != nil {
			return fmt.Errorf("build tree: %w", err)
		}

		// Idempotency short-circuit: when content-addressed filenames produce
		// a tree identical to the parent's, the caller's events are already
		// recorded. Skip the no-op commit + push so a re-run is truly free.
		if parentHash != plumbing.ZeroHash {
			if parent, err := repo.CommitObject(parentHash); err == nil && parent.TreeHash == treeHash {
				return nil
			}
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

		enc := st.NewEncodedObject()
		if err := commit.Encode(enc); err != nil {
			return fmt.Errorf("encode commit: %w", err)
		}
		commitHash, err := storeObject(st, enc)
		if err != nil {
			return fmt.Errorf("store commit: %w", err)
		}

		ref := plumbing.NewHashReference(plumbing.ReferenceName(EventsRef), commitHash)
		if err := repo.Storer.SetReference(ref); err != nil {
			return fmt.Errorf("set local ref: %w", err)
		}

		pushErr := pushEvents(repoPath, remote)
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
		// An aggressive concurrent gc can prune the objects this iteration
		// wrote before the ref moved to make them reachable, leaving the push
		// unable to read what it is packing. Drop the local ref so the next
		// pass re-fetches a clean store, and rebuild — the tree comes from the
		// caller's events, so every pruned object is written again. Bounded:
		// if a gc keeps collecting under us, surface the error rather than
		// loop.
		if isMissingLocalObject(pushErr) && writeRetries < maxWriteRetries {
			writeRetries++
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
func buildTree(st storer.EncodedObjectStorer, files map[string][]byte) (plumbing.Hash, error) {
	blobs := make(map[string]plumbing.Hash, len(files))
	for p, content := range files {
		enc := st.NewEncodedObject()
		enc.SetType(plumbing.BlobObject)
		w, err := enc.Writer()
		if err != nil {
			return plumbing.ZeroHash, err
		}
		if _, err := w.Write(content); err != nil {
			return plumbing.ZeroHash, err
		}
		w.Close()
		h, err := storeObject(st, enc)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		blobs[p] = h
	}
	return buildNestedTree(st, blobs, "")
}

func buildNestedTree(st storer.EncodedObjectStorer, blobs map[string]plumbing.Hash, prefix string) (plumbing.Hash, error) {
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
		h, err := buildNestedTree(st, blobs, sub)
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
	enc := st.NewEncodedObject()
	if err := tree.Encode(enc); err != nil {
		return plumbing.ZeroHash, err
	}
	return storeObject(st, enc)
}

func fetchEventsRef(repoPath, remote string) error {
	cmd := exec.Command("git", append(noAutoGC, "fetch", remote, "+"+EventsRef+":"+EventsRef)...)
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

// pushEvents is the push step of the write loop. A var so tests can empty the
// object store between the tree build and the push, reproducing what an
// aggressive concurrent gc does to objects this write has not yet made
// reachable.
var pushEvents = pushEventsRef

func pushEventsRef(repoPath, remote string) error {
	// --no-verify skips the user's pre-push hook. The events ref is internal
	// bookkeeping (event JSON files keyed by commit SHA), not user code, so
	// a hook gating real code pushes — tests, linters, etc. — has no business
	// inspecting it and shouldn't be able to block clarity from recording an
	// event.
	cmd := exec.Command("git", append(noAutoGC, "push", "--no-verify", remote, EventsRef+":"+EventsRef)...)
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

// isFastForwardRejected reports whether a push failed because another writer
// got there first, which the retry loop recovers from by re-fetching and
// replaying the commit on the new tip.
//
// The rejection has two shapes. When the client can see it is behind from the
// ref advertisement it declines locally and prints "[rejected] ... (fetch
// first)". When two pushes are in flight at once neither client knows it is
// behind, so the loser's compare-and-swap is refused by the server:
//
//	! [remote rejected] refs/clarity/events -> refs/clarity/events
//	  (cannot lock ref 'refs/clarity/events': is at a9f4824... but expected 1668810...)
//
// Note the "remote" inside the brackets: matching "[rejected]" does not see
// that line, which is why "cannot lock ref" is matched in its own right. The
// match stays on the lock failure rather than on "[remote rejected]" — a
// remote fsck rejection is worded the same way and needs the local ref
// dropped, which this recovery does not do.
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
		strings.Contains(msg, "cannot lock ref") ||
		strings.Contains(msg, "[rejected]")
}

// isPackfileNotFound reports whether err is go-git's transient
// dotgit.ErrPackfileNotFound. It surfaces when a concurrent git gc/repack
// deletes a .pack out from under an in-progress read: go-git has already
// recorded the object's pack from its .idx, then fails to open the now-removed
// .pack. The objects remain reachable on the remote, so the read is retried
// after re-fetching into a freshly-packed object store.
func isPackfileNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "packfile not found")
}

// missingLocalObjectRe matches git's diagnostics for an object it cannot read
// out of the local store, keyed on the shape they share: a message naming a
// raw object id. The wording varies by object type — "unable to read <id>"
// for a blob, "bad tree object <id>" for a tree, "bad object <id>" for a
// commit — and none of it mentions gc. Requiring the object id keeps
// unrelated "unable to read ..." diagnostics out.
var missingLocalObjectRe = regexp.MustCompile(`(?:bad object|bad tree object|unable to read) [0-9a-f]{40,64}\b`)

// isMissingLocalObject reports whether a push failed because objects the
// commit references are no longer in the local object store.
//
// The objects a write creates are unreachable until the events ref is moved
// to point at them. A gc pruning aggressively enough to collect objects that
// young — `--prune=now`, or a configured gc.pruneExpire; auto-gc's two-week
// default never does — deletes them in that window, and the push then fails
// locally while packing what it can no longer read.
//
// Recovery is to drop the local ref, re-fetch and rebuild: the tree is
// derived from the events map the caller supplied, so every pruned object is
// simply written again. Retrying the object write cannot help here — by the
// time the push runs, the objects are already gone.
func isMissingLocalObject(err error) bool {
	if err == nil {
		return false
	}
	return missingLocalObjectRe.MatchString(err.Error())
}

func isBrokenObjectError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "fullPathname") ||
		strings.Contains(msg, "fsck error")
}

// contentHash returns a short, deterministic suffix derived from the event's
// marshaled JSON. Same event content → same suffix, so two writes of the
// identical event collapse into one tree entry rather than two random files.
// That's what makes a backfill re-run idempotent: filenames depend only on
// the (sha, stage, status, time, ci) content the caller supplied.
func contentHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:4])
}
