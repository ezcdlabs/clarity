package tui

import (
	"image/color"
	"testing"
)

// TestIsDarkColor pins the classifier over the colours detectTerminal and
// chromeSurface feed it.
//
// The nil case is a field report, not a hypothetical: on Windows,
// lipgloss.BackgroundColor can return a nil colour with a nil error — the
// terminal simply has no OSC 11 answer — and isDarkColor(nil) crashed the
// whole TUI with a nil-pointer panic before a single frame rendered. nil
// must classify as dark, because "no answer" is exactly the case where
// HasDarkBackground's assume-dark default applies.
func TestIsDarkColor(t *testing.T) {
	tests := []struct {
		name string
		c    color.Color
		want bool
	}{
		{"nil colour (terminal gave no answer) defaults dark", nil, true},
		{"black", color.RGBA{0, 0, 0, 255}, true},
		{"white", color.RGBA{255, 255, 255, 255}, false},
		{"mid gray just below the lightness midpoint", color.RGBA{127, 127, 127, 255}, true},
		{"mid gray just above the lightness midpoint", color.RGBA{128, 128, 128, 255}, false},
		// HSL lightness is (max+min)/2, so a fully saturated primary sits
		// exactly on the midpoint — and the strict < means it reads light,
		// which is lipgloss's own verdict for these colours.
		{"pure red sits on the midpoint, reads light", color.RGBA{255, 0, 0, 255}, false},
		{"dark maroon reads dark", color.RGBA{128, 0, 0, 255}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDarkColor(tc.c); got != tc.want {
				t.Errorf("isDarkColor(%v) = %v, want %v", tc.c, got, tc.want)
			}
		})
	}
}
