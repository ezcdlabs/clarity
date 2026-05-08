package tui

import (
	"testing"
	"time"
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
		got := formatElapsed(tc.d)
		if got != tc.want {
			t.Errorf("formatElapsed(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
