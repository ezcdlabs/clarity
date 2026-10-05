package dev.ezcd.clarity.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.AltRoute
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.Error
import androidx.compose.material.icons.rounded.Fingerprint
import androidx.compose.material.icons.rounded.Lock
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.material.icons.rounded.Terminal
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Icon
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import dev.ezcd.clarity.AppState
import dev.ezcd.clarity.ClarityModel
import dev.ezcd.clarity.Connect

/**
 * Connecting a repository: one flow rather than a form and a surprise.
 *
 * Everything that can go wrong on a first connection is a step on the way
 * rather than an error thrown back at the field — an unknown host is a question
 * with an answer, and a refused key is a thing to go and fix with the key right
 * there to copy.
 */
@Composable
fun ConnectScreen(state: AppState, model: ClarityModel) {
    var url by rememberSaveable { mutableStateOf("") }
    var branch by rememberSaveable { mutableStateOf("") }
    val working = state.connect is Connect.Working
    val denied = state.connect as? Connect.Denied

    Column(Modifier.fillMaxSize().background(Ink.bg).imePadding()) {
        Row(Modifier.fillMaxWidth().height(64.dp).padding(start = 4.dp), Arrangement.Start, Alignment.CenterVertically) {
            // Cancel stays live while connecting: a fetch that is going nowhere
            // is exactly when someone wants out.
            GlyphButton(Icons.Rounded.Close, "Cancel") { model.cancelConnect() }
        }

        Column(
            Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = PageMargin),
        ) {
                Spacer(Modifier.height(8.dp))
                Text("Connect a repository", style = Type.pageTitle, color = Ink.text)
                Spacer(Modifier.height(8.dp))
                Text(
                    "Clarity reads it over SSH, like git fetch.",
                    style = Type.bodySmall,
                    color = Ink.dim,
                )
                Spacer(Modifier.height(24.dp))

                OutlinedTextField(
                    value = url,
                    onValueChange = { url = it },
                    enabled = !working,
                    isError = denied != null,
                    label = { Text("SSH clone address", style = Type.supporting) },
                    placeholder = { Text("git@github.com:you/thing.git", style = Type.mono) },
                    textStyle = Type.mono,
                    singleLine = true,
                    trailingIcon = if (denied != null) {
                        { Icon(Icons.Rounded.Error, "Refused", tint = Ink.red) }
                    } else {
                        null
                    },
                    keyboardOptions = KeyboardOptions(
                        keyboardType = KeyboardType.Uri,
                        autoCorrectEnabled = false,
                        imeAction = ImeAction.Go,
                    ),
                    shape = RoundedCornerShape(12.dp),
                    colors = fieldColours(),
                    modifier = Modifier.fillMaxWidth(),
                )

                Spacer(Modifier.height(12.dp))
                OutlinedTextField(
                    value = branch,
                    onValueChange = { branch = it },
                    enabled = !working,
                    leadingIcon = { Icon(Icons.Rounded.AltRoute, null, tint = Ink.dim) },
                    placeholder = { Text("main", style = Type.mono) },
                    trailingIcon = { Text("branch", style = Type.supporting, color = Ink.dim, modifier = Modifier.padding(end = 12.dp)) },
                    textStyle = Type.mono,
                    singleLine = true,
                    shape = RoundedCornerShape(12.dp),
                    colors = fieldColours(),
                    modifier = Modifier.fillMaxWidth(),
                )

                Spacer(Modifier.height(20.dp))

                when (val c = state.connect) {
                    is Connect.Working -> Progress(state)
                    is Connect.Denied -> DeniedCard(c)
                    is Connect.Failed -> Text(c.message, style = Type.supporting, color = Ink.red)
                    else -> Unit
                }

                if (state.connect !is Connect.Working) {
                    Spacer(Modifier.height(20.dp))
                    KeyCard(
                        publicKey = state.publicKey,
                        // Forced open when the host has just refused it: that
                        // is the moment the key is the whole answer.
                        startOpen = denied != null || !state.keyHasConnected,
                        collapsible = denied == null,
                    )
                }

                Spacer(Modifier.height(24.dp))
                PrimaryButton(
                    text = when {
                        working -> "Connecting…"
                        denied != null -> "Try again"
                        else -> "Connect"
                    },
                    icon = if (denied != null) Icons.Rounded.Refresh else null,
                    enabled = url.isNotBlank(),
                    working = working,
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    if (denied != null) model.retryConnect() else model.connect(url, branch)
                }
            Spacer(Modifier.height(24.dp))
        }
    }

    (state.connect as? Connect.AskHost)?.let { HostKeyDialog(it, model) }
}

/**
 * What is happening, while it happens.
 *
 * The core reports one thing — reached, or not — so this says that one thing
 * rather than inventing stages it cannot actually observe. A fake progress list
 * is a lie that gets found out the first time something hangs.
 */
