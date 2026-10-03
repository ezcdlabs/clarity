package dev.ezcd.clarity.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
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
@Composable
fun ReposPane(state: AppState, model: ClarityModel) {
    Column(Modifier.fillMaxSize().background(Ink.bg)) {
        TopBar("clarity") {
            TextButton(onClick = { model.showKey() }) { Text("key", color = Ink.dim, fontSize = 13.sp) }
            TextButton(onClick = { model.showAddRepo() }) { Text("add", color = Ink.blue, fontSize = 13.sp) }
        }

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
            return@Column
        }

        LazyColumn(Modifier.fillMaxSize()) {
            items(state.repos, key = { it.id }) { repo ->
                val open = repo.id == state.selected
                Row(
                    Modifier.fillMaxWidth()
                        .background(if (open) Ink.surface else Ink.bg)
                        .clickable { model.select(repo.id) }
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
                    TextButton(onClick = { model.removeRepo(repo.id) }) {
                        Text("remove", color = Ink.dim, fontSize = 12.sp)
                    }
                }
            }
        }
    }
}
