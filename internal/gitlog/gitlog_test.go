package gitlog_test

import (
	"fmt"
	"testing"

	"github.com/ezcdlabs/clarity/internal/gitlog"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// TestWalk_ReportsWhetherCommitsRemainBeyondTheLimit covers the signal the
// renderers need to tell "this is the whole history" apart from "this is
// where your --limit cut it off".
//
// Walk cannot answer that from the returned slice alone: a repo with exactly
// `limit` commits and a repo with a thousand more below the cut both return
// `limit` commits. So Walk looks one commit past the limit and reports what
// it saw, without carrying that extra commit into the snapshot.
func TestWalk_ReportsWhetherCommitsRemainBeyondTheLimit(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	// NewClone starts with one commit already on the branch; add four more.
	for i := 0; i < 4; i++ {
		clone.WriteFile(fmt.Sprintf("file%d.txt", i), "x")
		clone.CommitAll(fmt.Sprintf("commit %d", i))
	}
	clone.Push("main")

	total := len(clone.LogBranch("main"))
	if total != 5 {
		t.Fatalf("fixture should have 5 commits, got %d", total)
	}

	cases := []struct {
		name      string
		limit     int
		wantCount int
		wantMore  bool
	}{
		{name: "limit cuts the history short", limit: 3, wantCount: 3, wantMore: true},
		{name: "limit lands exactly on the last commit", limit: 5, wantCount: 5, wantMore: false},
		{name: "limit is larger than the history", limit: 10, wantCount: 5, wantMore: false},
		{name: "limit of one", limit: 1, wantCount: 1, wantMore: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			commits, more, err := gitlog.Walk(clone.Path, "main", c.limit)
			if err != nil {
				t.Fatalf("Walk: %v", err)
			}
			if len(commits) != c.wantCount {
				t.Errorf("got %d commits, want %d", len(commits), c.wantCount)
			}
			if more != c.wantMore {
				t.Errorf("more = %v, want %v", more, c.wantMore)
			}
		})
	}
}

// TestResolveLimit covers the fallback every caller has to agree on. Walk
// silently substitutes DefaultLimit for a non-positive limit, so an adapter
// that stores the raw value ends up reporting a cap that isn't the one the
// walk applied — and the renderer's notice then names a limit that never
// existed ("--limit 0 reached"). Resolve is that substitution, exported so
// callers can record the same number the walk used.
func TestResolveLimit(t *testing.T) {
	cases := []struct {
		limit int
		want  int
	}{
		{limit: 100, want: 100},
		{limit: 1, want: 1},
		{limit: 0, want: gitlog.DefaultLimit},
		{limit: -1, want: gitlog.DefaultLimit},
	}
	for _, c := range cases {
		if got := gitlog.Resolve(c.limit); got != c.want {
			t.Errorf("Resolve(%d) = %d, want %d", c.limit, got, c.want)
		}
	}
}
