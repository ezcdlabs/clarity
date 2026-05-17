package core_test

import (
	"context"
	"testing"

	"github.com/ezcdlabs/clarity/internal/core"
)

// --- Fake implementations used across core tests ----------------------------

// fakeSource is a programmable Source: tests push Snapshots onto its channel
// and the consumer reads them as if from a real adapter. Used by lens tests
// (next step) and as a sanity check that the Source interface compiles.
type fakeSource struct {
	out chan core.Snapshot
}

func newFakeSource() *fakeSource { return &fakeSource{out: make(chan core.Snapshot, 1)} }

func (f *fakeSource) Watch(ctx context.Context) <-chan core.Snapshot {
	// Real adapters should close the channel when ctx is cancelled. The fake
	// runs a tiny goroutine that closes on cancellation so tests can rely on
	// the same shutdown contract.
	go func() {
		<-ctx.Done()
		close(f.out)
	}()
	return f.out
}

func (f *fakeSource) emit(snap core.Snapshot) { f.out <- snap }

// fakeRenderer collects every View it sees into a slice. Used by composition
// tests in cmd/git-clarity later, and as a sanity check for the Renderer
// interface here.
type fakeRenderer struct {
	views []core.View
}

func (f *fakeRenderer) Render(ctx context.Context, views <-chan core.View) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case v, ok := <-views:
			if !ok {
				return nil
			}
			f.views = append(f.views, v)
		}
	}
}

// fakeWriter captures every Write call. Used by copy-events tests later, and
// as a sanity check for the EventWriter interface.
type fakeWriter struct {
	calls []core.Events
}

func (f *fakeWriter) Write(ctx context.Context, events core.Events) error {
	f.calls = append(f.calls, events)
	return nil
}

// --- Compile-time interface checks ------------------------------------------

// These declarations fail to compile if the fakes drift away from the port
// signatures. Cheap insurance against accidental refactors.
var (
	_ core.Source      = (*fakeSource)(nil)
	_ core.Renderer    = (*fakeRenderer)(nil)
	_ core.EventWriter = (*fakeWriter)(nil)
)

// --- Round-trip smoke checks ------------------------------------------------

func TestFakeSource_DeliversSnapshots(t *testing.T) {
	// Sanity: the fake Source can deliver a Snapshot through Watch's channel
	// — gives confidence the port's contract works for the obvious case.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	src := newFakeSource()
	out := src.Watch(ctx)

	go src.emit(core.Snapshot{Commits: []core.CommitView{{SHA: "abc"}}})

	got := <-out
	if len(got.Commits) != 1 || got.Commits[0].SHA != "abc" {
		t.Errorf("expected one Snapshot with SHA=abc, got %+v", got)
	}
}

func TestFakeRenderer_CollectsViews(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := &fakeRenderer{}
	views := make(chan core.View, 2)
	views <- core.View{}
	views <- core.View{}
	close(views)

	if err := r.Render(ctx, views); err != nil {
		t.Fatalf("Render returned err: %v", err)
	}
	if len(r.views) != 2 {
		t.Errorf("expected 2 views captured, got %d", len(r.views))
	}
}

func TestFakeWriter_CapturesWrites(t *testing.T) {
	ctx := context.Background()
	w := &fakeWriter{}
	if err := w.Write(ctx, core.Events{"abc": nil}); err != nil {
		t.Fatalf("Write returned err: %v", err)
	}
	if err := w.Write(ctx, core.Events{"def": nil}); err != nil {
		t.Fatalf("Write returned err: %v", err)
	}
	if len(w.calls) != 2 {
		t.Errorf("expected 2 Write calls captured, got %d", len(w.calls))
	}
}
