package icon

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"testing"
)

// The output of this package is committed, like generated protobuf. Two runs
// must produce the same bytes or every regeneration is a diff.
func TestRendersAreDeterministic(t *testing.T) {
	for _, shape := range []Shape{Transparent, Rounded, Circle, Square} {
		a := encode(t, Render(128, shape))
		b := encode(t, Render(128, shape))
		if !bytes.Equal(a, b) {
			t.Errorf("shape %v: two renders differ", shape)
		}
	}
}

// Android masks an adaptive foreground to an arbitrary shape and only
// guarantees the central 66% circle survives. A mark outside it gets clipped on
// some launchers and not others.
func TestForegroundStaysInsideTheSafeZone(t *testing.T) {
	const size = 216
	img := Render(size, Transparent)

	centre := float64(size) / 2
	safe := float64(size) * safeFraction / 2

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a == 0 {
				continue
			}
			dx, dy := float64(x)+0.5-centre, float64(y)+0.5-centre
			if d := math.Hypot(dx, dy); d > safe {
				t.Fatalf("ink at (%d,%d) is %.1fpx from centre, outside the %.1fpx safe zone", x, y, d, safe)
			}
		}
	}
}

// The mark is defined in fractions of the canvas, so it must cover the same
// proportion whatever the density.
func TestTheMarkScalesWithTheCanvas(t *testing.T) {
	small := inkFraction(Render(108, Transparent))
	large := inkFraction(Render(432, Transparent))

	if math.Abs(small-large) > 0.01 {
		t.Errorf("ink covers %.3f of a 108px canvas but %.3f of a 432px one", small, large)
	}
	if small < 0.08 || small > 0.35 {
		t.Errorf("ink covers %.3f of the canvas — a check should be neither a hairline nor a block", small)
	}
}

// A rounded or circular icon is masked by this package because Android 7, which
// minSdk still supports, does not mask legacy icons itself.
func TestShapesMaskTheirCorners(t *testing.T) {
	const size = 96
	tests := []struct {
		shape        Shape
		cornerOpaque bool
	}{
		{Square, true},
		{Rounded, false},
		{Circle, false},
	}
	for _, tc := range tests {
		img := Render(size, tc.shape)
		_, _, _, a := img.At(1, 1).RGBA()
		if got := a > 0; got != tc.cornerOpaque {
			t.Errorf("shape %v: corner opaque = %v, want %v", tc.shape, got, tc.cornerOpaque)
		}
		// Whatever the mask, the middle is always painted.
		if _, _, _, a := img.At(size/2, size/2).RGBA(); a == 0 {
			t.Errorf("shape %v: centre is transparent", tc.shape)
		}
	}
}

// iOS rejects an app icon with any transparency at all.
func TestSquareIsFullyOpaque(t *testing.T) {
	const size = 64
	img := Render(size, Square)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xffff {
				t.Fatalf("pixel (%d,%d) has alpha %d", x, y, a)
			}
		}
	}
}

// The mark is a check: ink low on the left, high on the right, and nothing in
// the top-left corner of the safe zone.
func TestTheMarkIsACheck(t *testing.T) {
	const size = 216
	img := Render(size, Transparent)

	lowLeft := columnInkCentre(img, size*33/100)
	highRight := columnInkCentre(img, size*68/100)

	if !(lowLeft > 0.5) {
		t.Errorf("the short arm should sit low in the canvas, found its centre at %.2f", lowLeft)
	}
	if !(highRight < 0.45) {
		t.Errorf("the long arm should rise, found its centre at %.2f", highRight)
	}
	if lowLeft <= highRight {
		t.Errorf("the arms do not rise left to right: %.2f then %.2f", lowLeft, highRight)
	}
}

func TestForegroundIsTheBrandGreen(t *testing.T) {
	img := Render(216, Transparent)
	// The middle of the long arm, so antialiasing cannot have diluted it.
	r, g, b, a := img.At(216*575/1000, 216*525/1000).RGBA()
	if a != 0xffff {
		t.Fatalf("expected solid ink, got alpha %d", a)
	}
	wr, wg, wb, _ := Ink.RGBA()
	if r != wr || g != wg || b != wb {
		t.Errorf("got %v %v %v, want %v %v %v", r, g, b, wr, wg, wb)
	}
}

func encode(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

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

// columnInkCentre is where the ink sits vertically in one column, 0 at the top
// and 1 at the bottom.
func columnInkCentre(img *image.RGBA, x int) float64 {
	b := img.Bounds()
	var weight, total float64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		_, _, _, a := img.At(x, y).RGBA()
		w := float64(a) / 0xffff
		weight += w * float64(y)
		total += w
	}
	if total == 0 {
		return -1
	}
	return weight / total / float64(b.Dy())
}
