package dev.ezcd.clarity.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.PlatformTextStyle
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.LineHeightStyle
import androidx.compose.ui.unit.sp

/**
 * The nine roles the TUI paints with, and nothing else.
 *
 * The terminal never picks a colour: it asks for ANSI 1/2/3/8/12 and lets the
 * emulator decide what those look like, which is the whole reason clarity works
 * on a light terminal without a line of code about light terminals. A phone has
 * no such table, so this is it — the same roles, answered twice.
 *
 * Roles, not values, is the part that matters. Anything reaching for a literal
 * colour is a thing that will look wrong under one of the two themes, and the
 * only way to find out is to be holding the phone.
 */
data class Palette(
    val bg: Color,
    /** One step off the background: the header bar the selected tab is cut out of. */
    val surface: Color,
    val line: Color,
    val text: Color,
    /** ANSI 8. Everything secondary in both UIs is this. */
    val dim: Color,
    val red: Color,
    val green: Color,
    val yellow: Color,
    val blue: Color,
    /** The error bar's ground — a tint of red, not red. */
    val errorBg: Color,
)

// Dark is the brand's own ground. The mark is a navy disc, the splash is navy
// to the edges, and opening the app on neutral black meant a visible step from
// one to the other — so the step goes instead.
//
// The lifts are derived from the navy rather than picked beside it, which is
// what the TUI does with the terminal's real background: lipgloss.Lighten(bg,
// 0.10) is how its deploy strip gets a surface that belongs to whatever theme
// it landed in. A grey chosen next to a blue can only ever look pasted on to
// it.
private val DarkInk = Palette(
    bg = Color(0xFF061732),      // the mark's navy, exactly
    surface = Color(0xFF092149),  // +5% lightness
    line = Color(0xFF0C2D63),     // +11%
    text = Color(0xFFE6EAF2),     // cooled a touch, so it belongs to the ground
    dim = Color(0xFF8A94A8),      // ANSI 8's job, in the navy's own family
    red = Color(0xFFE06C75),
    green = Color(0xFF98C379),
    yellow = Color(0xFFF7C421),   // the mark's yellow
    blue = Color(0xFF6AA2FF),     // the mark's blue
    errorBg = Color(0xFF2E1526),
)

// Light cannot take the brand's colours at their own values: the mark's yellow
// on white is barely a colour at all, and its blue is a highlight rather than a
// legible one. What carries over is the navy, as the ink — an off-black with
// the brand's hue in it, which is the one place a light theme can hold the
// identity without giving up contrast.
private val LightInk = Palette(
    bg = Color(0xFFFBFCFD),
    surface = Color(0xFFECEFF4),
    line = Color(0xFFDADFE7),
    text = Color(0xFF061732),     // the mark's navy, as off-black
    dim = Color(0xFF5A6473),
    red = Color(0xFFC0392B),
    green = Color(0xFF2E7D32),
    yellow = Color(0xFF8A6D00),
    blue = Color(0xFF1565C0),
    errorBg = Color(0xFFFBEAEC),
)

private val LocalInk = staticCompositionLocalOf { DarkInk }

/** The palette in force. Reads the same at every call site as a constant did. */
val Ink: Palette
    @Composable
    @ReadOnlyComposable
    get() = LocalInk.current

/** Commit shas and anything else that must line up vertically. */
val Mono = TextStyle(fontFamily = FontFamily.Monospace, fontSize = 13.sp)

/**
 * Text with the breathing room taken out of it.
 *
 * Android pads every text node with the font's own ascent and descent, and the
 * line box adds leading on top. Two stacked Texts therefore sit apart by a gap
 * that belongs to neither of them and that no padding of zero removes — which is
 * why a commit's two lines refused to close up. Trimming the leading on the
 * first and last line is what actually makes them touch.
 */
val Tight = TextStyle(
    platformStyle = PlatformTextStyle(includeFontPadding = false),
    lineHeightStyle = LineHeightStyle(
        alignment = LineHeightStyle.Alignment.Center,
        trim = LineHeightStyle.Trim.Both,
    ),
)

@Composable
fun ClarityTheme(content: @Composable () -> Unit) {
    val dark = isSystemInDarkTheme()
    val ink = if (dark) DarkInk else LightInk

    CompositionLocalProvider(LocalInk provides ink) {
        MaterialTheme(
            colorScheme = if (dark) {
                darkColorScheme(
                    background = ink.bg,
                    surface = ink.surface,
                    onBackground = ink.text,
                    onSurface = ink.text,
                    primary = ink.blue,
                    error = ink.red,
                )
            } else {
                lightColorScheme(
                    background = ink.bg,
                    surface = ink.surface,
                    onBackground = ink.text,
                    onSurface = ink.text,
                    primary = ink.blue,
                    error = ink.red,
                )
            },
            typography = Typography(),
            content = content,
        )
    }
}
