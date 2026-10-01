package dev.ezcd.clarity.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.Tab
import androidx.compose.material3.TabRow
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.ezcd.clarity.AppState
import dev.ezcd.clarity.ClarityModel
import dev.ezcd.clarity.proto.Commit
import dev.ezcd.clarity.proto.Group

/**
 * One repository's commits, in the same three-section shape the TUI shows:
 * what has landed, what is green, and what has shipped.
 *
 * Every grouping and status decision arrived settled in the view — this file
 * only decides how wide things are, which is the half of the job the proto
 * boundary leaves to the platform.
 */
@Composable
fun RepoScreen(repoId: String, state: AppState, model: ClarityModel) {
    var tab by rememberSaveable(repoId) { mutableIntStateOf(0) }
    val view = state.view
    val flows = view?.flowsList.orEmpty()
    val flow = flows.getOrNull(tab.coerceAtMost((flows.size - 1).coerceAtLeast(0)))

    Column(Modifier.fillMaxSize()) {
        TopBar(state.repo?.name ?: view?.repoName ?: "", onBack = { model.back() }) {
            TextButton(onClick = { model.refresh() }, enabled = !state.syncing) {
                Text(if (state.syncing) "fetching…" else "refresh", color = Ink.blue, fontSize = 13.sp)
            }
        }
        if (state.syncing) {
            LinearProgressIndicator(Modifier.fillMaxWidth(), color = Ink.blue, trackColor = Ink.line)
        }
        ErrorBar(state.error) { model.dismissError() }

        // A single-target repo has exactly one flow, and a tab bar over one tab
        // is just a wasted row.
        if (flows.size > 1) {
            TabRow(selectedTabIndex = tab, containerColor = Ink.bg, contentColor = Ink.text) {
                flows.forEachIndexed { i, f ->
                    Tab(selected = i == tab, onClick = { tab = i }) {
                        Row(
                            Modifier.padding(vertical = 10.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            StatusGlyph(f.deploy)
                            Text(
                                if (f.undeclared) "${f.name} ?" else f.name,
                                color = if (i == tab) Ink.text else Ink.dim,
                                fontSize = 13.sp,
                                modifier = Modifier.padding(start = 6.dp),
                            )
                        }
                    }
                }
            }
        }

        if (view == null) {
            Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Text(
                    if (state.syncing) "Fetching…" else "Nothing fetched yet.",
                    color = Ink.dim,
                    fontSize = 14.sp,
                )
            }
            return@Column
        }

        LazyColumn(Modifier.fillMaxSize()) {
            flow?.groupsList?.forEach { group ->
                item(key = "g-${group.label}-${group.deployedUnixSeconds}") { GroupHeader(group) }
                items(group.commitsList.size, key = { i -> group.commitsList[i].sha }) { i ->
                    CommitRow(group.commitsList[i])
                }
            }
            if (view.truncated) {
                item {
                    Text(
                        "Showing the most recent ${view.limit} commits.",
                        color = Ink.dim,
                        fontSize = 12.sp,
                        modifier = Modifier.fillMaxWidth().padding(16.dp),
                    )
                }
            }
        }
    }
}

@Composable
private fun GroupHeader(group: Group) {
    Row(
        Modifier.fillMaxWidth().background(Ink.surface).padding(horizontal = 16.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        StatusGlyph(group.status)
        Text(group.label, color = Ink.text, fontSize = 13.sp, modifier = Modifier.weight(1f))
        if (group.deployedAgo.isNotEmpty()) {
            Text(group.deployedAgo, color = Ink.dim, fontSize = 12.sp)
        }
    }
}

@Composable
private fun CommitRow(commit: Commit) {
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 10.dp),
        horizontalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
            StatusGlyph(commit.ci)
            StatusGlyph(commit.deploy)
        }
        Column(Modifier.weight(1f)) {
            Text(
                commit.subject,
                color = Ink.text,
                fontSize = 14.sp,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(commit.shortSha, style = Mono.copy(fontSize = 11.sp), color = Ink.dim)
                Text(commit.author, color = Ink.dim, fontSize = 11.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(commit.age, color = Ink.dim, fontSize = 11.sp)
            }
        }
        if (commit.hasLeadTime) {
            // A live lead time is still counting, and saying so is the difference
            // between "this took 3m" and "this has taken 3m so far".
            Text(
                if (commit.leadTimeLive) "${commit.leadTime}…" else commit.leadTime,
                style = Mono.copy(fontSize = 12.sp),
                color = if (commit.leadTimeLive) Ink.yellow else Ink.dim,
            )
        }
    }
}
