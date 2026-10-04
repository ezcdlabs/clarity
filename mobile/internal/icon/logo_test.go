package icon

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"testing"
)

// The mark is committed output, like generated protobuf. Two runs must produce
// the same bytes or every regeneration is a diff.
func TestLogoIsDeterministic(t *testing.T) {
	a := encodeLogo(t, Logo(128, OnNothing, 1))
	b := encodeLogo(t, Logo(128, OnNothing, 1))
	if !bytes.Equal(a, b) {
		t.Error("two renders of the mark differ")
	}
}

func TestLogoCentreIsTheBrandNavy(t *testing.T) {
	img := Logo(180, OnNothing, 1)
	// A point inside the circle and clear of all three checks: low and left.
	r, g, b, a := img.At(40, 150).RGBA()
	if a != 0xffff {
		t.Fatalf("the circle is not opaque at (40,150): alpha %d", a)
	}
	wr, wg, wb, _ := Navy.RGBA()
	if r != wr || g != wg || b != wb {
		t.Errorf("got %v %v %v, want the navy %v %v %v", r, g, b, wr, wg, wb)
	}
}

// The three checks are the brand. All of them have to survive, which a wrong
// offset or a wrong clip would quietly undo by hiding one behind another.
func TestLogoShowsAllThreeChecks(t *testing.T) {
	img := Logo(360, OnNothing, 1)
	seen := map[string]int{}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			switch at(img, x, y) {
			case White:
				seen["white"]++
			case Yellow:
				seen["yellow"]++
			case Blue:
				seen["blue"]++
			}
		}
	}
	for _, k := range []string{"white", "yellow", "blue"} {
		// A sliver would pass a "greater than zero" test while looking broken.
		if seen[k] < 2000 {
			t.Errorf("%s covers only %d pixels of a 360px mark", k, seen[k])
		}
	}
}

// The blue check runs past the circle at the top right, and the white one is
// cut off where it reaches the edge. That difference is the composition — a
// version that clipped all three, or none, is a different mark.
func TestLogoClipsTheBackChecksButNotTheFront(t *testing.T) {
	const size = 360
	img := Logo(size, OnNothing, 1)
	centre := float64(size) / 2
	radius := circleFraction * float64(size)

	outside := func(x, y int) bool {
		// A pixel's distance has to clear the edge by more than antialiasing
		// can smear, or the test reads the rim rather than the ink.
		return math.Hypot(float64(x)+0.5-centre, float64(y)+0.5-centre) > radius+1.5
	}

	blueOut := 0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if !outside(x, y) {
				continue
			}
			switch at(img, x, y) {
			case Blue:
				blueOut++
			case White:
				t.Fatalf("white ink at (%d,%d) is outside the circle", x, y)
			case Yellow:
				t.Fatalf("yellow ink at (%d,%d) is outside the circle", x, y)
			case Navy:
				t.Fatalf("the circle is painted at (%d,%d), outside its own radius", x, y)
			}
		}
	}
	if blueOut < 200 {
		t.Errorf("the blue check escapes the circle by only %d pixels; it should be visible", blueOut)
	}
}

// Everything outside both the circle and the blue check is nothing at all —
// the mark is placed on whatever is behind it.
func TestLogoCornersAreTransparent(t *testing.T) {
	img := Logo(180, OnNothing, 1)
	for _, p := range [][2]int{{2, 2}, {177, 177}, {2, 177}} {
		if _, _, _, a := img.At(p[0], p[1]).RGBA(); a != 0 {
			t.Errorf("corner (%d,%d) has alpha %d", p[0], p[1], a)
		}
	}
}

func TestLogoScalesWithoutChangingProportion(t *testing.T) {
	small := inkFraction(Logo(90, OnNothing, 1))
	large := inkFraction(Logo(540, OnNothing, 1))
	if math.Abs(small-large) > 0.01 {
		t.Errorf("the mark covers %.3f of a 90px canvas but %.3f of a 540px one", small, large)
	}
}

