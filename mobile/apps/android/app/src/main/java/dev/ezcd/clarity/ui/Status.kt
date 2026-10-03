package dev.ezcd.clarity.ui

import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import dev.ezcd.clarity.proto.Status

/**
 * The glyph and colour for a status.
 *
 * The proto deliberately sends the status and not a glyph, because the TUI's
 * ✓/✗ were chosen to survive a greyscale terminal. They survive here too, and
 * keeping them means someone who uses both reads the same marks — but colour is
 * never the only signal, which is the part that actually mattered.
 *
 * [stale] mutes a result a newer commit has already superseded: a build that
 * broke three commits ago and has since gone green is history, not an alarm.
 */
private fun glyph(status: Status, stale: Boolean): Pair<String, Color> = when (status) {
    Status.STATUS_PASSED -> "✓" to if (stale) Ink.dim else Ink.green
    Status.STATUS_FAILED -> "✗" to if (stale) Ink.dim else Ink.red
    Status.STATUS_STARTED -> "⋯" to Ink.yellow
    Status.STATUS_SKIPPED -> "–" to Ink.dim
    // Nothing reported is not a state to draw attention to: a commit nobody has
    // run anything against yet is normal, not pending.
    else -> "·" to Ink.line
}

@Composable
fun StatusGlyph(status: Status, stale: Boolean = false, modifier: Modifier = Modifier) {
    val (mark, colour) = glyph(status, stale)
    Text(mark, style = Mono, color = colour, modifier = modifier)
}
