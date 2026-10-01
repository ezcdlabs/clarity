// Package icon draws clarity's app icon.
//
// The icon is generated rather than drawn in an editor, and the PNGs are
// committed like generated protobuf: regenerating is reproducible, the mark is
// defined once for both platforms, and a change to it is a readable diff rather
// than a binary one.
//
// The mark is a check. It is the TUI's ✓ — the glyph the whole product is built
// around answering — and it survives being 48 pixels wide, which most marks do
// not.
package icon

import (
	"image"
	"image/color"
	"math"
)

// Ink is ANSI green, the colour the TUI uses for a passing stage.
var Ink = color.RGBA{0x98, 0xC3, 0x79, 0xFF}

// Background is the app's own background, so the icon and the first frame the
// app draws are the same colour.
var Background = color.RGBA{0x10, 0x10, 0x10, 0xFF}

// Shape is how the canvas behind the mark is filled.
type Shape int

const (
	// Transparent paints no background. This is the adaptive-icon foreground
	// layer, which Android composites over a colour of its own and masks to
	// whatever shape the launcher wants.
	Transparent Shape = iota
	// Rounded and Circle are pre-masked, for the legacy launcher icons that
	// Android 7 — still inside minSdk — does not mask itself.
	Rounded
	Circle
	// Square fills every pixel. iOS rejects an app icon with any transparency
	// and applies its own mask.
	Square
)

// safeFraction is the diameter, as a fraction of the canvas, that Android
// guarantees survives an adaptive icon's mask. Anything outside it is clipped on
// some launchers and not others.
const safeFraction = 0.66

// The mark, in fractions of the canvas with y downwards: the short arm falls to
// the elbow, the long arm rises out of it. Chosen so the whole stroke, cap
// included, stays inside the safe circle.
var (
	markPoints = [3][2]float64{
		{0.31, 0.53},
		{0.44, 0.66},
		{0.71, 0.36},
	}
	markHalfWidth = 0.0725
)

// cornerRadius is the radius of Rounded, as a fraction of the canvas.
const cornerRadius = 0.22

// Render draws the icon at size×size pixels.
func Render(size int, shape Shape) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Supersampling, rather than a rasteriser dependency: the shapes are a
	// polyline and a rounded rectangle, both of which are a distance function,
	// and 4×4 coverage is indistinguishable from analytic antialiasing at every
	// size this is rendered at.
	const ss = 4
	const samples = ss * ss
	s := float64(size)

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var inkHits, bgHits float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := (float64(x) + (float64(sx)+0.5)/ss) / s
					py := (float64(y) + (float64(sy)+0.5)/ss) / s
					if onMark(px, py) {
						inkHits++
					}
					if inShape(shape, px, py) {
						bgHits++
					}
				}
			}
			img.SetRGBA(x, y, composite(inkHits/samples, bgHits/samples))
		}
	}
	return img
}

// composite puts the ink over the background, in the premultiplied form
// image.RGBA stores.
func composite(inkAlpha, bgAlpha float64) color.RGBA {
	behind := bgAlpha * (1 - inkAlpha)
	mix := func(ink, bg uint8) uint8 {
		return uint8(math.Round(float64(ink)*inkAlpha + float64(bg)*behind))
	}
	return color.RGBA{
		R: mix(Ink.R, Background.R),
		G: mix(Ink.G, Background.G),
		B: mix(Ink.B, Background.B),
		A: uint8(math.Round(255 * (inkAlpha + behind))),
	}
}

// onMark reports whether a point is within half a stroke width of the polyline.
func onMark(x, y float64) bool {
	for i := 0; i < len(markPoints)-1; i++ {
		if distanceToSegment(x, y, markPoints[i], markPoints[i+1]) <= markHalfWidth {
			return true
		}
	}
	return false
}

func distanceToSegment(x, y float64, a, b [2]float64) float64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	length := dx*dx + dy*dy
	// Clamping t to [0,1] is what gives the stroke round caps, and what keeps
	// the elbow filled where the two segments meet.
	t := ((x-a[0])*dx + (y-a[1])*dy) / length
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(x-(a[0]+t*dx), y-(a[1]+t*dy))
}

func inShape(shape Shape, x, y float64) bool {
	switch shape {
	case Transparent:
		return false
	case Square:
		return true
	case Circle:
		return math.Hypot(x-0.5, y-0.5) <= 0.5
	case Rounded:
		// Distance to the rounded rectangle's inner box, which is zero
		// everywhere except the corners.
		dx := math.Max(0, math.Abs(x-0.5)-(0.5-cornerRadius))
		dy := math.Max(0, math.Abs(y-0.5)-(0.5-cornerRadius))
		return math.Hypot(dx, dy) <= cornerRadius
	}
	return false
}
