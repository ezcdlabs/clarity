package dev.ezcd.clarity.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.ezcd.clarity.AppState
import dev.ezcd.clarity.ClarityModel

/**
 * This device's public key, for pasting into whichever host the user uses.
 *
 * There is no provider integration and no OAuth app, because clarity is not a
 * GitHub tool — a key the user installs themselves works on GitHub, GitLab, and
 * a box in a cupboard equally.
 */
@Composable
fun KeyScreen(state: AppState, model: ClarityModel, modifier: Modifier = Modifier) {
    val clipboard = LocalClipboardManager.current

    Column(modifier.fillMaxSize().background(Ink.surface)) {
        TopBar("Device key", leading = { BackArrow { model.closeOverlay() } })

        Sheet {
        Column(Modifier.padding(16.dp).verticalScroll(rememberScrollState())) {
            ErrorBar(state.error) { model.dismissError() }
            Text(
                "Add this as a deploy key or an account key on the host, with read " +
                    "access. The private half never leaves this device.",
                color = Ink.dim,
                fontSize = 13.sp,
            )
            val key = state.publicKey
            if (key == null) {
                Text("Generating…", color = Ink.dim, fontSize = 13.sp, modifier = Modifier.padding(top = 16.dp))
            } else {
                Text(
                    key,
                    style = Mono.copy(fontSize = 12.sp),
                    color = Ink.text,
                    modifier = Modifier.fillMaxWidth()
                        .padding(top = 16.dp)
                        .background(Ink.surface)
                        .padding(12.dp),
                )
                Button(
                    onClick = { clipboard.setText(AnnotatedString(key)) },
                    modifier = Modifier.padding(top = 16.dp),
                ) { Text("Copy") }
            }
        }
        }
    }
}
