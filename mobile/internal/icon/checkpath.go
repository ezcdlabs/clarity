package icon

import (
	"fmt"
	"math"
	"strings"
)

// CheckPath returns the mark's front check as a path, fitted to a square canvas
// of the given size with the given margin on every side.
//
// One check rather than three, because this is for the places where only a
// silhouette survives: an Android status bar icon is drawn as a white mask, and
// at 24dp three overlapping checks are mud once the colours that tell them
// apart have been thrown away. The remaining shape is still the mark — the
// check is what the mark is a picture of.
//
// Coordinates are "x,y" pairs in a path of the form "M a L b L c Z", which is
// what both an Android vector drawable and an SVG understand.
func CheckPath(size, margin float64) string {
	h := check(elbowX, elbowY, strokeFraction, shortArm, longArm)

	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, c := range h {
		minX, maxX = math.Min(minX, c[0]), math.Max(maxX, c[0])
		minY, maxY = math.Min(minY, c[1]), math.Max(maxY, c[1])
	}

	// Fitted to whichever dimension runs out first, so the mark fills the canvas
	// it was given rather than sitting in the middle of it at some fraction of
	// the size. The check is wider than it is tall, so in practice that is the
	// width — but a canvas is not always square-ish and guessing would be the
	// kind of assumption that holds until one day it does not.
	room := size - 2*margin
	scale := math.Min(room/(maxX-minX), room/(maxY-minY))

	// Centred on what is left over in the other dimension.
	offX := margin + (room-(maxX-minX)*scale)/2
	offY := margin + (room-(maxY-minY)*scale)/2

	var b strings.Builder
	for i, c := range h {
		if i == 0 {
			b.WriteString("M ")
		} else {
			b.WriteString(" L ")
		}
		fmt.Fprintf(&b, "%.3f,%.3f",
			offX+(c[0]-minX)*scale,
			offY+(c[1]-minY)*scale)
	}
	b.WriteString(" Z")
	return b.String()
}
