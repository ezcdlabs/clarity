package dev.ezcd.clarity

import dev.ezcd.clarity.proto.RepoSummary
import dev.ezcd.clarity.proto.View

/** Which screen is in front of the user. */
sealed interface Screen {
    /** The menu of repositories that have been added. */
    data object Repos : Screen

    /** Paste a clone URL. */
    data object AddRepo : Screen

    /** The device's public key, for pasting into a host. */
    data object Key : Screen

    /** One repository's commits. */
    data class Repo(val id: String) : Screen
}

/**
 * The whole of what the UI draws, in one value.
 *
 * [view] is deliberately kept across a failed refresh: a phone loses its
 * connection constantly, and yesterday's pipeline is more use than an empty
 * screen with an error on it.
 */
data class AppState(
    val screen: Screen = Screen.Repos,
    val repos: List<RepoSummary> = emptyList(),
    val view: View? = null,
    /** True while a network fetch is in flight. */
    val syncing: Boolean = false,
    /** True while a local action (add, remove, key generation) is in flight. */
    val busy: Boolean = false,
    val publicKey: String? = null,
    val error: String? = null,
) {
    val repo: RepoSummary?
        get() = (screen as? Screen.Repo)?.let { s -> repos.firstOrNull { it.id == s.id } }
}
