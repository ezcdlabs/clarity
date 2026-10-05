package dev.ezcd.clarity.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import dev.ezcd.clarity.AppState
import dev.ezcd.clarity.ClarityModel

/**
 * This device's public key, on its own.
 *
 * Reached from the switcher once there are repositories, and from the empty
 * state's overflow before there are — because the key is the thing you need
 * *before* the first connection, and the host wants it pasted in.
 */
@Composable
fun KeyScreen(state: AppState, model: ClarityModel) {
    LaunchedEffect(Unit) { model.loadKey() }

    Column(Modifier.fillMaxSize().background(Ink.bg)) {
        Row(
            Modifier.fillMaxWidth().height(64.dp).padding(start = 4.dp, end = 4.dp),
            horizontalArrangement = Arrangement.Start,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            GlyphButton(Icons.Rounded.Close, "Close") { model.closeOverlay() }
            Text(
                "Device key",
                style = Type.appBarTitle,
                color = Ink.text,
                modifier = Modifier.padding(start = 4.dp),
            )
        }

        Column(
            Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(PageMargin),
        ) {
                ErrorBar(state.error) { model.dismissError() }
                Text(
                    "Add this as a deploy key on a repository, or under your account's " +
                        "SSH keys, with read access. The private half never leaves this phone.",
                    style = Type.bodySmall,
                    color = Ink.dim,
                )
                Spacer(Modifier.height(20.dp))
            KeyCard(
                publicKey = state.publicKey,
                startOpen = true,
                collapsible = false,
            )
        }
    }
}
