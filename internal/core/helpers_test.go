package core_test

import (
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
)

// ev constructs a clarityrefs.Event with the given stage / status and a Unix
// timestamp seconds value. Used throughout the core test suite.
func ev(stage, status string, ts int64) clarityrefs.Event {
	return clarityrefs.Event{Stage: stage, Status: status, Time: time.Unix(ts, 0)}
}
