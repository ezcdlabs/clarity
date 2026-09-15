package core

import "context"

// Lens is the bridge between a Source (which emits Snapshots) and the
// Renderer port (which consumes Views). Per snapshot the Lens runs the
// pure DeriveView transformation and forwards the result on its own
// channel; it does no I/O, no caching, and no buffering beyond one slot.
// Callers wrap it with CachedLens (step 5) for stale-while-revalidate
// startup.
type Lens struct {
	source Source
	mode   LeadTimeMode
	flows  []Flow
}

// NewLens returns a Lens that derives views from the given Source under the
// given LeadTimeMode. An empty mode means DefaultLeadTimeMode, so callers
// that don't configure lead time can pass the zero value.
// flows are the declared deploy flows from .ezcd.json; nil means discover them
// from the events. It is a constructor parameter rather than an optional
// setter on purpose: `clarity.leadTime` once shipped doing nothing because a
// configured value never reached the layer that used it, and a builder method
// is exactly the shape that lets a call site forget.
func NewLens(source Source, mode LeadTimeMode, flows []Flow) *Lens {
	if mode == "" {
		mode = DefaultLeadTimeMode
	}
	return &Lens{source: source, mode: mode, flows: flows}
}

// Views starts the upstream Source and returns a channel of derived
// Views. Closes when the Source's channel closes (i.e. when ctx is
// cancelled by the caller).
func (l *Lens) Views(ctx context.Context) <-chan View {
	out := make(chan View)
	go func() {
		defer close(out)
		for snap := range l.source.Watch(ctx) {
			select {
			case out <- DeriveView(snap, l.mode, l.flows):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
