// Package clock abstracts time.After so that tests can inject a fake clock
// instead of waiting for real time to pass.
package clock

import "time"

// Clock is the interface used by polling loops for the wait-poll timer.
type Clock interface {
	After(d time.Duration) <-chan time.Time
}

// Real returns a Clock backed by the real time package.
func Real() Clock { return realClock{} }

type realClock struct{}

func (realClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}
