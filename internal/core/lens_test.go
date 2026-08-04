package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/core"
)

// TestLens_RunsDeriveViewOnEachSnapshot is the load-bearing test for the
// Lens contract: push a Snapshot through a fake Source, and confirm a fully
// derived View — Groups, Weekly, Header — comes out the other side. The
// derivation itself is covered by DeriveView's own tests; here we only
// assert that the Lens runs DeriveView per Snapshot and preserves the
// channel-streaming shape.
func TestLens_RunsDeriveViewOnEachSnapshot(t *testing.T) {
	commitTime := time.Date(2026, 5, 11, 9, 0, 0, 0, time.UTC)
	snap := core.Snapshot{Commits: []core.CommitView{
		{SHA: "head", Author: "alice", Subject: "wip", Time: commitTime.Add(time.Minute)},
		{SHA: "deployed", Author: "alice", Subject: "shipped", Time: commitTime,
			Events: []clarityrefs.Event{
				{Stage: "ci", Status: "passed", Time: commitTime.Add(10 * time.Minute)},
				{Stage: "deploy", Status: "passed", Time: commitTime.Add(time.Hour)},
			}},
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	src := newFakeSource()
	lens := core.NewLens(src, core.DefaultLeadTimeMode)
	views := lens.Views(ctx)

	go src.emit(snap)

	select {
	case got := <-views:
		if len(got.Groups.Head) != 1 || got.Groups.Head[0].SHA != "head" {
			t.Errorf("Head: expected ['head'], got %+v", got.Groups.Head)
		}
		if got.Header.CI != "passed" || got.Header.Deploy != "passed" {
			t.Errorf("Header: expected ci=deploy=passed, got %+v", got.Header)
		}
		if got.Snapshot.Commits[0].SHA != "head" {
			t.Errorf("Snapshot not threaded through: got %+v", got.Snapshot)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the lens to emit a view")
	}
}

// TestLens_ClosesViewsWhenSourceCloses is the other half of the Lens
// contract: shutting down the upstream Source must propagate through the
// Lens so the Renderer can exit cleanly. Without this, the renderer
// goroutine would block forever on a Source that's gone.
func TestLens_ClosesViewsWhenSourceCloses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	src := newFakeSource()
	lens := core.NewLens(src, core.DefaultLeadTimeMode)
	views := lens.Views(ctx)

	// Cancelling the context closes the fake source, which should propagate
	// through to the views channel.
	cancel()

	select {
	case _, ok := <-views:
		if ok {
			t.Fatal("expected views channel to be closed after source shut down")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for views channel to close")
	}
}
