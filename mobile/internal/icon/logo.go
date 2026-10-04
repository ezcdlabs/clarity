package icon

import (
	"image"
	"image/color"
	"math"
)

// The ezcd mark, as geometry rather than as a file.
//
// Three identical checks, offset along the x axis, on a navy disc: white at the
// back, then yellow, then blue in front. The two at the back are clipped to the
// disc — the white one runs off its lower left and is cut there — while the blue
// one is allowed past the edge at the top right. That asymmetry is the
// composition, not an accident of how it was drawn: the mark reads as layers,
// and layers need something to be in front of.
//
// The numbers below were measured off the original artwork rather than guessed.
// Both arms of the check sit at exactly 45°, the ends are cut square to the arm
// rather than rounded, and the elbow is a mitre. Keeping it parametric is the
// point: every icon either platform wants is this, at a different size.
var (
	Navy   = color.RGBA{0x06, 0x17, 0x32, 0xFF}
	White  = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	Yellow = color.RGBA{0xF7, 0xC4, 0x21, 0xFF}
	Blue   = color.RGBA{0x6A, 0xA2, 0xFF, 0xFF}
)

// Everything is a fraction of the canvas, so the mark is resolution-free.
//
// These came off the artwork by fitting the edges of each arm rather than
// finding its corners: antialiasing erodes a sharp corner by a pixel or two, so
// the corners read as consistently inside where the geometry really is, while a
// long straight edge fits exactly. Both arms turn out to sit at precisely 45°
// and the three checks are exactly 33px apart on a 180px mark.
const (
	// circleFraction is the disc's radius.
	circleFraction = 0.4878

	// strokeFraction is the width of a check's arm.
	strokeFraction = 0.0864

	// The front check's elbow, and the length of each arm from it. The arms run
	// at 45°, up-left and up-right.
	elbowX   = 0.4770
	elbowY   = 0.6978
	shortArm = 0.2990
	longArm  = 0.6840

	// How far left each check sits from the one in front of it.
	checkOffset = 0.1806

	// maskFraction is the diameter, as a fraction of an adaptive icon's canvas,
	// that a launcher actually shows: 72dp of 108. The inner 66dp is the
	// stricter "safe zone", and a mark with a subject floating in space should
	// respect it.
	//
	// This one should not. It is a full-bleed disc, so there is no subject near
	// the edge to lose — a launcher that masks tighter crops navy, exactly as a
	// tighter mask crops any circular logo. Sizing to the safe zone instead left
	// the mark adrift in a ring of empty navy and barely legible at 48dp, which
	// is the size that decides whether an icon works.
	maskFraction = 72.0 / 108.0

	// Every check carries a navy moat, which is what makes the stack read as a
	// stack. Two parallel arms are far enough apart to show navy between them
	// on their own, but where a back check's long arm passes behind a front
	// check's short arm they would otherwise meet and merge into one shape.
	// The moat cuts the one behind short, so the front check always has a clean
	// edge against whatever it covers.
	moat = 0.0402
)

// ForegroundScale maps the mark's disc onto the launcher's mask, so the thing
// the user sees is the mark at the size the mark is drawn — not a shrunken copy
// of it sitting inside a second, invisible circle.
const ForegroundScale = (maskFraction / 2) / circleFraction

// SplashScale fits the whole mark — disc, and the blue check's overflow past it
// — inside the circle a launch screen is guaranteed to show.
//
// Smaller than ForegroundScale, and for a different reason. An adaptive icon is
// masked, so the overflow is given up and the disc is mapped onto the mask. A
// launch screen may or may not mask, so nothing may rely on it: the mark keeps
// its overflow and sits far enough in that a mask would have nothing to cut.
const SplashScale = 0.60

// Ground says what fills the canvas behind the mark.
type Ground int

const (
	// OnNothing leaves everything outside the disc transparent. This is the
	// mark as the brand uses it.
	OnNothing Ground = iota
	// OnNavy extends the disc's own colour to the whole canvas, for the places
	// that cannot have transparency — an iOS app icon, a launch screen — and
	// for anywhere the mark should bleed to the edges.
	OnNavy
	// InkOnly paints the checks and leaves every navy pixel transparent,
	// including the moats, so the layer can sit over a navy of its own. This is
	// what an adaptive icon's foreground is: Android paints the background and
	// masks the pair, so a foreground carrying its own disc would only be a
	// second navy that cannot move with the first.
	//
	// The blue check is clipped here like the others, which it is nowhere else.
	// A masked icon's mask plays the part the disc plays in the mark, and a
	// check cannot be seen escaping the thing that is cutting it.
	InkOnly
)

// Logo draws the mark at size×size pixels.
//
// scale shrinks it about the ink's own centre, for the formats that demand
// clearance round the edges. Pass 1 for the mark at full bleed.
func Logo(size int, ground Ground, scale float64) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	s := float64(size)

	// Each check twice: the ink, and the moat it cuts into whatever is behind.
	frontInk, frontMoat := layer(elbowX)
	middleInk, middleMoat := layer(elbowX - checkOffset)
	backInk, _ := layer(elbowX - 2*checkOffset)

	const ss = 4
	const samples = ss * ss
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var hits [5]float64 // navy, white, yellow, blue, nothing
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					// Scaled about the disc, which is the mark's own frame
					// and stays square with the canvas under every ground.
					px := (float64(x)+(float64(sx)+0.5)/ss)/s - 0.5
					py := (float64(y)+(float64(sy)+0.5)/ss)/s - 0.5
					px, py = px/scale+0.5, py/scale+0.5

					hits[sample(px, py, frontInk, frontMoat, middleInk, middleMoat, backInk, ground)]++
				}
			}
			img.SetRGBA(x, y, blend(hits, samples))
		}
	}
	return img
}

