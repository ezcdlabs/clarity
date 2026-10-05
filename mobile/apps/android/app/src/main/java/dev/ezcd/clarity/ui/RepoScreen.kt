package dev.ezcd.clarity.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.ContentCopy
import androidx.compose.material.icons.rounded.ExpandMore
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.ezcd.clarity.AppState
import dev.ezcd.clarity.ClarityModel
import dev.ezcd.clarity.proto.Batch
import dev.ezcd.clarity.proto.Commit
import dev.ezcd.clarity.proto.Flow
import dev.ezcd.clarity.proto.Section
import dev.ezcd.clarity.proto.SectionKind
import dev.ezcd.clarity.proto.Status
import dev.ezcd.clarity.proto.View
import dev.ezcd.clarity.subtitle
import dev.ezcd.clarity.title
import dev.ezcd.clarity.titlePrefix

/** The row grid, so section labels and author names share a left edge the way
 *  the terminal's dividers do. */
private val rowPadding = 16.dp
private val iconWidth = 14.dp
private val iconGap = 10.dp
private val subjectColumn = rowPadding + iconWidth + iconGap

/**
 * The vertical rhythm, which is the only thing saying what belongs to what.
 *
 * The terminal separates a deploy from the one above it with a blank line and
 * binds it to its own commits by adjacency. On a phone those two gaps have to
 * be visibly different sizes or the subheader floats between the batch above
 * and the batch below, attached to neither — which is exactly how it read.
 *
 * So: a commit's own two lines touch — no gap at all, they are one commit —
 * commits in a batch are a small gap apart, and the gap before a deploy line is
 * bigger than both put together. The gap *after* it is the smallest on the
 * screen, because what follows it is what it describes.
 */
private val commitGap = 14.dp
private val batchGapAbove = 30.dp
private val batchGapBelow = 3.dp

/**
 * One repository, in the same three-band shape the TUI draws: what has landed,
 * what is green, and what has shipped.
 *
 * Every grouping and status decision arrived settled in the view. What this
 * file decides is how wide things are and which colour they take, which is the
 * half of the job the proto boundary leaves to the platform.
 */
@Composable
fun RepoScreen(state: AppState, model: ClarityModel) {
    var tab by rememberSaveable(state.selected) { mutableIntStateOf(0) }
    var switcher by remember { mutableStateOf(false) }
    val view = state.view
    val flows = view?.flowsList.orEmpty()
    val flow = flows.getOrNull(tab.coerceAtMost((flows.size - 1).coerceAtLeast(0)))

    if (switcher) {
        Switcher(state, model) { switcher = false }
    }

    Column(Modifier.fillMaxSize().background(Ink.surface)) {
        AppBar(state, model) { switcher = true }
        if (view != null) {
            Strip(state, model, view, tab) { tab = it }
        }
        // On the chrome, not the sheet: a fetch belongs to the bar that started
        // it, and the sheet's turned corners would clip the ends off it.
        if (state.syncing) {
            LinearProgressIndicator(Modifier.fillMaxWidth(), color = Ink.blue, trackColor = Ink.line)
        }

        Sheet(topStartRadius = if (flows.size > 1 && tab == 0) 0 else 20) {
            Column(Modifier.fillMaxSize()) {
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

                CommitList(view, flow, state, model)
            }
        }
    }
}

