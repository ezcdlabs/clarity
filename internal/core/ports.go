package core

import "context"

// Source is the inbound port for pipeline data. Each Source adapter owns
// its own polling cadence, fetch verb, and snapshot-build logic — it polls
// whatever underlying system (a git ref, the GitHub Actions API, etc.),
// joins commits with their events via BuildSnapshot, and emits Snapshots
// on its returned channel whenever it detects a change.
//
// The Lens calls Watch exactly once per invocation and consumes from the
// channel until it's closed. Adapters MUST close the channel when ctx is
// cancelled so the Lens can shut down cleanly.
//
// The first Snapshot lands after the adapter's initial fetch completes;
// subsequent Snapshots arrive on the adapter's own cadence (refsource ~5s
// via ls-remote, ghsource ~30s via the GH API, fake-for-tests on demand).
type Source interface {
	Watch(ctx context.Context) <-chan Snapshot
}

// Renderer is the outbound port that consumes derived Views and presents
// them to the user. Same signature for every output mode:
//
//   - The bubble-tea TUI adapter blocks until the user quits.
//   - The plain-text adapter reads the first View, writes once, and returns.
//   - The (future) web adapter starts an HTTP server and runs until ctx
//     cancellation.
//   - Test fakes capture Views into a slice for assertions.
//
// Adapters choose how long they consume the channel; the Lens just feeds it.
type Renderer interface {
	Render(ctx context.Context, views <-chan View) error
}