@Composable
private fun Progress(state: AppState) {
    Column(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(20.dp)).background(Ink.surface).padding(16.dp),
    ) {
        Text("CONNECTING", style = Type.overline, color = Ink.dim)
        Spacer(Modifier.height(10.dp))
        Text(
            "Reading the branch and clarity's events over SSH. The first fetch " +
                "reads the recent history; later ones only fetch what changed.",
            style = Type.supporting,
            color = Ink.dim,
        )
    }
}

/** The host refused the key. Fixable, and the fix is on this screen. */
@Composable
private fun DeniedCard(denied: Connect.Denied) {
    var showOutput by remember { mutableStateOf(false) }
    Column(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(20.dp)).background(Ink.errorBg).padding(16.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Rounded.Lock, null, tint = Ink.red, modifier = Modifier.size(18.dp))
            Text(
                "The host didn't accept this device's key",
                style = Type.cardTitle,
                color = Ink.text,
                modifier = Modifier.padding(start = 10.dp),
            )
        }
        Spacer(Modifier.height(8.dp))
        Text(
            "Add the key below to the repository's deploy keys — read access is " +
                "enough — then try again.",
            style = Type.supporting,
            color = Ink.dim,
        )
        if (denied.gitOutput.isNotEmpty()) {
            Spacer(Modifier.height(10.dp))
            Row(
                Modifier.clickable { showOutput = !showOutput },
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Icon(Icons.Rounded.Terminal, null, tint = Ink.dim, modifier = Modifier.size(16.dp))
                Text(
                    if (showOutput) "Hide git output" else "Show git output",
                    style = Type.supporting,
                    color = Ink.blue,
                    modifier = Modifier.padding(start = 8.dp),
                )
            }
            if (showOutput) {
                Spacer(Modifier.height(8.dp))
                // Git's own words, not a summary of them: whoever is debugging
                // a key wants the thing the host actually said.
                Text(denied.gitOutput, style = Type.mono, color = Ink.dim)
            }
        }
    }
}

/**
 * The first connection to a host.
 *
 * Clarity used to trust whatever answered and write it down. This is the prompt
 * that replaced it — and it can be answered no, which is the only thing that
 * makes it worth asking.
 */
@Composable
private fun HostKeyDialog(asking: Connect.AskHost, model: ClarityModel) {
    AlertDialog(
        onDismissRequest = { model.cancelConnect() },
        icon = { Icon(Icons.Rounded.Fingerprint, null, tint = if (asking.changed) Ink.red else Ink.blue) },
        title = {
            Text(
                if (asking.changed) "Host key changed" else "Trust this host?",
                style = Type.dialogTitle,
                color = if (asking.changed) Ink.red else Ink.text,
            )
        },
        text = {
            Column {
                Text(
                    if (asking.changed) {
                        "The key ${asking.key.host} is presenting is not the one this " +
                            "device trusted before. That can mean the host was rebuilt — " +
                            "or that something is between you and it."
                    } else {
                        "This is the first connection to ${asking.key.host}. Check that " +
                            "this fingerprint matches the one your host publishes."
                    },
                    style = Type.bodySmall,
                    color = Ink.dim,
                )
                Spacer(Modifier.height(14.dp))
                Column(
                    Modifier.fillMaxWidth().clip(RoundedCornerShape(12.dp)).background(Ink.bg).padding(12.dp),
                ) {
                    Text(asking.key.type, style = Type.overline, color = Ink.dim)
                    Spacer(Modifier.height(4.dp))
                    Text(asking.key.fingerprint, style = Type.mono, color = Ink.text)
                }
                Spacer(Modifier.height(12.dp))
                Text(
                    "Clarity remembers it. If it ever changes, you'll be asked again.",
                    style = Type.supporting,
                    color = Ink.dim,
                )
            }
        },
        confirmButton = {
            PrimaryButton("Trust and continue") { model.trustHost() }
        },
        dismissButton = {
            TextButton(onClick = { model.cancelConnect() }) {
                Text("Cancel", style = Type.buttonSmall, color = Ink.dim)
            }
        },
        containerColor = Ink.surface,
        shape = RoundedCornerShape(28.dp),
    )
}

@Composable
private fun fieldColours() = OutlinedTextFieldDefaults.colors(
    focusedTextColor = Ink.text,
    unfocusedTextColor = Ink.text,
    disabledTextColor = Ink.dim,
    focusedBorderColor = Ink.blue,
    unfocusedBorderColor = Ink.line,
    disabledBorderColor = Ink.line,
    errorBorderColor = Ink.red,
    focusedLabelColor = Ink.blue,
    unfocusedLabelColor = Ink.dim,
    cursorColor = Ink.blue,
)
