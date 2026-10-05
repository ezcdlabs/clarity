package dev.ezcd.clarity.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.AltRoute
import androidx.compose.material.icons.rounded.Delete
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import dev.ezcd.clarity.ClarityModel
import dev.ezcd.clarity.proto.RepoSummary
import dev.ezcd.clarity.title

/** Renaming, which is local to this phone and clearable. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RenameSheet(repo: RepoSummary, model: ClarityModel, onDismiss: () -> Unit) {
    var name by remember(repo.id) { mutableStateOf(repo.alias) }
    val derived = (if (repo.namespace.isNotEmpty()) "${repo.namespace}/" else "") + repo.name

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        containerColor = Ink.surface,
        shape = RoundedCornerShape(topStart = 28.dp, topEnd = 28.dp),
    ) {
        Column(Modifier.padding(PageMargin).padding(bottom = 24.dp).imePadding()) {
            Text("Rename", style = Type.title, color = Ink.text)
            Spacer(Modifier.height(16.dp))
            OutlinedTextField(
                value = name,
                onValueChange = { name = it },
                label = { Text("Display name", style = Type.supporting) },
                singleLine = true,
                textStyle = Type.body,
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                shape = RoundedCornerShape(12.dp),
                modifier = Modifier.fillMaxWidth(),
            )
            Spacer(Modifier.height(8.dp))
            Text(
                "Only on this phone. Leave it empty to go back to $derived.",
                style = Type.supporting,
                color = Ink.dim,
            )
            Spacer(Modifier.height(20.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                TextButton(onClick = onDismiss) {
                    Text("Cancel", style = Type.buttonSmall, color = Ink.dim)
                }
                Spacer(Modifier.size(8.dp))
                PrimaryButton("Save") {
                    model.rename(repo.id, name)
                    onDismiss()
                }
            }
        }
    }
}

/** Removing, which deletes the local copy and nothing on the host. */
@Composable
fun RemoveDialog(repo: RepoSummary, model: ClarityModel, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(Icons.Rounded.Delete, null, tint = Ink.red) },
        title = { Text("Remove ${repo.title}?", style = Type.dialogTitle, color = Ink.text) },
        text = {
            Text(
                "Clarity stops watching it and deletes its local copy from this phone. " +
                    "Nothing changes on the host, and the device key stays authorised " +
                    "until you remove it there.",
                style = Type.bodySmall,
                color = Ink.dim,
            )
        },
        confirmButton = {
            TextButton(onClick = {
                model.removeRepo(repo.id)
                onDismiss()
            }) { Text("Remove", style = Type.buttonSmall, color = Ink.red) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text("Cancel", style = Type.buttonSmall, color = Ink.dim)
            }
        },
        containerColor = Ink.surface,
        shape = RoundedCornerShape(28.dp),
    )
}

/**
 * Changing which branch a repository watches.
 *
 * The same sheet as renaming, because they are the same shape of question —
 * but this one throws the view away and fetches again. Leaving the old
 * branch's commits on screen under a new branch's name would be the most
 * confusing possible outcome.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BranchSheet(repo: RepoSummary, model: ClarityModel, onDismiss: () -> Unit) {
    var branch by remember(repo.id) { mutableStateOf(repo.branch) }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        containerColor = Ink.surface,
        shape = RoundedCornerShape(topStart = 28.dp, topEnd = 28.dp),
    ) {
        Column(Modifier.padding(PageMargin).padding(bottom = 24.dp).imePadding()) {
            Text("Change branch", style = Type.title, color = Ink.text)
            Spacer(Modifier.height(16.dp))
            OutlinedTextField(
                value = branch,
                onValueChange = { branch = it },
                leadingIcon = { Icon(Icons.Rounded.AltRoute, null, tint = Ink.dim) },
                label = { Text("Branch", style = Type.supporting) },
                singleLine = true,
                textStyle = Type.mono,
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                shape = RoundedCornerShape(12.dp),
                modifier = Modifier.fillMaxWidth(),
            )
            Spacer(Modifier.height(8.dp))
            Text(
                "Clarity will fetch this branch instead. Leave it empty for main.",
                style = Type.supporting,
                color = Ink.dim,
            )
            Spacer(Modifier.height(20.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                TextButton(onClick = onDismiss) {
                    Text("Cancel", style = Type.buttonSmall, color = Ink.dim)
                }
                Spacer(Modifier.size(8.dp))
                PrimaryButton("Save") {
                    model.changeBranch(repo.id, branch)
                    onDismiss()
                }
            }
        }
    }
}
