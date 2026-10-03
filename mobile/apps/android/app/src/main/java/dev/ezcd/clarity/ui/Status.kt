package dev.ezcd.clarity.ui

import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import dev.ezcd.clarity.proto.Status

/**
 * The glyph and colour for a status, following the terminal's two tables.
 *
 * The proto sends the status and not a glyph, because the TUI's ✓/✗ were chosen
 * to survive a greyscale terminal. They survive here too — but colour is never
 * the only signal, which is the part that actually mattered.
 *
 * **In a row** nothing is green. A tick is grey, and red is spent only on a
 * failure that is still breaking something: once a newer commit has gone green,
 * the older failure is history and goes grey with everything else. A screen of
 * green ticks spends the one colour that means "look at this" on the state that
 * needs no attention at all.
 *
 * **In the header** the tick is green, because that row is the summary and its
 * whole job is answering "is the pipeline green?" at a glance. A started stage
 * shows as unresolved rather than as progress, the same as the terminal: the
 * header is a verdict, and "something is happening" is not one.
 */
@Composable
fun StatusGlyph(
    status: Status,
    prominent: Boolean = false,
    stale: Boolean = false,
    modifier: Modifier = Modifier,
) {
    val (mark, colour) = if (prominent) summary(status) else row(status, stale)
    Text(mark, style = Mono, color = colour, modifier = modifier)
}

/** The header table: green, red, or nothing resolved yet. */
private fun summary(status: Status): Pair<String, Color> = when (status) {
    Status.STATUS_PASSED -> "✓" to Ink.green
    Status.STATUS_FAILED -> "✗" to Ink.red
    else -> "·" to Ink.dim
}

/** The row table: shape carries the meaning, red carries the alarm. */
private fun row(status: Status, stale: Boolean): Pair<String, Color> = when (status) {
    Status.STATUS_PASSED -> "✓" to Ink.dim
    Status.STATUS_FAILED -> "✗" to if (stale) Ink.dim else Ink.red
    Status.STATUS_STARTED -> "⋯" to Ink.dim
    else -> "·" to Ink.dim
}
