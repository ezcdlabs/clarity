package dev.ezcd.clarity.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.sp

/**
 * The TUI's palette, read on a screen instead of a terminal.
 *
 * The terminal asks for ANSI 1/2/3/8/12 and lets the emulator decide what those
 * look like. A phone has no such table, so these are the same six roles pinned
 * to values that stay legible on an OLED panel — close enough that the two UIs
 * are recognisably the same tool, not matched hex for hex to a palette that was
 * never fixed in the first place.
 */
object Ink {
    val bg = Color(0xFF101010)
    val surface = Color(0xFF1A1A1A)
    val line = Color(0xFF2A2A2A)
    val text = Color(0xFFE6E6E6)

    /** ANSI 8. Everything secondary in both UIs is this. */
    val dim = Color(0xFF808080)

    val red = Color(0xFFE06C75)
    val green = Color(0xFF98C379)
    val yellow = Color(0xFFE5C07B)
    val blue = Color(0xFF61AFEF)
}

/** Commit shas and anything else that must line up vertically. */
val Mono = TextStyle(fontFamily = FontFamily.Monospace, fontSize = 13.sp)

@Composable
fun ClarityTheme(content: @Composable () -> Unit) {
    // Dark either way: clarity is a dark UI, and isSystemInDarkTheme is read
    // only so a future light palette has somewhere obvious to go.
    @Suppress("UNUSED_EXPRESSION") isSystemInDarkTheme()

    MaterialTheme(
        colorScheme = darkColorScheme(
            background = Ink.bg,
            surface = Ink.surface,
            onBackground = Ink.text,
            onSurface = Ink.text,
            primary = Ink.blue,
            error = Ink.red,
        ),
        typography = Typography(),
        content = content,
    )
}