func encodeLogo(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// at reports which brand colour a pixel is, exactly — antialiased edges match
// nothing, which is what keeps these tests off the rim.
func at(img *image.RGBA, x, y int) interface{} {
	r, g, b, a := img.At(x, y).RGBA()
	if a != 0xffff {
		return nil
	}
	for _, c := range []interface{}{Navy, White, Yellow, Blue} {
		cr, cg, cb, _ := c.(interface {
			RGBA() (uint32, uint32, uint32, uint32)
		}).RGBA()
		if r == cr && g == cg && b == cb {
			return c
		}
	}
	return nil
}

// TestFilledHasNoTransparency covers the iOS app icon, which is rejected
// outright if any pixel is see-through.
func TestFilledHasNoTransparency(t *testing.T) {
	const size = 128
	img := Logo(size, OnNavy, 1)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xffff {
				t.Fatalf("pixel (%d,%d) has alpha %d", x, y, a)
			}
		}
	}
	// And the disc has to vanish into the ground rather than sit on it as a
	// slightly different navy.
	r, g, b, _ := img.At(2, 2).RGBA()
	wr, wg, wb, _ := Navy.RGBA()
	if r != wr || g != wg || b != wb {
		t.Errorf("the corner is %v %v %v, not the navy %v %v %v", r, g, b, wr, wg, wb)
	}
}

// TestInkOnlyLeavesTheNavyOut covers the adaptive icon's foreground layer.
// Android paints the background itself; a foreground carrying its own navy
// would be a second one that cannot move with the first.
func TestInkOnlyLeavesTheNavyOut(t *testing.T) {
	img := Logo(360, InkOnly, ForegroundScale)
	b := img.Bounds()

	ink := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bb, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			nr, ng, nb, _ := Navy.RGBA()
			if r == nr && g == ng && bb == nb && a == 0xffff {
				t.Fatalf("navy painted at (%d,%d); the background layer cannot show through it", x, y)
			}
			ink++
		}
	}
	if ink < 4000 {
		t.Errorf("only %d pixels of ink on a 360px foreground", ink)
	}
}

// TestTheForegroundFillsTheMaskWithoutOverflowing pins both halves of what the
// adaptive foreground is for.
//
// Nothing may be drawn outside the circle a launcher shows, or it is sliced off
// at an arbitrary radius. And the mark has to reach that circle, or the icon is
// a small logo adrift in navy — which is what sizing to the inner safe zone
// produced, and what made it illegible at 48dp.
func TestTheForegroundFillsTheMaskWithoutOverflowing(t *testing.T) {
	const size = 216
	img := Logo(size, InkOnly, ForegroundScale)

	centre := float64(size) / 2
	mask := float64(size) * maskFraction / 2

	reach := 0.0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a == 0 {
				continue
			}
			d := math.Hypot(float64(x)+0.5-centre, float64(y)+0.5-centre)
			if d > mask+1 {
				t.Fatalf("ink at (%d,%d) is %.1fpx from centre, past the %.1fpx mask", x, y, d, mask)
			}
			reach = math.Max(reach, d)
		}
	}
	if reach < mask*0.9 {
		t.Errorf("the mark only reaches %.1fpx of the %.1fpx mask — it will read as too small", reach, mask)
	}
}

// inkFraction is how much of the canvas carries any ink at all.
func inkFraction(img *image.RGBA) float64 {
	b := img.Bounds()
	var ink float64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			ink += float64(a) / 0xffff
		}
	}
	return ink / float64(b.Dx()*b.Dy())
}

// TestTheSplashMarkSurvivesAMask covers the thing a launch screen will not
// promise. Android gives a splash icon a 288dp canvas and guarantees only the
// inner 192dp; whether it masks is not something to rely on, so the whole mark
// — including the blue check's overflow, which the adaptive foreground gives up
// — has to sit inside that circle on its own.
func TestTheSplashMarkSurvivesAMask(t *testing.T) {
	const size = 288
	img := Logo(size, OnNothing, SplashScale)

	centre := float64(size) / 2
	visible := float64(size) * maskFraction / 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a == 0 {
				continue
			}
			if d := math.Hypot(float64(x)+0.5-centre, float64(y)+0.5-centre); d > visible {
				t.Fatalf("ink at (%d,%d) is %.1fpx out, past the %.1fpx a mask would keep", x, y, d, visible)
			}
		}
	}
}
