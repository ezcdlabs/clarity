package dev.ezcd.clarity.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import dev.ezcd.clarity.ClarityModel
import dev.ezcd.clarity.Overlay

/** The page margin, and the column everything that is not an icon starts at. */
val PageMargin = 20.dp

/**
 * The whole app.
 *
 * One repository, with everything else happening in front of it: the switcher
 * is a bottom sheet, connect and the device key are full-screen flows. The
 * pager that used to put the list beside the repository is gone — a list of
 * three rows did not earn half the gesture space, and a sheet can be opened
 * from the title the user is already looking at.
 */
@Composable
fun App(model: ClarityModel, modifier: Modifier = Modifier) {
    val state by model.state.collectAsStateWithLifecycle()

    Box(modifier.fillMaxSize().background(Ink.surface)) {
        when (state.overlay) {
            Overlay.Connect -> ConnectScreen(state, model)
            Overlay.Key -> KeyScreen(state, model)
            null -> if (state.empty) EmptyScreen(model) else RepoScreen(state, model)
        }
    }
}

/**
 * The reading surface, laid on the chrome.
 *
 * The turned corners are what make the two tones read as a sheet on a ground
 * rather than as a join between two panels that failed to match — the levels
 * are deliberately close in tone, so the shape has to do the explaining.
 *
 * [topStartRadius] is squared off when a selected tab sits directly above, so
 * the tab and the sheet read as one surface.
 */
@Composable
fun Sheet(
    modifier: Modifier = Modifier,
    topStartRadius: Int = 20,
    content: @Composable () -> Unit,
) {
    Box(
        modifier
            .fillMaxSize()
            .clip(RoundedCornerShape(topStart = topStartRadius.dp, topEnd = 20.dp))
            .background(Ink.bg),
    ) { content() }
}

/**
 * An icon button.
 *
 * The label is never drawn, but it is what a screen reader announces and what a
 * long press surfaces. An unlabelled icon button is a button only sighted users
 * have, and this screen just traded most of its words for glyphs.
 */
@Composable
fun GlyphButton(
    icon: ImageVector,
    label: String,
    tint: Color = Ink.dim,
    enabled: Boolean = true,
    onClick: () -> Unit,
) {
    IconButton(onClick = onClick, enabled = enabled, modifier = Modifier.size(48.dp)) {
        Icon(icon, contentDescription = label, tint = if (enabled) tint else Ink.line)
    }
}

/** The one filled button a screen is allowed, fully rounded and 52dp tall. */
@Composable
fun PrimaryButton(
    text: String,
    modifier: Modifier = Modifier,
    icon: ImageVector? = null,
    enabled: Boolean = true,
    working: Boolean = false,
    onClick: () -> Unit,
) {
    Button(
        onClick = onClick,
        enabled = enabled && !working,
        shape = RoundedCornerShape(26.dp),
        colors = ButtonDefaults.buttonColors(
            containerColor = Ink.blue,
            // The background, so the label is dark on blue in dark mode and
            // light on blue in light. Material's default here leaks purple.
            contentColor = Ink.bg,
            disabledContainerColor = Ink.line,
            disabledContentColor = Ink.dim,
        ),
        modifier = modifier.height(52.dp),
    ) {
        if (working) {
            CircularProgressIndicator(
                Modifier.size(18.dp),
                color = Ink.dim,
                strokeWidth = 2.dp,
            )
        } else if (icon != null) {
            Icon(icon, contentDescription = null, modifier = Modifier.size(20.dp))
        }
        Text(text, style = Type.button, modifier = Modifier.padding(start = if (working || icon != null) 10.dp else 0.dp))
    }
}

/**
 * The error, if there is one, under whatever is on screen.
 *
 * Deliberately not a dialog: the messages come from the core and from git
 * itself, they are often long, and they usually describe why the thing behind
 * them is stale rather than why it is absent. Covering the data up to explain
 * that it is old would be the wrong trade.
 */
@Composable
fun ErrorBar(error: String?, onDismiss: () -> Unit) {
    if (error == null) return
    Row(
        Modifier.fillMaxWidth().background(Ink.errorBg).padding(start = PageMargin),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(
            error,
            style = Type.supporting,
            color = Ink.red,
            modifier = Modifier.weight(1f).padding(vertical = 10.dp),
        )
        GlyphButton(Icons.Rounded.Close, "Dismiss", onClick = onDismiss)
    }
}
