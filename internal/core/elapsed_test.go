package core_test

import (
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/internal/core"
)

func TestFormatElapsed(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{5 * time.Second, "5s"},
		{59 * time.Second, "59s"},
		{60 * time.Second, "1m 00s"},
		{90 * time.Second, "1m 30s"},
		{59*time.Minute + 59*time.Second, "59m 59s"},
		{60 * time.Minute, "1h 00m 00s"},
		{2*time.Hour + 3*time.Minute + 5*time.Second, "2h 03m 05s"},
	}
	for _, tc := range cases {
		got := core.FormatElapsed(tc.d)
		if got != tc.want {
			t.Errorf("FormatElapsed(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

// TestFormatAge covers the precision-decay ladder for live "… ago" timers.
// Precision is what makes a timer tick, so it decays with magnitude: seconds
// below the hour, minutes below two days, whole days beyond — a week-old
// deploy must not claim second-level relevance by ticking. The lead-time
// column deliberately keeps FormatElapsed: it is read down as a column, and
// consistent unit shape is what makes it scannable.
func TestFormatAge(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{59 * time.Second, "59s"},
		{60 * time.Second, "1m 00s"},
		{59*time.Minute + 59*time.Second, "59m 59s"},
		// at the hour the seconds go: the timer now changes once a minute
		{60 * time.Minute, "1h 00m"},
		{2*time.Hour + 3*time.Minute + 5*time.Second, "2h 03m"},
		{47*time.Hour + 59*time.Minute, "47h 59m"},
		// at two days a single floor unit: "1d" never appears, and the
		// timer changes once a day
		{48 * time.Hour, "2d"},
		{49 * time.Hour, "2d"},
		{192*time.Hour + 7*time.Second, "8d"},
	}
	for _, tc := range cases {
		got := core.FormatAge(tc.d)
		if got != tc.want {
			t.Errorf("FormatAge(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
