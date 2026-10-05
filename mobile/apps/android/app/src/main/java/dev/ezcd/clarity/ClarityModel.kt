package dev.ezcd.clarity

import dev.ezcd.clarity.bridge.ClarityBridge
import dev.ezcd.clarity.proto.Outcome
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Job
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * The app's state machine.
 *
 * Holds no Android types and no bindings, so the whole of it runs on a JVM
 * against [dev.ezcd.clarity.bridge.FakeBridge]. The UI reads [state] and calls
 * these methods; it makes no decisions of its own.
 *
 * [io] is where every bridge call goes. The bridge blocks — a fetch is a real
 * network round trip — and nothing here may block the caller's thread.
 *
 * [clock] supplies epoch seconds. Injected rather than read, for the same
 * reason every renderer in this project takes a `now`: a test that cannot move
 * the clock cannot test anything that ticks.
 */
class ClarityModel(
    private val bridge: ClarityBridge,
    private val scope: CoroutineScope,
    private val io: CoroutineDispatcher,
    private val clock: () -> Long = { System.currentTimeMillis() / 1000 },
) {
    private val _state = MutableStateFlow(AppState())
    val state: StateFlow<AppState> = _state.asStateFlow()

    /** How many commits a repository screen asks for. */
    private val commitLimit = 200

    /** Zero lets the core pick its own fetch depth and timeout. */
    private val fetchDepth = 0
    private val fetchTimeoutSeconds = 0

    /** How often the open repository re-fetches, like the TUI's watcher. */
    private val refreshSeconds = 30

    /** The clock and the auto-refresh, running only while the UI is visible. */
    private var pump: Job? = null

    fun start() = act {
        loadRepos()
        // Land in a repository rather than on a menu. There is nothing to read
        // on the menu, and the one thing the app exists to show is one commit
        // list — the same reason a chat app opens in a channel.
        val first = _state.value.repos.firstOrNull()?.id
        if (_state.value.selected == null && first != null) {
            open(first)
        }
    }

    /**
     * Starts the clock and the automatic refresh. Call when the UI becomes
     * visible, and [pause] when it stops being.
     */
    fun resume() {
        if (pump != null) return
        pump = scope.launch {
            var sinceFetch = 0
            while (true) {
                _state.update { it.copy(nowSeconds = clock()) }
                delay(1_000)
                sinceFetch++
                if (sinceFetch >= refreshSeconds) {
                    sinceFetch = 0
                    _state.value.selected?.let { fetch(it, background = true) }
                }
            }
        }
    }

    /**
     * Stops both. Nothing ticks and nothing fetches while the app is in the
     * background: a timer nobody can see is only spending battery, and a fetch
     * nobody asked for is spending their data too.
     */
    fun pause() {
        pump?.cancel()
        pump = null
    }

    /** Opens the connect flow, from the empty state or from the switcher. */
    fun showConnect() = _state.update {
        it.copy(overlay = Overlay.Connect, connect = Connect.Idle, error = null)
    }

    fun showKey() {
        _state.update { it.copy(overlay = Overlay.Key, error = null) }
        loadKey()
    }

    /**
     * Fetches the device key, generating it on first call.
     *
     * Asking for it is what creates it, so this is not done at launch: a user
     * who never adds an ssh remote never needs one.
     */
    fun loadKey() = act {
        val key = bridge.publicKey("clarity on android")
        val fingerprint = bridge.keyFingerprint()
        _state.update { it.copy(publicKey = key, fingerprint = fingerprint) }
    }

    /** Closes whatever is over the repository, leaving the selection alone. */
    fun closeOverlay() = _state.update { it.copy(overlay = null, connect = Connect.Idle, error = null) }

    /**
     * Connects a repository: track it, then fetch it once.
     *
     * The two are one action from the user's side — a repository that was added
     * but never reached is not connected — so a fetch that comes back asking
     * about a host key or refusing the device key leaves the flow open on the
     * screen that explains it, rather than dropping them into an empty feed.
     */
    fun connect(url: String, branch: String) {
        scope.launch {
            _state.update { it.copy(connect = Connect.Working, error = null) }
            val id = try {
                withContext(io) { bridge.addRepo(url, branch) }
            } catch (e: Exception) {
                _state.update { it.copy(connect = Connect.Failed(message(e))) }
                return@launch
            }
            act { loadRepos() }
            attempt(id)
        }
    }

    /** Records the host key the prompt showed, then picks up where it stopped. */
    fun trustHost() {
        val asking = _state.value.connect as? Connect.AskHost ?: return
        val id = _state.value.repos.lastOrNull()?.id ?: return
        scope.launch {
            _state.update { it.copy(connect = Connect.Working) }
            try {
                withContext(io) { bridge.trustHost(asking.key.host, asking.key.fingerprint) }
            } catch (e: Exception) {
                _state.update { it.copy(connect = Connect.Failed(message(e))) }
                return@launch
            }
            attempt(id)
        }
    }

    /** Tries the fetch again after the user has fixed something on the host. */
    fun retryConnect() {
        val id = _state.value.repos.lastOrNull()?.id ?: return
        scope.launch {
            _state.update { it.copy(connect = Connect.Working) }
            attempt(id)
        }
    }

    /** Abandons the flow. The repository stays tracked if it was added. */
    fun cancelConnect() = _state.update { it.copy(overlay = null, connect = Connect.Idle) }

    /**
     * One fetch of a repository being connected, sorted into the screen it
     * should leave behind.
     */
    private suspend fun attempt(repoId: String) {
        val result = try {
            withContext(io) { bridge.sync(repoId, fetchDepth, fetchTimeoutSeconds) }
        } catch (e: Exception) {
            _state.update { it.copy(connect = Connect.Failed(message(e))) }
            return
        }
        when (result.outcome) {
            Outcome.OUTCOME_OK -> {
                // The key has now been accepted by a host at least once, which
                // is what folds the key card away on the next connect.
                _state.update { it.copy(connect = Connect.Idle, overlay = null, keyHasConnected = true) }
                open(repoId)
            }
            Outcome.OUTCOME_HOST_KEY_UNKNOWN ->
                _state.update { it.copy(connect = Connect.AskHost(result.hostKey, changed = false)) }
            Outcome.OUTCOME_HOST_KEY_CHANGED ->
                _state.update { it.copy(connect = Connect.AskHost(result.hostKey, changed = true)) }
            Outcome.OUTCOME_AUTH_DENIED ->
                _state.update { it.copy(connect = Connect.Denied(result.gitOutput)) }
            else ->
                _state.update { it.copy(connect = Connect.Failed(result.message)) }
        }
    }

    /** Renames a repository on this device. A blank name clears the rename. */
    fun rename(repoId: String, name: String) = act {
        bridge.rename(repoId, name)
        loadRepos()
    }

    fun removeRepo(repoId: String) = act {
        bridge.removeRepo(repoId)
        loadRepos()
        if (_state.value.selected == repoId) {
            // Falling back to whatever is left beats leaving the main surface
            // showing a repository that is no longer in the list.
            val next = _state.value.repos.firstOrNull()?.id
            _state.update { it.copy(selected = null, view = null) }
            if (next != null) open(next)
        }
    }

    /**
     * Shows a repository: what is already on disk first, then a fetch.
     *
     * The order is the point. A phone is offline often, and the local object
     * store is what makes that survivable — showing the last fetch immediately
     * beats a spinner over nothing.
     */
    fun select(repoId: String) {
        if (_state.value.selected == repoId) return
        scope.launch { open(repoId) }
    }

    /** Re-fetches the open repository. */
    fun refresh() {
        val repoId = _state.value.selected ?: return
        scope.launch { fetch(repoId, background = false) }
    }

    fun dismissError() = _state.update { it.copy(error = null) }

    /**
     * Formats a duration, for the timers the UI ticks between fetches.
     *
     * The view arrives with every duration preformatted, which is right for the
     * instant it was built and wrong one second later. This is how a row stays
     * honest without asking Go to walk the commit graph again.
     */
    fun elapsed(seconds: Long): String = bridge.elapsed(if (seconds < 0) 0 else seconds)

    // --- internals -----------------------------------------------------------

    private suspend fun open(repoId: String) {
        // The outgoing repository's commits go immediately. Leaving them up
        // while the next one loads shows one repository's work under another's
        // name, which is worse than showing nothing.
        _state.update { it.copy(selected = repoId, view = null, error = null) }

        // A repository that has never been fetched has no branch to resolve,
        // so this throws. That is the expected state on a first open, not
        // something to report: the fetch about to run is the answer to it,
        // and if the fetch fails too, its reason is the useful one.
        readView(repoId)?.let { v -> _state.update { it.copy(view = v) } }
        fetch(repoId, background = false)
    }

    /**
     * Fetches, then re-reads.
     *
     * A background fetch that fails says nothing when there is already a view
     * on screen. The alternative is an error bar that reappears every thirty
     * seconds for as long as you are on a train, over data that is perfectly
     * readable — the failure is worth reporting when you asked for it, or when
     * there is nothing behind it to read.
     */
    private suspend fun fetch(repoId: String, background: Boolean) {
        _state.update { it.copy(syncing = true, error = if (background) it.error else null) }
        try {
            val result = withContext(io) { bridge.sync(repoId, fetchDepth, fetchTimeoutSeconds) }
            if (result.outcome != Outcome.OUTCOME_OK) {
                // A refresh is not the place to ask about a host key: this
                // repository has connected before, so anything unexpected now
                // is news rather than a setup step.
                throw RuntimeException(result.message)
            }
        } catch (e: Exception) {
            _state.update {
                val quiet = background && it.view != null
                it.copy(syncing = false, error = if (quiet) it.error else message(e))
            }
            return
        }
        val fresh = readView(repoId)
        _state.update {
            // A fetch that finished after the user moved on belongs to a
            // repository that is no longer on screen.
            if (it.selected != repoId) it.copy(syncing = false)
            else it.copy(syncing = false, view = fresh ?: it.view)
        }
    }

    /** The view on disk, or null if there is not one yet. */
    private suspend fun readView(repoId: String) = try {
        withContext(io) { bridge.view(repoId, commitLimit) }
    } catch (e: Exception) {
        null
    }

    private fun loadRepos() {
        val list = bridge.listRepos()
        _state.update { it.copy(repos = list.reposList) }
    }

    /**
     * Runs a local action off the UI thread, reporting whatever it throws.
     *
     * Everything the core raises is already written for a person to read — the
     * registry's "paste the URL you would clone", git's own fetch diagnosis — so
     * the message is passed through rather than replaced with a category.
     */
    private fun act(action: suspend () -> Unit) {
        scope.launch {
            _state.update { it.copy(busy = true, error = null) }
            try {
                withContext(io) { action() }
                _state.update { it.copy(busy = false) }
            } catch (e: Exception) {
                _state.update { it.copy(busy = false, error = message(e)) }
            }
        }
    }

    private fun message(e: Exception) = e.message?.takeIf { it.isNotBlank() } ?: e.toString()
}
