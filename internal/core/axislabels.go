package core

import (
	"fmt"
	"math"
	"time"
)

// WeekLabel is the one spelling of an ISO week: "W2026-39".
//
// Here rather than at each call site because it is written into a deploy
// divider, a metrics row and a mobile payload, and three spellings of the same
// week would read as three different weeks. Zero-padded so a column of them
// lines up and W2026-9 cannot be misread as week 90-something.
func WeekLabel(s WeekStat) string {
	return fmt.Sprintf("W%d-%02d", s.Year, s.Week)
}

// FormatAxisTick labels one tick on a lead time axis.
//
// Not FormatElapsed, which is right for a timer and wrong here: it renders two
// hours as "2h 00m 00s" and has no day unit at all, so a three-day axis would
// be labelled "72h 00m 00s" four times over.
//
// The rule is precision at the quarter marks. An axis is labelled at 0, a
// quarter, a half, three quarters and the end, and whatever unit a tick uses
// has to keep those five distinct — days lose too much precision to do that on
// their own, so they carry a decimal where it is the only thing separating two
// ticks. The quarters of a 2d axis are 12h, 1d and 1.5d.
//
// A rendering decision rather than terminal geometry, which is why it is here
// and not in the TUI: a phone's axis has the same five fractions and the same
// problem, and a second copy of this switch is a second answer to it.
func FormatAxisTick(d time.Duration) string {
	switch {
	case d == 0:
		// The origin of the scale rather than a duration of zero seconds.
		return "0"
	case d >= 72*time.Hour:
		days := d.Hours() / 24
		if days == math.Trunc(days) {
			return fmt.Sprintf("%dd", int(days))
		}
		return fmt.Sprintf("%.1fd", days)
	case d >= time.Hour:
		// A decimal for the same reason days carry one, and it was missing:
		// rounding to whole hours labelled the three-quarter tick of a 2h axis
		// "2h" and then labelled the end "2h" as well, so the axis read
		// "0 30m 1h 2h 2h". It also quietly misreported every other quarter it
		// touched — the quarters of a 6h axis were 2h and 5h rather than 1.5h
		// and 4.5h.
		hours := d.Hours()
		if hours == math.Trunc(hours) {
			return fmt.Sprintf("%dh", int(hours))
		}
		return fmt.Sprintf("%.1fh", hours)
	default:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
}
