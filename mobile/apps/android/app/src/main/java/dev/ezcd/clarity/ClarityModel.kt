package dev.ezcd.clarity

import dev.ezcd.clarity.bridge.ClarityBridge
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
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
 */
class ClarityModel(
    private val bridge: ClarityBridge,
    private val scope: CoroutineScope,
    private val io: CoroutineDispatcher,
) {
    private val _state = MutableStateFlow(AppState())
    val state: StateFlow<AppState> = _state.asStateFlow()

    /** How many commits a repository screen asks for. */
    private val commitLimit = 200

    /** Zero lets the core pick its own fetch depth and timeout. */
    private val fetchDepth = 0
    private val fetchTimeoutSeconds = 0

    fun start() = act { loadRepos() }

    fun showAddRepo() = _state.update { it.copy(screen = Screen.AddRepo, error = null) }

    fun showKey() {
        _state.update { it.copy(screen = Screen.Key, error = null) }
        act {
            // Asking for the key is what creates it. There is no point generating
            // one on first launch for a user who never adds an ssh remote.
            val key = bridge.publicKey("clarity on android")
            _state.update { it.copy(publicKey = key) }
        }
    }

    /** Back to the menu, dropping whatever the last screen was showing. */
    fun back() = _state.update { it.copy(screen = Screen.Repos, view = null, error = null) }

    fun addRepo(url: String, branch: String) = act {
        bridge.addRepo(url, branch)
        loadRepos()
        _state.update { it.copy(screen = Screen.Repos) }
    }

    fun removeRepo(repoId: String) = act {
        bridge.removeRepo(repoId)
        loadRepos()
        _state.update { if (it.screen == Screen.Repo(repoId)) it.copy(screen = Screen.Repos, view = null) else it }
    }

    /**
     * Opens a repository: what is already on disk first, then a fetch.
     *
     * The order is the point. A phone is offline often, and the local object
     * store is what makes that survivable — showing the last fetch immediately
     * beats a spinner over nothing.
     */
    fun openRepo(repoId: String) {
        _state.update { it.copy(screen = Screen.Repo(repoId), view = null, error = null) }
        scope.launch {
            // A repository that has never been fetched has no branch to resolve,
            // so this throws. That is the expected state on a first open, not
            // something to report: the fetch about to run is the answer to it,
            // and if the fetch fails too, its reason is the useful one.
            readView(repoId)?.let { v -> _state.update { it.copy(view = v) } }
            fetch(repoId)
        }
    }

    /** Re-fetches the open repository. */
    fun refresh() {
        val repoId = (state.value.screen as? Screen.Repo)?.id ?: return
        scope.launch { fetch(repoId) }
    }

    fun dismissError() = _state.update { it.copy(error = null) }

    // --- internals -----------------------------------------------------------

    private suspend fun fetch(repoId: String) {
        _state.update { it.copy(syncing = true, error = null) }
        try {
            withContext(io) { bridge.sync(repoId, fetchDepth, fetchTimeoutSeconds) }
        } catch (e: Exception) {
            // The view already in state stays there. A failed refresh must not
            // cost you the data you had.
            _state.update { it.copy(syncing = false, error = message(e)) }
            return
        }
        val fresh = readView(repoId)
        _state.update { it.copy(syncing = false, view = fresh ?: it.view) }
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
