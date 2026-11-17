package core

import "context"

// MarkDegraded wraps a Source so every snapshot it emits carries the given
// Degraded reason (unless the inner source already set one of its own).
// It exists for fallbacks: a dashboard served by its fallback source must
// say so on every frame, because the one thing worse than degraded data
// is degraded data presented as the real thing.
func MarkDegraded(inner Source, reason string) Source {
	return &degradedSource{inner: inner, reason: reason}
}

type degradedSource struct {
	inner  Source
	reason string
}

func (d *degradedSource) Watch(ctx context.Context) <-chan Snapshot {
	out := make(chan Snapshot, 1)
	go func() {
		defer close(out)
		for snap := range d.inner.Watch(ctx) {
			if snap.Degraded == "" {
				snap.Degraded = d.reason
			}
			select {
			case out <- snap:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
