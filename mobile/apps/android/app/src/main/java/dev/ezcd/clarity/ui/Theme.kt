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
    /** The reading surface: the sheet the commits sit on, and the darker of the two. */
    val bg: Color,
    /**
     * The chrome around it — the top bar, the flow strip, the ground the sheet
     * is laid on. One step toward the middle from [bg], and the tone that
     * carries the brand.
     */
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

// Two tones: a chrome the bars sit on, and a darker sheet the commits are read
// on. A reading surface should be the extreme of the pair — every role gains
// contrast against it — and the step between them is tonal rather than a
// boundary, which is what makes them read as one ground with a sheet laid on it.
//
// Both are neutral. The brand navy was tried here and it was too much: a tint
// across a whole screen is not a brand, it is a cast, and it fought the one
// colour on the screen that is supposed to mean something. The brand lives in
// the mark, in the launch screen, and in the two accents below — which is
// enough, and is where a reader is looking anyway.
private val DarkInk = Palette(
    bg = Color(0xFF0E0E0E),
    surface = Color(0xFF1A1A1A),
    line = Color(0xFF2B2B2B),
    text = Color(0xFFE6E6E6),
    dim = Color(0xFF8C8C8C),
    red = Color(0xFFE06C75),
    green = Color(0xFF98C379),
    yellow = Color(0xFFF7C421),  // the mark's yellow
    blue = Color(0xFF6AA2FF),    // the mark's blue
    errorBg = Color(0xFF2A1416),
)

// Light cannot take the brand's colours at their own values: the mark's yellow
// on white is barely a colour and its blue is a highlight rather than something
// legible. The navy does carry, as the ink — an off-black with the brand's hue
// in it, at 17:1 against the page. It is a tint you read rather than one you
// sit in, which is the difference that made it work here and not there.
private val LightInk = Palette(
    bg = Color(0xFFFCFCFC),
    surface = Color(0xFFEDEDED),
    line = Color(0xFFDCDCDC),
    text = Color(0xFF061732),    // the mark's navy, as off-black
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
