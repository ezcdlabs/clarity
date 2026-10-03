package dev.ezcd.clarity.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.ezcd.clarity.AppState
import dev.ezcd.clarity.ClarityModel
import dev.ezcd.clarity.proto.Batch
import dev.ezcd.clarity.proto.Commit
import dev.ezcd.clarity.proto.Section
import dev.ezcd.clarity.proto.SectionKind
import dev.ezcd.clarity.proto.Status
import dev.ezcd.clarity.proto.View

/** The row grid, so section labels and author names share a left edge the way
 *  the terminal's dividers do. */
private val rowPadding = 16.dp
private val iconWidth = 14.dp
private val iconGap = 10.dp
private val authorColumn = 88.dp

/**
 * One repository, in the same three-band shape the TUI draws: what has landed,
 * what is green, and what has shipped.
 *
 * Every grouping and status decision arrived settled in the view. What this
 * file decides is how wide things are and which colour they take, which is the
 * half of the job the proto boundary leaves to the platform.
 */
@Composable
fun RepoScreen(state: AppState, model: ClarityModel, onOpenList: () -> Unit) {
    var tab by rememberSaveable(state.selected) { mutableIntStateOf(0) }
    val view = state.view
    val flows = view?.flowsList.orEmpty()
    val flow = flows.getOrNull(tab.coerceAtMost((flows.size - 1).coerceAtLeast(0)))

    Column(Modifier.fillMaxSize().background(Ink.bg)) {
        TopBar(
            title = state.repo?.name ?: view?.repoName ?: "clarity",
            // The name goes red when something is broken, as it does in the
            // terminal: the one place the repository is always named is the one
            // place a verdict is always visible.
            titleColor = if (broken(view)) Ink.red else Ink.text,
            leading = {
                // The list is a swipe away; this is for anyone who does not find
                // a gesture that has no affordance.
                IconButton(onClick = onOpenList) {
                    Text("≡", style = Mono.copy(fontSize = 20.sp), color = Ink.dim)
                }
            },
        ) {
            TextButton(onClick = { model.refresh() }, enabled = !state.syncing) {
                Text(if (state.syncing) "fetching…" else "refresh", color = Ink.blue, fontSize = 13.sp)
            }
        }
        if (view != null) {
            Header(view, tab) { tab = it }
        }
        if (state.syncing) {
            LinearProgressIndicator(Modifier.fillMaxWidth(), color = Ink.blue, trackColor = Ink.line)
        }
        ErrorBar(state.error) { model.dismissError() }

        if (view == null) {
            Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Text(
                    when {
                        state.syncing -> "Fetching…"
                        state.repos.isEmpty() -> "Add a repository to begin."
                        else -> "Nothing fetched yet."
                    },
                    color = Ink.dim,
                    fontSize = 14.sp,
                )
            }
            return@Column
        }

        LazyColumn(Modifier.fillMaxSize()) {
            flow?.sectionsList?.forEach { section ->
                item(key = "s-${section.kind}") { SectionHeader(section) }

                if (section.commitsCount == 0 && section.batchesCount == 0) {
                    item(key = "e-${section.kind}") { EmptyRow(emptyLine(section.kind)) }
                }
                items(section.commitsCount, key = { i -> section.getCommits(i).sha }) { i ->
                    CommitRow(section.getCommits(i), state, model)
                }
                section.batchesList.forEachIndexed { b, batch ->
                    item(key = "b-${section.kind}-$b") { BatchHeader(batch, state, model) }
                    items(batch.commitsCount, key = { i -> batch.getCommits(i).sha }) { i ->
                        CommitRow(batch.getCommits(i), state, model)
                    }
                }
            }
            if (view.truncated) {
                item {
                    Text(
                        "Showing the most recent ${view.limit} commits.",
                        color = Ink.dim,
                        fontSize = 12.sp,
                        modifier = Modifier.fillMaxWidth().padding(rowPadding),
                    )
                }
            }
        }
    }
}

/**
 * The header answers two questions with different scopes: CI is repo-wide,
 * deploys are per-flow. Naming the group is what says so — `deploy:` labels the
 * strip, so the flows read as sub-items of deploy rather than as peers of `ci`.
 *
 * One flow renders flat, with no bar, because a lone raised tab looks like a
 * control and is not one.
 */
@Composable
private fun Header(view: View, tab: Int, onPick: (Int) -> Unit) {
    val flows = view.flowsList
    if (flows.size <= 1) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = rowPadding, vertical = 6.dp),
            horizontalArrangement = Arrangement.spacedBy(16.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Badge("ci:", view.ci)
            Badge("deploy:", flows.firstOrNull()?.deploy ?: view.deploy)
        }
        return
    }

    // The selected flow is cut out of the chrome rather than raised above it:
    // the bar is painted one step off the background and the selected tab in the
    // background itself, so it is the only thing on the row sharing the body's
    // colour. That reads as a tab continuous with what it controls.
    Row(
        Modifier.fillMaxWidth().background(Ink.surface).horizontalScroll(rememberScrollState()),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Badge("ci:", view.ci, Modifier.padding(start = rowPadding, end = 16.dp))
        Text("deploy:", color = Ink.dim, fontSize = 13.sp)
        flows.forEachIndexed { i, f ->
            Row(
                Modifier
                    .background(if (i == tab) Ink.bg else Color.Transparent)
                    .clickable { onPick(i) }
                    .padding(horizontal = 10.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                // Undeclared: seen in the events but absent from .ezcd.json.
                Text(
                    if (f.undeclared) "${f.name} ?" else f.name,
                    color = if (i == tab) Ink.text else Ink.dim,
                    fontSize = 13.sp,
                )
                // Status sits after the name, matching `ci: ✓`.
                StatusGlyph(f.deploy, prominent = true)
            }
        }
    }
}

