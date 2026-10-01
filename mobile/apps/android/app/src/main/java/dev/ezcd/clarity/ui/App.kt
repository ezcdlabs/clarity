package dev.ezcd.clarity.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import dev.ezcd.clarity.ClarityModel
import dev.ezcd.clarity.Screen

/**
 * The whole app: one state value in, one screen out.
 *
 * Routing is a `when` over [Screen] rather than a navigation library. There are
 * four destinations and no deep links, and a back stack here would be a second
 * place for state to live.
 */
@Composable
fun App(model: ClarityModel, modifier: Modifier = Modifier) {
    val state by model.state.collectAsStateWithLifecycle()

    Column(modifier.fillMaxSize().background(Ink.bg)) {
        when (val screen = state.screen) {
            Screen.Repos -> ReposScreen(state, model)
            Screen.AddRepo -> AddRepoScreen(state, model)
            Screen.Key -> KeyScreen(state, model)
            is Screen.Repo -> RepoScreen(screen.id, state, model)
        }
    }
}

/** A title row with an optional back arrow and trailing content. */
@Composable
fun TopBar(
    title: String,
    onBack: (() -> Unit)? = null,
    trailing: @Composable () -> Unit = {},
) {
    Row(
        Modifier.fillMaxWidth().background(Ink.bg).padding(horizontal = 4.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (onBack != null) {
            IconButton(onClick = onBack) {
                Text("‹", style = Mono.copy(fontSize = 24.sp), color = Ink.dim)
            }
        } else {
            Box(Modifier.padding(start = 12.dp))
        }
        Text(title, color = Ink.text, fontSize = 18.sp, modifier = Modifier.weight(1f))
        trailing()
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
