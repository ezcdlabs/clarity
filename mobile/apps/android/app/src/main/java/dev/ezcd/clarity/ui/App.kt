package dev.ezcd.clarity.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import dev.ezcd.clarity.ClarityModel
import dev.ezcd.clarity.Overlay
import kotlinx.coroutines.launch

/** The repository list and the repository, in that order. */
private const val PAGE_REPOS = 0
private const val PAGE_REPO = 1

/**
 * The whole app: two pages side by side, the way Slack puts its channel list
 * beside the conversation.
 *
 * Not a drawer. A drawer opens from an edge and sits over what it covers; this
 * swipes from anywhere on the screen and the two pages are peers, so going back
 * to the list is the same gesture as going back to the repository. The burger
 * button does the same thing for anyone who does not go looking for a gesture.
 *
 * That the selection survives the trip is the model's doing, not the pager's —
 * moving to the list is not leaving the repository.
 *
 * Add-a-repository and the device key are overlays rather than pages, because
 * both are things you do once and then never again.
 */
@Composable
fun App(model: ClarityModel, modifier: Modifier = Modifier) {
    val state by model.state.collectAsStateWithLifecycle()

    when (state.overlay) {
        Overlay.AddRepo -> {
            AddRepoScreen(state, model, modifier)
            return
        }
        Overlay.Key -> {
            KeyScreen(state, model, modifier)
            return
        }
        null -> Unit
    }

    // A fresh install has nothing to show on the repository page, so it opens
    // on the list, where the only useful thing to do is add one.
    val pager = rememberPagerState(
        initialPage = if (state.selected == null) PAGE_REPOS else PAGE_REPO,
        pageCount = { 2 },
    )
    val scope = rememberCoroutineScope()

    // Picking a repository from the list carries you to it. Doing this here
    // rather than in the click handler means it also happens when the selection
    // changes for another reason — the first launch, or the fallback after a
    // removal.
    LaunchedEffect(state.selected) {
        if (state.selected != null && pager.currentPage == PAGE_REPOS) {
            pager.animateScrollToPage(PAGE_REPO)
        }
    }

    HorizontalPager(state = pager, modifier = modifier.fillMaxSize().background(Ink.bg)) { page ->
        when (page) {
            PAGE_REPOS -> ReposPane(state, model)
            else -> RepoScreen(state, model, onOpenList = {
                scope.launch { pager.animateScrollToPage(PAGE_REPOS) }
            })
        }
    }
}

/** A title row with a leading action and trailing content. */
@Composable
fun TopBar(
    title: String,
    leading: @Composable () -> Unit = {},
    trailing: @Composable () -> Unit = {},
) {
    Row(
        Modifier.fillMaxWidth().background(Ink.bg).padding(horizontal = 4.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        leading()
        Text(
            title,
            color = Ink.text,
            fontSize = 18.sp,
            modifier = Modifier.weight(1f).padding(start = 8.dp),
        )
        trailing()
    }
}

@Composable
fun BackArrow(onClick: () -> Unit) {
    IconButton(onClick = onClick) {
        Text("‹", style = Mono.copy(fontSize = 24.sp), color = Ink.dim)
    }
}

/**
 * The error, if there is one, under whatever is on screen.
 *
 * Deliberately not a dialog: the messages come from the core and from git
 * itself, they are often long, and they are usually describing why the thing
 * behind them is stale rather than why it is absent. Covering the data up to
 * explain that it is old would be the wrong trade.
 */
@Composable
fun ErrorBar(error: String?, onDismiss: () -> Unit) {
    if (error == null) return
    Row(
        Modifier.fillMaxWidth().background(Color(0xFF2A1416)).padding(start = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(error, color = Ink.red, fontSize = 13.sp, modifier = Modifier.weight(1f).padding(vertical = 10.dp))
        TextButton(onClick = onDismiss) { Text("dismiss", color = Ink.dim, fontSize = 13.sp) }
    }
}

/** Space where a section has nothing in it, so the frame still reads. */
@Composable
fun EmptyRow(text: String) {
    Box(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 10.dp)) {
        Text(text, color = Ink.line, fontSize = 13.sp)
    }
}
