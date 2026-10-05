package dev.ezcd.clarity.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.ContentCopy
import androidx.compose.material.icons.rounded.ExpandLess
import androidx.compose.material.icons.rounded.ExpandMore
import androidx.compose.material.icons.rounded.Key
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.ButtonDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp

/**
 * This device's public key, with what to do with it.
 *
 * Shared between the connect flow and the key screen, because they are the same
 * card with a different reason for being on screen.
 *
 * [startOpen] is false once any repository has connected with this key: the
 * first time you need the whole thing in front of you, and after that it is a
 * fact you occasionally check.
 */
@Composable
fun KeyCard(
    publicKey: String?,
    fingerprint: String?,
    startOpen: Boolean,
    collapsible: Boolean = true,
) {
    var open by remember(startOpen) { mutableStateOf(startOpen) }
    val clipboard = LocalClipboardManager.current

    Column(
        Modifier.fillMaxWidth()
            .clip(RoundedCornerShape(20.dp))
            .background(Ink.surface)
            .padding(16.dp),
    ) {
        Row(
            Modifier.fillMaxWidth()
                .then(if (collapsible) Modifier.clickable { open = !open } else Modifier),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(Icons.Rounded.Key, null, tint = Ink.dim, modifier = Modifier.size(18.dp))
            Column(Modifier.weight(1f).padding(start = 10.dp)) {
                Text(
                    if (open || !collapsible) "This device's key" else "Connects with this device's key",
                    style = Type.cardTitle,
                    color = Ink.text,
                )
                if (!open && collapsible && fingerprint != null) {
                    Text(
                        "ed25519 · $fingerprint",
                        style = Type.mono,
                        color = Ink.dim,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
            if (collapsible) {
                Icon(
                    if (open) Icons.Rounded.ExpandLess else Icons.Rounded.ExpandMore,
                    contentDescription = if (open) "Hide the key" else "Show the key",
                    tint = Ink.dim,
                )
            }
        }

        if (!open && collapsible) return@Column

        Spacer(Modifier.height(12.dp))
        Text(
            "Add it on your git host first, either as a read-only deploy key on " +
                "the repository or under your account's SSH keys.",
            style = Type.supporting,
            color = Ink.dim,
        )
        Spacer(Modifier.height(12.dp))

        Text(
            publicKey ?: "Generating…",
            style = Type.mono,
            color = Ink.text,
            modifier = Modifier.fillMaxWidth()
                .clip(RoundedCornerShape(12.dp))
                .background(Ink.bg)
                .padding(12.dp),
        )

        Spacer(Modifier.height(12.dp))
        // No fingerprint beside the button. With the whole key three lines
        // above it, a fingerprint here identifies something already on screen
        // — and at this width it could only ever be shown as a fragment, which
        // is a hash you cannot check against anything. It earns its place in
        // the collapsed row above, where the key is hidden, and in the
        // switcher, where there is no key at all.
        Row(
            Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.End,
        ) {
            TextButton(
                onClick = { publicKey?.let { clipboard.setText(AnnotatedString(it)) } },
                enabled = publicKey != null,
                shape = RoundedCornerShape(18.dp),
                colors = ButtonDefaults.textButtonColors(
                    containerColor = Ink.tonal,
                    contentColor = Ink.blue,
                ),
                modifier = Modifier.height(36.dp),
            ) {
                Icon(Icons.Rounded.ContentCopy, null, modifier = Modifier.size(16.dp))
                Text("Copy key", style = Type.buttonSmall, modifier = Modifier.padding(start = 6.dp))
            }
        }
    }
}