// sample decides what one point lands on, front to back.
func sample(px, py float64, frontInk, frontMoat, middleInk, middleMoat, backInk hexagon, ground Ground) int {
	const (
		navy = iota
		white
		yellow
		blue
		nothing
	)
	// What the navy of the mark becomes: itself, or a hole for a background
	// layer to show through.
	bg := navy
	if ground == InkOnly {
		bg = nothing
	}
	// And what lies outside the disc.
	outside := nothing
	if ground == OnNavy {
		outside = navy
	}

	onDisc := math.Hypot(px-0.5, py-0.5) <= circleFraction
	switch {
	// Everywhere but a masked layer, the blue check may run past the edge. Its
	// moat may not: out there it would be a navy shadow hanging in mid-air.
	case frontInk.contains(px, py) && (onDisc || ground != InkOnly):
		return blue
	case frontMoat.contains(px, py):
		if onDisc {
			return bg
		}
		return outside
	case !onDisc:
		return outside
	case middleInk.contains(px, py):
		return yellow
	case middleMoat.contains(px, py):
		return bg
	case backInk.contains(px, py):
		return white
	default:
		return bg
	}
}

// layer returns a check and the slightly larger one that clears a path for it.
func layer(ex float64) (ink, clearance hexagon) {
	return check(ex, elbowY, strokeFraction, shortArm, longArm),
		check(ex, elbowY, strokeFraction+2*moat, shortArm+moat, longArm+moat)
}

// blend averages the colours a pixel's samples landed on, in the premultiplied
// form image.RGBA stores. Averaging the samples rather than compositing layer
// over layer is what antialiases the boundary *between* two checks, not just
// their outer edges.
func blend(hits [5]float64, samples float64) color.RGBA {
	// The last slot is "nothing", and it has to be skipped rather than blended
	// as a transparent colour: adding its share to the alpha is what makes a
	// wholly empty pixel come out opaque black.
	cols := [4]color.RGBA{Navy, White, Yellow, Blue}
	var r, g, b, a float64
	for i, n := range hits {
		if n == 0 || i >= len(cols) {
			continue
		}
		f := n / samples
		r += float64(cols[i].R) * f
		g += float64(cols[i].G) * f
		b += float64(cols[i].B) * f
		a += f
	}
	return color.RGBA{
		R: uint8(math.Round(r)),
		G: uint8(math.Round(g)),
		B: uint8(math.Round(b)),
		A: uint8(math.Round(a * 255)),
	}
}

// hexagon is the outline of one check: a two-segment polyline given square ends
// and a mitred elbow. Six corners, which is why it is a polygon test rather
// than the distance field the earlier lone-check mark used — a distance field
// gives round ends and a round elbow, and this artwork has neither.
type hexagon [6][2]float64

// check builds the outline for an elbow at (ex, ey).
//
// The six corners are walked in order around the boundary — out along the
// bottom edge of the short arm, round the elbow's outer mitre, up the bottom
// edge of the long arm, across the tip, and back along the top edges. Order is
// not cosmetic: an even-odd fill of the same six points in the wrong sequence
// is a self-intersecting figure that fills the V between the arms, which is
// precisely what the first attempt drew.
func check(ex, ey, width, short, long float64) hexagon {
	h := width / 2
	const d = math.Sqrt2 / 2

	// Arm directions, from the elbow outwards.
	toTip := [2]float64{d, -d}  // up-right, the long arm
	toEnd := [2]float64{-d, -d} // up-left, the short arm

	p0 := [2]float64{ex + toEnd[0]*short, ey + toEnd[1]*short}
	p1 := [2]float64{ex, ey}
	p2 := [2]float64{ex + toTip[0]*long, ey + toTip[1]*long}

	// The outer side of each arm is the one facing down, away from the V.
	outShort := [2]float64{-d, d}
	outLong := [2]float64{d, d}

	// Where the two outer edges would meet if they ran on. For a right-angled
	// elbow that is h·√2 past the centreline, straight down — and the inner
	// corner is the same distance straight up.
	mitre := math.Sqrt2 * h

	return hexagon{
		{p0[0] + outShort[0]*h, p0[1] + outShort[1]*h},
		{p1[0], p1[1] + mitre},
		{p2[0] + outLong[0]*h, p2[1] + outLong[1]*h},
		{p2[0] - outLong[0]*h, p2[1] - outLong[1]*h},
		{p1[0], p1[1] - mitre},
		{p0[0] - outShort[0]*h, p0[1] - outShort[1]*h},
	}
}

// contains is an even-odd crossing test.
func (hx hexagon) contains(x, y float64) bool {
	in := false
	for i, j := 0, len(hx)-1; i < len(hx); j, i = i, i+1 {
		xi, yi := hx[i][0], hx[i][1]
		xj, yj := hx[j][0], hx[j][1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			in = !in
		}
	}
	return in
}
