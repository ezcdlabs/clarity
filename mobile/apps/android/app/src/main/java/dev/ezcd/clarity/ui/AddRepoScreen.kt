package dev.ezcd.clarity.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.ezcd.clarity.AppState
import dev.ezcd.clarity.ClarityModel

/**
 * Paste a clone URL.
 *
 * Nothing is validated here. The registry already decides what a git remote
 * looks like and writes the message explaining a rejection; a second opinion in
 * Kotlin could only disagree with it.
 */
@Composable
fun AddRepoScreen(state: AppState, model: ClarityModel, modifier: Modifier = Modifier) {
    var url by rememberSaveable { mutableStateOf("") }
    var branch by rememberSaveable { mutableStateOf("") }

    Column(modifier.fillMaxSize().background(Ink.surface)) {
        TopBar("Add a repository", leading = { BackArrow { model.closeOverlay() } })

        Sheet {
        Column(Modifier.padding(16.dp)) {
            ErrorBar(state.error) { model.dismissError() }
            OutlinedTextField(
                value = url,
                onValueChange = { url = it },
                label = { Text("Clone URL") },
                placeholder = { Text("git@github.com:you/thing.git", style = Mono) },
                textStyle = Mono,
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = branch,
                onValueChange = { branch = it },
                label = { Text("Branch") },
                placeholder = { Text("main", style = Mono) },
                textStyle = Mono,
                singleLine = true,
                modifier = Modifier.fillMaxWidth().padding(top = 12.dp),
            )
            Text(
                "Leave the branch blank for main. An ssh URL uses this device's key — " +
                    "add it on the host first, under “key”.",
                color = Ink.dim,
                fontSize = 12.sp,
                modifier = Modifier.padding(top = 12.dp),
            )
            Button(
                onClick = { model.addRepo(url, branch) },
                enabled = !state.busy && url.isNotBlank(),
                modifier = Modifier.padding(top = 20.dp),
            ) {
                Text(if (state.busy) "Adding…" else "Add")
            }
        }
        }
    }
}
