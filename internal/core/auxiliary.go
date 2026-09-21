package core

import "context"

// EventWriter is an auxiliary type — it lives next to the core's
// inbound/outbound ports for type-symmetry with EventReader (the source
// side), but the core itself does NOT depend on it. Nothing in core
// imports EventWriter, and the Lens never sends events anywhere.
//
// EventWriter is used only by the `copy-events` subcommand path, which
// reads from a Source and writes to a writer without going through the
// lens at all. Defining it here keeps all I/O-shaped contracts in one
// package, but the file is deliberately separate from ports.go so a
// quick read of the core's surface area doesn't suggest the Lens
// touches it.
type EventWriter interface {
	Write(ctx context.Context, events Events) error
}
