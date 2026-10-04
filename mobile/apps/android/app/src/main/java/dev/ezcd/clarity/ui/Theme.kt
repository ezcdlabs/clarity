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

private val DarkInk = Palette(
    bg = Color(0xFF101010),
    surface = Color(0xFF1A1A1A),
    line = Color(0xFF2A2A2A),
    text = Color(0xFFE6E6E6),
    dim = Color(0xFF808080),
    red = Color(0xFFE06C75),
    green = Color(0xFF98C379),
    yellow = Color(0xFFE5C07B),
    blue = Color(0xFF61AFEF),
    errorBg = Color(0xFF2A1416),
)

// Not the dark values lightened. A pastel green legible on near-black is
// invisible on near-white, so each role is answered again at a weight that
// carries against a light ground — which is exactly what a terminal theme does
// when it maps the same ANSI codes for a light profile.
private val LightInk = Palette(
    bg = Color(0xFFFCFCFC),
    surface = Color(0xFFEDEDED),
    line = Color(0xFFDCDCDC),
    text = Color(0xFF1A1A1A),
    dim = Color(0xFF6B6B6B),
    red = Color(0xFFC0392B),
    green = Color(0xFF2E7D32),
    yellow = Color(0xFF8A6D00),
    blue = Color(0xFF1565C0),
    errorBg = Color(0xFFFBE9E9),
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
