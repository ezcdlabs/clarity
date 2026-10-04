package dev.ezcd.clarity.ui

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.ezcd.clarity.AppState
import dev.ezcd.clarity.ClarityModel

/**
 * The left page: every tracked repository, with the open one marked.
 *
 * Picking one only changes the selection. Sliding to the repository is the
 * pager's job, triggered by the selection changing — so the first launch and
 * the fallback after a removal carry you across too, not just a tap.
 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun ReposPane(state: AppState, model: ClarityModel) {
    Column(Modifier.fillMaxSize().background(Ink.surface)) {
        TopBar("clarity") {
            GlyphButton(Icons.Default.Add, "Add a repository", tint = Ink.blue) { model.showAddRepo() }
            // The device key is a once-ever action, so it goes where once-ever
            // actions go rather than taking a permanent seat in the bar.
            Overflow(model)
        }

        Sheet {
            RepoList(state, model)
        }
    }
}

@Composable
private fun Overflow(model: ClarityModel) {
    var open by remember { mutableStateOf(false) }
    Box {
        GlyphButton(Icons.Default.MoreVert, "More") { open = true }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            DropdownMenuItem(
                text = { Text("Device key", color = Ink.text, fontSize = 14.sp) },
                onClick = {
                    open = false
                    model.showKey()
                },
            )
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun RepoList(state: AppState, model: ClarityModel) {
    if (state.repos.isEmpty()) {
        Column(Modifier.padding(16.dp)) {
            Text("No repositories yet.", color = Ink.text, fontSize = 15.sp)
            Text(
                "Add one by pasting the URL you would clone. If it is an ssh URL, " +
                    "put this device's key on the host first.",
                color = Ink.dim,
                fontSize = 13.sp,
                modifier = Modifier.padding(top = 8.dp),
            )
        }
        return
    }

    LazyColumn(Modifier.fillMaxSize()) {
        items(state.repos, key = { it.id }) { repo ->
            val open = repo.id == state.selected
            var menu by remember(repo.id) { mutableStateOf(false) }

            Box {
                Row(
                    Modifier.fillMaxWidth()
                        // The open one is marked with the chrome tone, which is
                        // the lighter of the two and reads as lifted off the
                        // sheet.
                        .background(if (open) Ink.surface else Color.Transparent)
                        // Removing is behind a long press rather than a button
                        // on every row: it is rare, it is destructive, and a row
                        // whose only purpose is being tapped should not carry a
                        // second thing to tap by mistake.
                        .combinedClickable(
                            onClick = { model.select(repo.id) },
                            onLongClick = { menu = true },
                        )
                        .padding(horizontal = 16.dp, vertical = 14.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Column(Modifier.weight(1f)) {
                        Text(repo.name, color = if (open) Ink.text else Ink.dim, fontSize = 16.sp)
                        Text(
                            "${repo.url}  ${repo.branch}",
                            color = Ink.dim,
                            style = Mono.copy(fontSize = 11.sp),
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                    }
                }
                DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                    DropdownMenuItem(
                        text = { Text("Remove", color = Ink.red, fontSize = 14.sp) },
                        onClick = {
                            menu = false
                            model.removeRepo(repo.id)
                        },
                    )
                }
            }
        }
    }
}
