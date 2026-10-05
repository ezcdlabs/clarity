package dev.ezcd.clarity.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.sharp.Check
import androidx.compose.material.icons.sharp.Close
import androidx.compose.material.icons.sharp.MoreHoriz
import androidx.compose.material3.Icon
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import dev.ezcd.clarity.proto.Status

/**
 * A status, as a mark.
 *
 * Material Symbols Sharp rather than the ✓ and ✗ characters the terminal uses.
 * The shapes still carry the meaning — that was the point of the terminal's
 * choice and it survives the move — but a glyph drawn by whichever font the
 * device happens to have is a glyph that is a different weight on every phone.
 *
 * [prominent] is where the colour goes. The header is the summary and earns it;
 * row marks deliberately forgo it and spend red only on a failure that is still
 * breaking something. A screen of green ticks spends the one colour that means
 * "look at this" on the state needing no attention at all.
 *
 * [stale] mutes a result a newer commit has already superseded: a build that
 * broke three commits ago and has since gone green is history, not an alarm.
 */
@Composable
fun StatusGlyph(
    status: Status,
    prominent: Boolean = false,
    stale: Boolean = false,
    size: Int = 18,
    modifier: Modifier = Modifier,
) {
    val colour = if (prominent) summaryColour(status) else rowColour(status, stale)

    when (status) {
        Status.STATUS_PASSED -> Mark(Icons.Sharp.Check, "passed", colour, size, modifier)
        Status.STATUS_FAILED -> Mark(Icons.Sharp.Close, "failed", colour, size, modifier)
        Status.STATUS_STARTED -> Mark(Icons.Sharp.MoreHoriz, "in progress", colour, size, modifier)
        else ->
            // Nothing reported. A small square rather than a shrunken icon:
            // it has to read as an empty slot, not as a mark too faint to make
            // out, and at this size any glyph would.
            Box(modifier.size(size.dp), contentAlignment = androidx.compose.ui.Alignment.Center) {
                Box(Modifier.size(4.dp).background(colour))
            }
    }
}

@Composable
private fun Mark(
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    label: String,
    colour: Color,
    size: Int,
    modifier: Modifier,
) {
    Icon(icon, contentDescription = label, tint = colour, modifier = modifier.size(size.dp))
}

/** The header table: green, red, or nothing resolved yet. */
@Composable
private fun summaryColour(status: Status): Color = when (status) {
    Status.STATUS_PASSED -> Ink.green
    Status.STATUS_FAILED -> Ink.red
    else -> Ink.dim
}

/** The row table: shape carries the meaning, red carries the alarm. */
@Composable
private fun rowColour(status: Status, stale: Boolean): Color = when (status) {
    Status.STATUS_FAILED -> if (stale) Ink.dim else Ink.red
    else -> Ink.dim
}
