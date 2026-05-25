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
}

// NewLens returns a Lens that derives views from the given Source.
func NewLens(source Source) *Lens {
	return &Lens{source: source}
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
			case out <- DeriveView(snap):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
