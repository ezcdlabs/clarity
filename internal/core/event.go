package core

import "github.com/ezcdlabs/clarity/clarityrefs"

// Event is the on-ref event shape, re-exported from clarityrefs (which
// stays the canonical public Go API for reading/writing the events ref).
// The core imports clarityrefs; clarityrefs does NOT import core — the
// dependency is one-way.
type Event = clarityrefs.Event

// Events is a SHA-keyed grouping of events, the shape produced by Source
// adapters and consumed by EventWriter implementations. Equivalent to
// clarityrefs.ReadAllEvents' return type, just named for ergonomic use
// inside the core.
type Events map[string][]Event
