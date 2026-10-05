package dev.ezcd.clarity

import android.Manifest
import android.app.Application
import android.content.Context
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.runtime.LaunchedEffect
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
import dev.ezcd.clarity.watch.CheckWorker
import dev.ezcd.clarity.watch.Notifications
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
        // The launcher started us in the splash theme; this is where it stops.
        // Before super.onCreate, because that is when the window is created and
        // a theme set after it has no window left to apply to.
        setTheme(R.style.Theme_Clarity)
        super.onCreate(savedInstanceState)
        // Default arguments, deliberately: they pick the system bar icon colours
        // from the system's own dark-mode setting, which is now the same thing
        // the palette follows. Forcing a style here is what made the tray
        // unreadable — dark icons drawn over a window we had painted dark.
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

                // Asked for when it first means something — once there is a
                // repository to be told about — rather than at launch. A
                // permission dialog in front of an empty screen is a question
                // about nothing, and the answer to those is usually no.
                val ask = rememberLauncherForActivityResult(
                    ActivityResultContracts.RequestPermission(),
                ) { if (it) CheckWorker.schedule(applicationContext) }

                LaunchedEffect(state.repos.isNotEmpty()) {
                    if (!state.repos.isNotEmpty()) {
                        // Nothing to watch. A check that wakes the phone to
                        // fetch no repositories is pure cost.
                        CheckWorker.cancel(applicationContext)
                        return@LaunchedEffect
                    }
                    Notifications.ensureChannels(applicationContext)
                    when {
                        Notifications.permitted(applicationContext) ->
                            CheckWorker.schedule(applicationContext)
                        // Once, ever. Android stops showing the dialog after two
                        // refusals anyway, but asking again on every cold start
                        // is nagging whether or not the system draws anything.
                        !askedAlready() -> {
                            rememberAsked()
                            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                                ask.launch(Manifest.permission.POST_NOTIFICATIONS)
                            }
                        }
                    }
                }

                // System back closes an overlay; from the repository itself it
                // leaves the app, because the repository is the home screen.
                BackHandler(enabled = state.overlay != null) { viewModel.model.closeOverlay() }
                App(viewModel.model, Modifier.windowInsetsPadding(WindowInsets.systemBars))
            }
        }
    }

    /**
     * Whether the notification permission has been put to the user before.
     *
     * Persisted rather than held in memory: a flag that resets with the process
     * asks again on every cold start, which is the behaviour people are
     * complaining about when they say an app nags.
     */
    private fun askedAlready(): Boolean =
        prefs().getBoolean(ASKED_FOR_NOTIFICATIONS, false)

    private fun rememberAsked() {
        prefs().edit().putBoolean(ASKED_FOR_NOTIFICATIONS, true).apply()
    }

    private fun prefs() = getSharedPreferences("clarity", Context.MODE_PRIVATE)

    private companion object {
        const val ASKED_FOR_NOTIFICATIONS = "asked-for-notifications"
    }
}