@Composable
private fun CommitList(view: View, flow: Flow?, state: AppState, model: ClarityModel) {
    LazyColumn(Modifier.fillMaxSize()) {
            flow?.sectionsList?.forEach { section ->
            item(key = "s-${section.kind}") { SectionHeader(section) }

            items(section.commitsCount, key = { i -> section.getCommits(i).sha }) { i ->
                CommitRow(section.getCommits(i), state, model)
            }
            section.batchesList.forEachIndexed { b, batch ->
                val divided = batch.weekLabel.isNotEmpty()
                if (divided) {
                    item(key = "w-${section.kind}-$b") { WeekDivider(batch.weekLabel) }
                }
                item(key = "b-${section.kind}-$b") {
                    BatchHeader(batch, state, model, tight = divided || b == 0)
                }
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

/**
 * The bar, fixed at 64dp.
 *
 * The title is the switcher: two lines, the namespace stepped back in front of
 * the name, with the branch and host underneath. It does not collapse — the
 * whole title is a tap target, and a target that changes size and position as
 * you scroll is one you have to look at before you can hit it.
 */
@Composable
private fun AppBar(state: AppState, model: ClarityModel, onOpenSwitcher: () -> Unit) {
    val repo = state.repo
    Row(
        Modifier.fillMaxWidth().height(64.dp).padding(start = PageMargin, end = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(
            Modifier.weight(1f).clickable(onClick = onOpenSwitcher),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                if (repo != null && repo.titlePrefix.isNotEmpty()) {
                    Text(
                        repo.titlePrefix,
                        style = Type.titleNamespace,
                        color = Ink.dim,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f, fill = false),
                    )
                }
                Text(
                    repo?.title ?: state.view?.repoName ?: "clarity",
                    style = Type.title,
                    // Red when something is broken, as the terminal does with
                    // the repository name.
                    color = if (broken(state.view)) Ink.red else Ink.text,
                    maxLines = 1,
                )
                Icon(
                    Icons.Rounded.ExpandMore,
                    contentDescription = "Switch repository",
                    tint = Ink.dim,
                    modifier = Modifier.size(20.dp).padding(start = 2.dp),
                )
            }
            repo?.let {
                Text(it.subtitle, style = Type.mono, color = Ink.dim, maxLines = 1)
            }
        }
        RepoMenu(state, model)
    }
}

/** The per-repository menu from 3a. No refresh button in the bar — pull to
 *  refresh, or the first item here. */
@Composable
private fun RepoMenu(state: AppState, model: ClarityModel) {
    var open by remember { mutableStateOf(false) }
    val clipboard = LocalClipboardManager.current
    val repo = state.repo

    Box {
        GlyphButton(Icons.Rounded.MoreVert, "More") { open = true }
        DropdownMenu(
            expanded = open,
            onDismissRequest = { open = false },
            containerColor = Ink.menu,
            modifier = Modifier.width(252.dp),
        ) {
            DropdownMenuItem(
                text = {
                    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                        Text("Refresh now", style = Type.bodySmall, color = Ink.text, modifier = Modifier.weight(1f))
                        Text(viewAge(state, model), style = Type.mono, color = Ink.dim)
                    }
                },
                leadingIcon = { Icon(Icons.Rounded.Refresh, null, tint = Ink.dim) },
                onClick = {
                    open = false
                    model.refresh()
                },
            )
            DropdownMenuItem(
                text = { Text("Copy clone address", style = Type.bodySmall, color = Ink.text) },
                leadingIcon = { Icon(Icons.Rounded.ContentCopy, null, tint = Ink.dim) },
                enabled = repo != null,
                onClick = {
                    open = false
                    repo?.let { clipboard.setText(AnnotatedString(it.url)) }
                },
            )
        }
    }
}

/**
 * The summary strip: CI for the repository, deploy per flow.
 *
 * Naming the group is what says the two have different scopes — `deploy:`
 * labels the tabs, so they read as sub-items of deploy rather than as peers of
 * `ci`. The age of the view sits on the right and ticks, because a dashboard
 * that cannot say how old it is is a dashboard you have to trust.
 */
@Composable
private fun Strip(state: AppState, model: ClarityModel, view: View, tab: Int, onPick: (Int) -> Unit) {
    val flows = view.flowsList
    Row(
        Modifier.fillMaxWidth()
            .then(if (flows.size > 1) Modifier else Modifier.padding(bottom = 12.dp))
            .padding(start = PageMargin, end = PageMargin),
        verticalAlignment = Alignment.Bottom,
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("ci:", style = Type.monoMeta, color = Ink.dim)
            Spacer(Modifier.size(6.dp))
            StatusGlyph(view.ci, prominent = true, size = 16)
            Spacer(Modifier.size(18.dp))
            Text("deploy:", style = Type.monoMeta, color = Ink.dim)
        }

        if (flows.size <= 1) {
            Spacer(Modifier.size(6.dp))
            StatusGlyph(flows.firstOrNull()?.deploy ?: view.deploy, prominent = true, size = 16)
        }

        Spacer(Modifier.weight(1f))
        Text(viewAge(state, model), style = Type.monoMeta, color = Ink.dim, modifier = Modifier.padding(bottom = 2.dp))
    }

    if (flows.size > 1) {
        Row(
            Modifier.fillMaxWidth()
                .horizontalScroll(rememberScrollState())
                .padding(start = PageMargin, top = 4.dp),
        ) {
            flows.forEachIndexed { i, f ->
                val selected = i == tab
                Row(
                    Modifier
                        // The selected tab is cut out of the chrome and flush
                        // with the sheet below, so it reads as continuous with
                        // what it controls.
                        .clip(RoundedCornerShape(topStart = 12.dp, topEnd = 12.dp))
                        .background(if (selected) Ink.bg else Color.Transparent)
                        .clickable { onPick(i) }
                        .padding(start = 12.dp, end = 12.dp, top = 10.dp, bottom = 12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                ) {
                    Text(
                        if (f.undeclared) "${f.name} ?" else f.name,
                        style = Type.monoMeta,
                        color = if (selected) Ink.text else Ink.dim,
                    )
                    StatusGlyph(f.deploy, prominent = true, size = 16)
                }
            }
        }
    }
}

/** How old what you are looking at is, ticking. */
@Composable
private fun viewAge(state: AppState, model: ClarityModel): String {
    val view = state.view ?: return ""
    val anchor = view.generatedUnixSeconds
    if (anchor <= 0L || state.nowSeconds <= 0L) return ""
    return "${model.elapsed(state.nowSeconds - anchor)} ago"
}

/** Whether anything in this repository is currently failing. *//** Whether anything in this repository is currently failing. */
private fun broken(view: View?): Boolean =
    view != null && (view.ci == Status.STATUS_FAILED || view.deploy == Status.STATUS_FAILED)

/**
 * The lifecycle accents the TUI uses: HEAD neutral, CI Passed yellow, Deployed
 * blue. The colour marks the band, not a status — a yellow CI Passed header
 * does not mean anything is wrong.
 */
@Composable
private fun accent(kind: SectionKind): Color = when (kind) {
    SectionKind.SECTION_KIND_CI_PASSED -> Ink.yellow
    SectionKind.SECTION_KIND_DEPLOYED -> Ink.blue
    else -> Ink.text
}

@Composable
private fun SectionHeader(section: Section) {
    Column(Modifier.fillMaxWidth().padding(top = batchGapAbove)) {
        Row(
            Modifier.fillMaxWidth().padding(start = subjectColumn, end = rowPadding, bottom = 3.dp),
            verticalAlignment = Alignment.Bottom,
        ) {
            Text(
                section.label,
                color = accent(section.kind),
                fontSize = 13.sp,
                fontWeight = FontWeight.Bold,
                modifier = Modifier.weight(1f),
            )
            // This week's throughput rides on the Deployed rule rather than
            // taking a row of its own, as it does in the terminal — this week
            // is the one a reader is asking about.
            if (section.summary.isNotEmpty()) {
                Text(section.summary, color = Ink.dim, fontSize = 11.sp, fontStyle = FontStyle.Italic)
            }
        }
        Box(Modifier.fillMaxWidth().padding(horizontal = rowPadding).background(Ink.line).padding(top = 1.dp))
    }
}

/**
 * A week other than the current one, named above its first batch.
 *
 * The less-prominent sibling of the section rule, and right-aligned like it is
 * in the terminal: peripheral context about the rows below, not a row itself.
 */
@Composable
private fun WeekDivider(label: String) {
    Row(
        Modifier.fillMaxWidth().padding(
            start = subjectColumn,
            end = rowPadding,
            top = batchGapAbove,
            bottom = 0.dp,
        ),
        horizontalArrangement = Arrangement.End,
    ) {
        Text(label, color = Ink.dim, fontSize = 11.sp, fontStyle = FontStyle.Italic)
    }
}

@Composable
private fun BatchHeader(batch: Batch, state: AppState, model: ClarityModel, tight: Boolean) {
    val colour = when (batch.status) {
        Status.STATUS_FAILED -> Ink.red
        Status.STATUS_STARTED -> Ink.dim
        else -> Ink.blue
    }
    // One line, with the time inline, exactly as the terminal writes it:
    // "live on production · deployed 4m 43s ago". Pushing the time to the right
    // edge made it look like a column of its own and broke the sentence.
    val ago = ticking(state, model, batch.deployedUnixSeconds, batch.deployedAgo) { "$it ago" }
    Text(
        if (ago.isEmpty()) batch.label else "${batch.label} $ago",
        color = colour,
        fontSize = 12.sp,
        // The live batch is the present state rather than a past event, so it is
        // the one carrying weight.
        fontWeight = if (batch.live) FontWeight.Bold else FontWeight.Normal,
        fontStyle = if (batch.live) FontStyle.Normal else FontStyle.Italic,
        maxLines = 1,
        overflow = TextOverflow.Ellipsis,
        modifier = Modifier.fillMaxWidth().padding(
            start = subjectColumn,
            end = rowPadding,
            // Something already separated this one: the section rule it opens,
            // or the week divider naming it. Spending the gap twice would push
            // the subheader away from the commits it describes.
            top = if (tight) batchGapBelow else batchGapAbove,
            bottom = batchGapBelow,
        ),
    )
}

/**
 * One commit, on two lines.
 *
 * A phone is too narrow for the terminal's single row: at this width the
 * subject is the first thing to be clipped, and it is the thing you are reading
 * the list for. So the identity and the lead time share the top line, and the
 * subject gets the full width below, indented to line up under the author.
 *
 * No sha — the terminal does not print one, and nothing on a phone can be
 * copied out of a list row anyway. No age either: the only timer on a row is the
 * lead time, and a second one beside it invites the reader to work out which is
 * which.
 */
@Composable
private fun CommitRow(commit: Commit, state: AppState, model: ClarityModel) {
    Column(
        Modifier.fillMaxWidth().padding(start = rowPadding, end = rowPadding, bottom = commitGap),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            // One mark, not two. Whether this commit shipped is said by the band
            // and the batch it sits in, which is why the terminal has only ever
            // drawn the CI result here.
            StatusGlyph(commit.ci, stale = commit.ciStale, modifier = Modifier.width(iconWidth))

            Text(
                commit.author,
                color = Ink.dim,
                fontSize = 12.sp,
                lineHeight = 14.sp,
                style = Tight,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f).padding(start = iconGap),
            )

            if (commit.hasLeadTime) {
                // Grey while it runs, blue once the deploy that stopped the
                // clock landed — so a lead time blooms blue exactly when it
                // freezes, matching the Deployed band it came to rest in.
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

        Text(
            commit.subject,
            color = Ink.text,
            fontSize = 14.sp,
            // Tighter than the default for this size: a wrapped subject is one
            // sentence, and default leading opens a gap inside it wide enough to
            // compete with the gap between two commits.
            lineHeight = 17.sp,
            style = Tight,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
            // Nothing between the two lines. They are one commit, and the only
            // gap on this row that means anything is the one below it.
            modifier = Modifier.padding(start = iconWidth + iconGap),
        )
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
