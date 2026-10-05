package icon_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/mobile/internal/icon"
)

// points pulls the coordinates back out of a path, so the tests can measure the
// shape rather than compare strings.
func points(t *testing.T, path string) [][2]float64 {
	t.Helper()
	var out [][2]float64
	for _, token := range strings.Fields(strings.NewReplacer("M", " ", "L", " ", "Z", " ").Replace(path)) {
		var x, y float64
		if _, err := fmt.Sscanf(token, "%g,%g", &x, &y); err != nil {
			t.Fatalf("unparsable coordinate %q in %q", token, path)
		}
		out = append(out, [2]float64{x, y})
	}
	return out
}

// TestCheckPath_IsTheSixCorneredCheck. A status bar icon is a silhouette, and
// the silhouette of this mark is one check — three overlapping ones are mud at
// 24dp, where the colours that tell them apart have been thrown away.
func TestCheckPath_IsTheSixCorneredCheck(t *testing.T) {
	p := points(t, icon.CheckPath(24, 2))
	if len(p) != 6 {
		t.Fatalf("got %d corners, want the check's six", len(p))
	}
}

// TestCheckPath_FitsTheCanvasWithItsMargin covers both halves of fitting: it
// has to be inside the box, and it has to fill it. An icon drawn at a third of
// the size it was given is legal and useless.
func TestCheckPath_FitsTheCanvasWithItsMargin(t *testing.T) {
	const size, margin = 24, 2
	p := points(t, icon.CheckPath(size, margin))

	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, c := range p {
		minX, maxX = math.Min(minX, c[0]), math.Max(maxX, c[0])
		minY, maxY = math.Min(minY, c[1]), math.Max(maxY, c[1])
	}

	const eps = 0.01
	if minX < margin-eps || minY < margin-eps || maxX > size-margin+eps || maxY > size-margin+eps {
		t.Errorf("the mark runs outside its margin: x %.2f..%.2f, y %.2f..%.2f in %v with margin %v",
			minX, maxX, minY, maxY, float64(size), float64(margin))
	}

	// The check is wider than it is tall, so width is the dimension that should
	// be fully used.
	if want := float64(size - 2*margin); math.Abs((maxX-minX)-want) > eps {
		t.Errorf("width = %.2f, want the whole %.2f it was given", maxX-minX, want)
	}
}

// TestCheckPath_IsCentred — an icon that fits but sits against one edge reads
// as misaligned next to every other icon in the status bar.
func TestCheckPath_IsCentred(t *testing.T) {
	const size, margin = 24, 2
	p := points(t, icon.CheckPath(size, margin))

	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, c := range p {
		minY, maxY = math.Min(minY, c[1]), math.Max(maxY, c[1])
	}
	above, below := minY, float64(size)-maxY
	if math.Abs(above-below) > 0.01 {
		t.Errorf("%.2f above and %.2f below; the mark is not centred", above, below)
	}
}

// TestCheckPath_ScalesWithTheCanvas keeps it resolution-free, like every other
// use of the mark's geometry.
func TestCheckPath_ScalesWithTheCanvas(t *testing.T) {
	small := points(t, icon.CheckPath(24, 0))
	large := points(t, icon.CheckPath(48, 0))
	for i := range small {
		for axis := 0; axis < 2; axis++ {
			if math.Abs(small[i][axis]*2-large[i][axis]) > 0.01 {
				t.Fatalf("corner %d axis %d: %.3f at 24 but %.3f at 48",
					i, axis, small[i][axis], large[i][axis])
			}
		}
	}
}