@Composable
private fun Badge(label: String, status: Status, modifier: Modifier = Modifier) {
    Row(
        modifier,
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        // Lowercase, as the header has always shipped.
        Text(label, color = Ink.dim, fontSize = 13.sp)
        StatusGlyph(status, prominent = true)
    }
}

/** Whether anything in this repository is currently failing. */
private fun broken(view: View?): Boolean =
    view != null && (view.ci == Status.STATUS_FAILED || view.deploy == Status.STATUS_FAILED)

/** What an empty band says, so the frame still reads as an answer. */
private fun emptyLine(kind: SectionKind): String = when (kind) {
    SectionKind.SECTION_KIND_HEAD -> "nothing waiting on CI"
    SectionKind.SECTION_KIND_CI_PASSED -> "nothing waiting to deploy"
    SectionKind.SECTION_KIND_DEPLOYED -> "nothing deployed yet"
    else -> ""
}

/**
 * The lifecycle accents the TUI uses: HEAD neutral, CI Passed yellow, Deployed
 * blue. The colour marks the band, not a status — a yellow CI Passed header
 * does not mean anything is wrong.
 */
private fun accent(kind: SectionKind): Color = when (kind) {
    SectionKind.SECTION_KIND_CI_PASSED -> Ink.yellow
    SectionKind.SECTION_KIND_DEPLOYED -> Ink.blue
    else -> Ink.text
}

@Composable
private fun SectionHeader(section: Section) {
    Column(Modifier.fillMaxWidth().padding(top = 18.dp)) {
        Text(
            section.label,
            color = accent(section.kind),
            fontSize = 13.sp,
            fontWeight = FontWeight.Bold,
            // Indented to the author column, so a label and the names beneath it
            // share a left edge the way the terminal's dividers do.
            modifier = Modifier.padding(start = rowPadding + iconWidth + iconGap, bottom = 4.dp),
        )
        Box(Modifier.fillMaxWidth().padding(horizontal = rowPadding).background(Ink.line).padding(top = 1.dp))
    }
}

@Composable
private fun BatchHeader(batch: Batch, state: AppState, model: ClarityModel) {
    val colour = when (batch.status) {
        Status.STATUS_FAILED -> Ink.red
        Status.STATUS_STARTED -> Ink.dim
        else -> Ink.blue
    }
    Row(
        Modifier.fillMaxWidth().padding(
            start = rowPadding + iconWidth + iconGap,
            end = rowPadding,
            top = 12.dp,
            bottom = 2.dp,
        ),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            batch.label,
            color = colour,
            fontSize = 12.sp,
            // The live batch is the present state rather than a past event, so
            // it is the one that is not italic.
            fontWeight = if (batch.live) FontWeight.Bold else FontWeight.Normal,
            modifier = Modifier.weight(1f),
        )
        val ago = ticking(state, model, batch.deployedUnixSeconds, batch.deployedAgo) { "$it ago" }
        if (ago.isNotEmpty()) {
            Text(ago, color = Ink.dim, fontSize = 12.sp)
        }
    }
}

/**
 * One commit, laid out the way the terminal lays one out: a status icon, the
 * author, the subject, and a right-aligned lead time. One line.
 *
 * No sha — the terminal does not print one, and nothing on a phone can be
 * copied out of a list row anyway. No age either: the only timer on a row is the
 * lead time, and a second one beside it invites the reader to work out which is
 * which.
 */
@Composable
private fun CommitRow(commit: Commit, state: AppState, model: ClarityModel) {
    Row(
        Modifier.fillMaxWidth().padding(horizontal = rowPadding, vertical = 7.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        // One mark, not two. Whether this commit shipped is said by the band and
        // the batch it sits in, which is why the terminal has only ever drawn
        // the CI result here.
        StatusGlyph(commit.ci, stale = commit.ciStale, modifier = Modifier.width(iconWidth))

        Text(
            commit.author,
            color = Ink.dim,
            fontSize = 13.sp,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            // A fixed column, so subjects start at the same place down the list
            // rather than stepping in and out with the length of a name.
            modifier = Modifier.padding(start = iconGap).width(authorColumn),
        )

        Text(
            commit.subject,
            color = Ink.text,
            fontSize = 13.sp,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f).padding(start = 8.dp),
        )

        if (commit.hasLeadTime) {
            // Grey while it runs, blue once the deploy that stopped the clock
            // landed — so a lead time blooms blue exactly when it freezes,
            // matching the Deployed band it came to rest in.
            Text(
                if (commit.leadTimeLive) {
                    ticking(state, model, commit.leadTimeAnchorUnixSeconds, commit.leadTime) { it }
                } else {
                    commit.leadTime
                },
                style = Mono.copy(fontSize = 12.sp),
                color = if (commit.leadTimeLive) Ink.dim else Ink.blue,
                maxLines = 1,
                modifier = Modifier.padding(start = 8.dp),
            )
        }
    }
}

/**
 * A duration that keeps counting between fetches.
 *
 * The view arrives with its durations preformatted, which is right for the
 * instant it was built and wrong a second later. Given an anchor, this
 * recomputes against the model's clock; without one — or before the clock has
 * started — it falls back to what the view said, which is never worse than what
 * the last fetch showed.
 */
@Composable
private fun ticking(
    state: AppState,
    model: ClarityModel,
    anchorUnixSeconds: Long,
    fallback: String,
    format: (String) -> String,
): String {
    if (anchorUnixSeconds <= 0L || state.nowSeconds <= 0L) return fallback
    return format(model.elapsed(state.nowSeconds - anchorUnixSeconds))
}
