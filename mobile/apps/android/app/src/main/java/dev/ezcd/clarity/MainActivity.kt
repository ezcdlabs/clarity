package dev.ezcd.clarity

import android.app.Application
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import dev.ezcd.clarity.bridge.GoBridge
import dev.ezcd.clarity.ui.App
import dev.ezcd.clarity.ui.ClarityTheme
import kotlinx.coroutines.Dispatchers
import java.io.File

/**
 * Owns the bridge and the model for the process.
 *
 * One client per app, not one per screen: the key, the registry and every
 * object store live under a single directory, and two clients over the same
 * directory would be two writers.
 */
class ClarityViewModel(app: Application) : AndroidViewModel(app) {
    val model: ClarityModel = run {
        val dir = File(app.filesDir, "clarity").apply { mkdirs() }
        ClarityModel(GoBridge.open(dir), viewModelScope, Dispatchers.IO)
    }

    init {
        model.start()
    }
}

class MainActivity : ComponentActivity() {
    private val viewModel: ClarityViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            ClarityTheme {
                val state by viewModel.model.state.collectAsStateWithLifecycle()

                // The clock and the automatic refresh follow the window, not
                // the process: a backgrounded app that keeps fetching is
                // spending someone's battery and data on a screen nobody is
                // looking at.
                LifecycleEventEffect(Lifecycle.Event.ON_START) { viewModel.model.resume() }
                LifecycleEventEffect(Lifecycle.Event.ON_STOP) { viewModel.model.pause() }

                // System back closes an overlay; from the repository itself it
                // leaves the app, because the repository is the home screen.
                BackHandler(enabled = state.overlay != null) { viewModel.model.closeOverlay() }
                App(viewModel.model, Modifier.windowInsetsPadding(WindowInsets.systemBars))
            }
        }
    }
}
