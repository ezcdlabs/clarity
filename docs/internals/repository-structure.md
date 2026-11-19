# Repository Structure

```
ezcdlabs/clarity/
├── cmd/git-clarity/      # main binary (named git-clarity for git extension discovery)
├── clarityrefs/          # public package: read/write clarity's events
├── internal/
│   ├── adapters/tui/     # terminal UI rendering
│   ├── report/           # the `report` subcommand logic
│   ├── refs/             # fetch refspec configuration
│   └── ci/               # opportunistic CI env var detection
├── proto/                # the presentation model the mobile apps render
├── mobile/               # go-git source, FFI surface, and the native apps
└── go.mod
```

The `clarityrefs` package is the public Go API for anyone (third-party tooling, future EzcdLabs projects) who wants to read clarity's events:

```go
package clarityrefs

const EventsRef = "refs/clarity/events"

type Event struct {
    Stage  string
    Status string
    Time   time.Time
    Target string             // deploy only; "" is the untargeted deploy
    CI     map[string]string  // optional, may be empty
}

func ReadEvents(repoPath, sha string) ([]Event, error)
func ReadAllEvents(repoPath string) (map[string][]Event, error)
func WriteEvent(repoPath, remote, sha string, event Event) error

// Candidacy lives in the sibling scope/ tree, not in events/.
type Scope struct {
    Target   string
    Affected bool
    Time     time.Time
}

func ReadAllScope(repoPath string) (map[string][]Scope, error)
func WriteScope(repoPath, remote, sha string, scope Scope) error
```

Reads operate on the local events ref only — callers (typically the watcher) fetch first, via `FetchEventsRef`, which brings the complete object graph (see "Clone shapes and the object store"). `WriteEvent` fetches the remote ref before writing and pushes after, retrying on fast-forward rejection so concurrent reporters never lose events. Repository handles are passed as paths rather than `*git.Repository` so callers don't have to depend on a specific go-git version.

This mirrors the pattern pushq uses with its `pushqrefs` package — the ref format is the public contract, exposed via a Go package.
