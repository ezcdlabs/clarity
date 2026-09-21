package clarityrefs

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
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
	// Unix nanoseconds, unlike an event's seconds. Candidacy supersedes by
	// timestamp — a later record corrects an earlier one — so the resolution
	// has to be finer than the rate records can be written at. At second
	// resolution two reports in the same second tie, and a tie has no honest
	// winner.
	Ts int64 `json:"ts"`
}

func (s Scope) marshal() ([]byte, error) {
	affected := s.Affected
	return json.Marshal(scopeJSON{Target: s.Target, Affected: &affected, Ts: s.Time.UnixNano()})
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
	return Scope{Target: sj.Target, Affected: *sj.Affected, Time: time.Unix(0, sj.Ts)}, nil
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
	files, _, err := readEventsFiles(repoPath)
	if err != nil {
		return nil, err
	}

	out := map[string][]Scope{}
	for name, content := range files {
		if !strings.HasPrefix(name, ScopePrefix) || !strings.HasSuffix(name, ".json") {
			continue
		}
		rest := strings.TrimPrefix(name, ScopePrefix)
		sha, _, ok := strings.Cut(rest, "/")
		if !ok {
			continue
		}
		s, err := unmarshalScope(content)
		if err != nil {
			// A record this build can't parse is skipped rather than fatal:
			// one unreadable file must not blank out a repo's whole view.
			continue
		}
		out[sha] = append(out[sha], s)
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

// ReadAllRef reads both trees in one pass: pipeline events and candidacy
// records, each keyed by commit SHA.
//
// The polling loop needs both on every tick, and reading them separately means
// two PlainOpen calls and two full walks of a tree that can hold thousands of
// files — roughly doubling the cost of a snapshot for every repo, including
// the ones that report no candidacy at all.
func ReadAllRef(repoPath string) (map[string][]Event, map[string][]Scope, error) {
	files, _, err := readEventsFiles(repoPath)
	if err != nil {
		return nil, nil, err
	}

	events := map[string][]Event{}
	scope := map[string][]Scope{}
	for name, content := range files {
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		switch {
		case strings.HasPrefix(name, "events/"):
			sha, ok := shaFromPath(name, "events/")
			if !ok {
				continue
			}
			e, err := unmarshalEvent(content)
			if err != nil {
				continue
			}
			events[sha] = append(events[sha], e)
		case strings.HasPrefix(name, ScopePrefix):
			sha, ok := shaFromPath(name, ScopePrefix)
			if !ok {
				continue
			}
			s, err := unmarshalScope(content)
			if err != nil {
				continue
			}
			scope[sha] = append(scope[sha], s)
		}
	}

	sortEventsByTime(events)
	sortScopeByTime(scope)
	return events, scope, nil
}

func shaFromPath(name, prefix string) (string, bool) {
	sha, _, ok := strings.Cut(strings.TrimPrefix(name, prefix), "/")
	return sha, ok
}

func sortScopeByTime(m map[string][]Scope) {
	for sha := range m {
		records := m[sha]
		sort.SliceStable(records, func(i, j int) bool {
			return records[i].Time.Before(records[j].Time)
		})
		m[sha] = records
	}
}

func sortEventsByTime(m map[string][]Event) {
	for sha := range m {
		evs := m[sha]
		sort.SliceStable(evs, func(i, j int) bool { return evs[i].Time.Before(evs[j].Time) })
		m[sha] = evs
	}
}
