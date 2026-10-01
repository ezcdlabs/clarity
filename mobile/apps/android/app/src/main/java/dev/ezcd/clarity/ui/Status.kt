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
 */
private fun glyph(status: Status): Pair<String, Color> = when (status) {
    Status.STATUS_PASSED -> "✓" to Ink.green
    Status.STATUS_FAILED -> "✗" to Ink.red
    Status.STATUS_STARTED -> "⋯" to Ink.yellow
    Status.STATUS_SKIPPED -> "–" to Ink.dim
    // Nothing reported is not a state to draw attention to: a commit nobody has
    // run anything against yet is normal, not pending.
    else -> "·" to Ink.line
}

@Composable
fun StatusGlyph(status: Status, modifier: Modifier = Modifier) {
    val (mark, colour) = glyph(status)
    Text(mark, style = Mono, color = colour, modifier = modifier)
}
