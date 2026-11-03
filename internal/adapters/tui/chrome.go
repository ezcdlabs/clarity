package tui

import (
	"image/color"
	"math"

	"charm.land/lipgloss/v2"
)

// minSurfaceContrast is the least WCAG contrast ratio between the chrome and
// the terminal background at which the bar still reads as its own surface.
// Below it the elevation disappears back into the background it was cut from:
// lightening white and darkening near-black both land here.
const minSurfaceContrast = 1.1

// chromeSurface picks the colour of the header bar: an elevation of the
// terminal background, steered by the default foreground when the terminal
// answered the OSC 10 query.
//
// Two candidates are considered, one step lighter and one step darker than the
// background. A candidate is viable only when it still contrasts with the
// background itself — otherwise there is no visible bar at all. Among viable
// candidates the one that keeps more contrast against the foreground wins, so
// a theme whose background sits mid-range elevates away from its text rather
// than parking the bar underneath it, which is what a fixed "dark lightens"
// rule did to every mid-tone scheme.
//
// With no foreground to steer by, the directional rule stands: dark
// backgrounds lighten, light ones darken.
func chromeSurface(bg, fg color.Color) color.Color {
	if bg == nil {
		return nil
	}

	up := lipgloss.Lighten(bg, chromeElevation)
	// Darkening is multiplicative, so the step that is heavy-handed on a pale
	// background barely moves a dark one: the dark side takes the full
	// elevation, the light side the reduced one (see lightElevationRatio).
	downAmount := chromeElevation
	if !isDarkColor(bg) {
		downAmount *= lightElevationRatio
	}
	down := lipgloss.Darken(bg, downAmount)

	directional := func() color.Color {
		if isDarkColor(bg) {
			return up
		}
		return down
	}

	if fg == nil {
		return directional()
	}

	upViable := contrastRatio(up, bg) >= minSurfaceContrast
	downViable := contrastRatio(down, bg) >= minSurfaceContrast
	switch {
	case upViable && downViable:
		if contrastRatio(fg, down) > contrastRatio(fg, up) {
			return down
		}
		return up
	case upViable:
		return up
	case downViable:
		return down
	}
	// Neither direction produces a visible surface — a degenerate palette.
	// Keep the bar rather than give up on it.
	return directional()
}

// contrastRatio is the WCAG contrast ratio between two colours: 1 for equal
// colours, up to 21 for black against white.
func contrastRatio(a, b color.Color) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// relativeLuminance implements the WCAG definition over Go's 16-bit
// alpha-premultiplied channels. Terminal colours are opaque, so the
// premultiplication is a no-op.
func relativeLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(ch uint32) float64 {
		s := float64(ch) / 65535.0
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}
