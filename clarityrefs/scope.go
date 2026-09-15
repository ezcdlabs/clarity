package clarityrefs

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ScopePrefix is the sibling tree candidacy records live under, alongside
// events/ on the same ref.
//
// A separate tree rather than a new event kind, because candidacy is not a
// pipeline event: nothing was attempted, and the record carries no stage and
// no status. It also means every binary released before candidacy existed
// never sees these files — both event readers filter on the events/ prefix —
// so adopting it can't make an older clarity render a phantom stage.
const ScopePrefix = "scope/"

// Scope is one commit's candidacy for one deploy target: whether the commit
// contains any change that target ships.
//
// It exists to keep a flow's lead time honest. Flows share one commit list, so
// without it an Android deploy weeks later sweeps up hundreds of web-only
// commits and the DORA number becomes the average age of the monorepo rather
// than how long Android changes take to ship.
type Scope struct {
	Target   string
	Affected bool
	Time     time.Time
}

// scopeJSON is the on-disk shape. `affected` is never omitted: false means
// "CI said this commit doesn't touch that target", which is a different answer
// from the absence of any record, and they produce different lead times.
type scopeJSON struct {
	Target string `json:"target"`
	// A pointer, so the reader can tell false from absent. The whole point of
	// the field is that "CI says this commit doesn't touch that target" and
	// "nobody has said anything" are different answers producing different
	// lead times — and with a plain bool a truncated or foreign record
	// degrades silently into the first, which is the more consequential one.
	Affected *bool `json:"affected"`
	Ts       int64 `json:"ts"`
}

func (s Scope) marshal() ([]byte, error) {
	affected := s.Affected
	return json.Marshal(scopeJSON{Target: s.Target, Affected: &affected, Ts: s.Time.Unix()})
}

func unmarshalScope(data []byte) (Scope, error) {
	var sj scopeJSON
	if err := json.Unmarshal(data, &sj); err != nil {
		return Scope{}, err
	}
	// Every field is required. An event JSON happens to be structurally valid
	// scope JSON, so without these checks a record that strayed into the wrong
	// tree would read back as a confident "unaffected" for the untargeted
	// deploy rather than as the garbage it is.
	if sj.Affected == nil {
		return Scope{}, fmt.Errorf("scope record has no \"affected\" field")
	}
	if sj.Target == "" {
		return Scope{}, fmt.Errorf("scope record has no \"target\" field")
	}
	return Scope{Target: sj.Target, Affected: *sj.Affected, Time: time.Unix(sj.Ts, 0)}, nil
}

// WriteScope records one commit's candidacy for one target, using the same
// optimistic push loop the event writer uses.
func WriteScope(repoPath, remote, sha string, scope Scope) error {
	// An empty target is the untargeted deploy by the Event convention, so a
	// forgotten field would record a real claim about a real flow rather than
	// failing. Candidacy has to be about something.
	if scope.Target == "" {
		return fmt.Errorf("scope: target is required")
	}
	if remote == "" {
		remote = "origin"
	}
	data, err := scope.marshal()
	if err != nil {
		return err
	}
	verb := "unaffected"
	if scope.Affected {
		verb = "affected"
	}
	path := ScopePrefix + sha + "/" + fmt.Sprintf("%d-%s.json", scope.Time.Unix(), contentHash(data))
	message := fmt.Sprintf("report: %s %s %s", sha, verb, scope.Target)

	return updateEventsRef(repoPath, remote, func(files map[string][]byte) {
		files[path] = data
	}, message)
}

// ReadAllScope returns every candidacy record on the ref, keyed by commit SHA.
// Reads the local ref only; callers fetch first.
func ReadAllScope(repoPath string) (map[string][]Scope, error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, err
	}
	tree, err := loadEventsTree(repo)
	if err != nil || tree == nil {
		return map[string][]Scope{}, err
	}

	out := map[string][]Scope{}
	err = tree.Files().ForEach(func(f *object.File) error {
		if !strings.HasPrefix(f.Name, ScopePrefix) || !strings.HasSuffix(f.Name, ".json") {
			return nil
		}
		rest := strings.TrimPrefix(f.Name, ScopePrefix)
		sha, _, ok := strings.Cut(rest, "/")
		if !ok {
			return nil
		}
		content, err := f.Contents()
		if err != nil {
			return err
		}
		s, err := unmarshalScope([]byte(content))
		if err != nil {
			// A record this build can't parse is skipped rather than fatal:
			// one unreadable file must not blank out a repo's whole view.
			return nil
		}
		out[sha] = append(out[sha], s)
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Ascending by time, matching the event readers. A consumer resolving
	// "the latest record wins" would otherwise be reading git tree order,
	// which is lexicographic on the filename rather than chronological.
	for sha := range out {
		records := out[sha]
		sort.SliceStable(records, func(i, j int) bool {
			return records[i].Time.Before(records[j].Time)
		})
		out[sha] = records
	}
	return out, nil
}
