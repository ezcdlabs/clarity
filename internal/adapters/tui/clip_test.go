package tui_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/ezcdlabs/clarity/internal/adapters/tui"
)

// Commit subjects come straight out of `git log %s`, so they are arbitrary
// user text: CJK (two columns per rune), emoji, ZWJ sequences, combining
// marks, and — since nothing sanitises them — ANSI escapes.
var clipSamples = map[string]string{
	"ascii":     "refactor the entire billing subsystem",
	"cjk":       strings.Repeat("重", 18),
	"cjk mixed": "fix 重构 bug 🚀 now",
	"emoji":     "🚀🔥✨ deploy the thing now",
	"zwj":       "👨‍👩‍👧‍👦 family emoji tail",
	"combining": "abcéfgh ijkl",
	"ansi":      "\x1b[31mred\x1b[0m tail tail tail",
	"short":     "ok",
	"empty":     "",
}

// TestClip_NeverExceedsAndNeverPanics is the invariant both helpers exist to
// provide. They previously measured in display columns but sliced by rune
// index, so for any double-width text the index could run past the end of the
// slice — emitting NUL bytes into the terminal, then panicking outright and
// taking the whole program down.
func TestClip_NeverExceedsAndNeverPanics(t *testing.T) {
	for name, sample := range clipSamples {
		for max := -2; max <= 40; max++ {
			t.Run(name, func(t *testing.T) {
				right := tui.ClipRight(sample, max)
				left := tui.ClipLeft(sample, max)

				if max > 0 {
					if w := lipgloss.Width(right); w > max {
						t.Errorf("ClipRight(%q, %d) is %d columns: %q", sample, max, w, right)
					}
					if w := lipgloss.Width(left); w > max {
						t.Errorf("ClipLeft(%q, %d) is %d columns: %q", sample, max, w, left)
					}
				}
				if strings.ContainsRune(right, 0) || strings.ContainsRune(left, 0) {
					t.Errorf("clipping %q at %d emitted NUL bytes", sample, max)
				}
			})
		}
	}
}

// An escape opened inside a clipped subject must not bleed into the rest of
// the row — an unterminated SGR would recolour everything after it, including
// the lead time.
func TestClip_DoesNotLeaveAnOpenEscape(t *testing.T) {
	out := tui.ClipRight("\x1b[31mred\x1b[0m tail tail tail", 7)
	opens := strings.Count(out, "\x1b[31m")
	if opens > 0 && !strings.Contains(out, "\x1b[0m") && !strings.Contains(out, "\x1b[m") {
		t.Errorf("clipped text leaves an unterminated escape: %q", out)
	}
}

// Clipping must be visible: a shortened subject that looks complete is worse
// than one that admits it was cut.
func TestClip_MarksTheCut(t *testing.T) {
	if got := tui.ClipRight("refactor the billing subsystem", 12); !strings.Contains(got, "…") {
		t.Errorf("ClipRight did not mark the cut: %q", got)
	}
	if got := tui.ClipLeft("W2026-38  4 deploys  6d avg", 12); !strings.Contains(got, "…") && !strings.HasPrefix(got, "6d") {
		t.Errorf("ClipLeft did not mark the cut: %q", got)
	}
}
