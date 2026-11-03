package tui

import (
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"
)

// chromeSurface decides which way the header bar elevates off the terminal
// background, and by how much. The table pins every rule:
//
//   - no background, no surface — there is nothing to derive one from
//   - with no foreground answer, the old directional rule stands: dark
//     backgrounds lighten, light ones darken
//   - a candidate that cannot be told apart from the background is not a
//     surface: near-black cannot darken, near-white cannot lighten
//   - when both directions are visible, the one that keeps more contrast
//     against the default foreground wins — a mid-tone theme must elevate
//     away from its text, not under it
func TestChromeSurface(t *testing.T) {
	var (
		nearBlack = lipgloss.Color("#0d1117")
		nearWhite = lipgloss.Color("#ffffff")
		solLight  = lipgloss.Color("#fdf6e3")
		midDark   = lipgloss.Color("#44475a")
		midLight  = lipgloss.Color("#d0d0d0")
		lightText = lipgloss.Color("#f8f8f2")
		darkText  = lipgloss.Color("#222222")
	)

	cases := []struct {
		name   string
		bg, fg color.Color
		want   color.Color
	}{
		{"no background means no surface",
			nil, lightText, nil},
		{"unknown foreground on a dark terminal lightens",
			nearBlack, nil, lipgloss.Lighten(nearBlack, chromeElevation)},
		{"unknown foreground on a light terminal darkens",
			solLight, nil, lipgloss.Darken(solLight, chromeElevation*lightElevationRatio)},
		{"near-black can only lighten — darkening it is invisible",
			nearBlack, lightText, lipgloss.Lighten(nearBlack, chromeElevation)},
		{"near-white can only darken — lightening it is invisible",
			nearWhite, darkText, lipgloss.Darken(nearWhite, chromeElevation*lightElevationRatio)},
		{"mid-dark with light text darkens, away from the text",
			midDark, lightText, lipgloss.Darken(midDark, chromeElevation)},
		{"mid-light with dark text lightens, away from the text",
			midLight, darkText, lipgloss.Lighten(midLight, chromeElevation)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := chromeSurface(tc.bg, tc.fg); got != tc.want {
				t.Errorf("chromeSurface(%v, %v) = %v, want %v", tc.bg, tc.fg, got, tc.want)
			}
		})
	}
}
