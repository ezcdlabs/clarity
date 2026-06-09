package refsource

import (
	"fmt"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
	"github.com/ezcdlabs/clarity/internal/gitlog"
)

// BuildSnapshot walks up to limit commits starting from the given branch
// and joins each one to its events from the local clarity events ref. Pure
// in the sense that it never touches the remote — callers (Source.Watch)
// are responsible for fetching first.
func BuildSnapshot(repoPath, branch string, limit int) (core.Snapshot, error) {
	commits, err := gitlog.Walk(repoPath, branch, limit)
	if err != nil {
		return core.Snapshot{}, err
	}
	eventsByCommit, err := clarityrefs.ReadAllEvents(repoPath)
	if err != nil {
		return core.Snapshot{}, fmt.Errorf("read events: %w", err)
	}
	return core.BuildSnapshot(commits, core.Events(eventsByCommit)), nil
}
