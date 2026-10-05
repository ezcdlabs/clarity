package dev.ezcd.clarity.ui

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
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
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.Delete
import androidx.compose.material.icons.rounded.Key
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.ezcd.clarity.AppState
import dev.ezcd.clarity.ClarityModel
import dev.ezcd.clarity.proto.RepoSummary
import dev.ezcd.clarity.subtitle
import dev.ezcd.clarity.title
import dev.ezcd.clarity.titlePrefix

/**
 * The repository switcher.
 *
 * A sheet rather than a page, because switching is a thing you do *to* the
 * screen you are looking at rather than somewhere you go. Every row carries how
 * that repository is doing, from the last view it had — so the answer to "is
 * anything red?" is in the list, not one tap into each.
 */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalFoundationApi::class)
@Composable
fun Switcher(state: AppState, model: ClarityModel, onDismiss: () -> Unit) {
    var renaming by remember { mutableStateOf<RepoSummary?>(null) }
    var removing by remember { mutableStateOf<RepoSummary?>(null) }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        containerColor = Ink.surface,
        shape = RoundedCornerShape(topStart = 28.dp, topEnd = 28.dp),
    ) {
        Column(Modifier.padding(horizontal = 12.dp).padding(bottom = 24.dp)) {
            Row(
                Modifier.fillMaxWidth().padding(start = 8.dp, end = 4.dp, bottom = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text("Repositories", style = Type.title, color = Ink.text, modifier = Modifier.weight(1f))
                TextButton(
                    onClick = {
                        onDismiss()
                        model.showConnect()
                    },
                    shape = RoundedCornerShape(18.dp),
                    colors = ButtonDefaults.textButtonColors(
                        containerColor = Ink.tonal, contentColor = Ink.blue,
                    ),
                ) {
                    Icon(Icons.Rounded.Add, null, modifier = Modifier.size(16.dp))
                    Text("Add", style = Type.buttonSmall, modifier = Modifier.padding(start = 6.dp))
                }
            }

            state.repos.forEach { repo ->
                SwitcherRow(
                    repo = repo,
                    current = repo.id == state.selected,
                    onPick = {
                        model.select(repo.id)
                        onDismiss()
                    },
                    onRename = { renaming = repo },
                    onRemove = { removing = repo },
                )
                Spacer(Modifier.height(2.dp))
            }

            Spacer(Modifier.height(8.dp))
            HorizontalDivider(color = Ink.line)
            Spacer(Modifier.height(8.dp))

            Row(
                Modifier.fillMaxWidth()
                    .clip(RoundedCornerShape(16.dp))
                    .combinedClickable(onClick = {
                        onDismiss()
                        model.showKey()
                    })
                    .padding(12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Icon(Icons.Rounded.Key, null, tint = Ink.dim, modifier = Modifier.size(18.dp))
                // Just the label. This row is a way through to the key, and a
                // fingerprint clipped to fit is one nobody can check against
                // anything — which is the only thing a fingerprint is for. The
                // whole key is one tap away.
                Text(
                    "Device key",
                    style = Type.bodySmall,
                    color = Ink.text,
                    maxLines = 1,
                    modifier = Modifier.weight(1f).padding(start = 10.dp),
                )
            }
        }
    }

    renaming?.let { repo ->
        RenameSheet(repo, model) { renaming = null }
    }
    removing?.let { repo ->
        RemoveDialog(repo, model) { removing = null }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun SwitcherRow(
    repo: RepoSummary,
    current: Boolean,
    onPick: () -> Unit,
    onRename: () -> Unit,
    onRemove: () -> Unit,
) {
    var menu by remember(repo.id) { mutableStateOf(false) }
    val broken = repo.ci == dev.ezcd.clarity.proto.Status.STATUS_FAILED ||
        repo.flowsList.any { it.deploy == dev.ezcd.clarity.proto.Status.STATUS_FAILED }

    Box {
        Row(
            Modifier.fillMaxWidth()
                .clip(RoundedCornerShape(16.dp))
                .background(if (current) Ink.line else Color.Transparent)
                // Rename and remove are rare and one of them is destructive, so
                // they live behind a press rather than taking a control each on
                // every row.
                .combinedClickable(onClick = onPick, onLongClick = { menu = true })
                .padding(12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(Modifier.weight(1f)) {
                Row {
                    if (repo.titlePrefix.isNotEmpty()) {
                        Text(
                            repo.titlePrefix,
                            style = Type.cardTitle.copy(fontWeight = androidx.compose.ui.text.font.FontWeight(400)),
                            color = Ink.dim,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            modifier = Modifier.weight(1f, fill = false),
                        )
                    }
                    // The name never truncates; the namespace in front of it
                    // yields first.
                    Text(
                        repo.title,
                        style = Type.cardTitle,
                        color = if (broken) Ink.red else Ink.text,
                        maxLines = 1,
                    )
                }
                Text(repo.subtitle, style = Type.mono, color = Ink.dim, maxLines = 1)
            }

            Spacer(Modifier.size(10.dp))
            RowStatus(repo)
        }

        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }, containerColor = Ink.menu) {
            DropdownMenuItem(
                text = { Text("Rename…", style = Type.bodySmall, color = Ink.text) },
                onClick = { menu = false; onRename() },
            )
            DropdownMenuItem(
                text = { Text("Remove", style = Type.bodySmall, color = Ink.red) },
                leadingIcon = { Icon(Icons.Rounded.Delete, null, tint = Ink.red) },
                onClick = { menu = false; onRemove() },
            )
        }
    }
}

/**
 * How a repository is doing, in the space a list row has.
 *
 * One glyph per flow and no names: a flow is never hidden, because the stuck
 * one is the one worth seeing and would be the one dropped. The description a
 * screen reader gets does name them, since it has no width to run out of.
 */
@Composable
private fun RowStatus(repo: RepoSummary) {
    val described = buildString {
        append("ci ${spoken(repo.ci)}")
        repo.flowsList.forEach { append(", ${it.name} ${spoken(it.deploy)}") }
    }
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
        modifier = Modifier.describedAs(described),
    ) {
        Text("ci", style = Type.mono, color = Ink.dim)
        StatusGlyph(repo.ci, prominent = true, size = 16)
        Spacer(Modifier.size(2.dp))
        Text("deploy", style = Type.mono, color = Ink.dim)
        if (repo.flowsCount == 0) {
            StatusGlyph(repo.deploy, prominent = true, size = 16)
        } else {
            repo.flowsList.forEach { StatusGlyph(it.deploy, prominent = true, size = 16) }
        }
    }
}

private fun spoken(status: dev.ezcd.clarity.proto.Status) = when (status) {
    dev.ezcd.clarity.proto.Status.STATUS_PASSED -> "passed"
    dev.ezcd.clarity.proto.Status.STATUS_FAILED -> "failed"
    dev.ezcd.clarity.proto.Status.STATUS_STARTED -> "in progress"
    else -> "none"
}

/** One description for the whole status group, so a screen reader reads "ci
 *  passed, web passed, ios failed" rather than four unexplained marks. */
private fun Modifier.describedAs(description: String) =
    this.semantics(mergeDescendants = true) { contentDescription = description }
