package dev.ezcd.clarity

import dev.ezcd.clarity.proto.RepoSummary
import dev.ezcd.clarity.proto.View

/** A screen that sits over the repository, rather than replacing it. */
enum class Overlay { AddRepo, Key }

/**
 * The whole of what the UI draws, in one value.
 *
 * [selected] outlives the drawer on purpose. Opening the repository list is not
 * leaving the repository — the same way it is not in Slack or Discord — so the
 * list is somewhere you pass through, and sliding it away puts you back where
 * you were rather than nowhere.
 *
 * [view] is deliberately kept across a failed refresh: a phone loses its
 * connection constantly, and yesterday's pipeline is more use than an empty
 * screen with an error on it.
 */
data class AppState(
    val repos: List<RepoSummary> = emptyList(),
    val selected: String? = null,
    val view: View? = null,
    val overlay: Overlay? = null,
    /** True while a network fetch is in flight. */
    val syncing: Boolean = false,
    /** True while a local action (add, remove, key generation) is in flight. */
    val busy: Boolean = false,
    val publicKey: String? = null,
    val error: String? = null,
    /**
     * Epoch seconds, advanced by the model while the UI is visible. Every timer
     * on screen is formatted against this, so they all tick together and none
     * of them tick while nobody is looking.
     */
    val nowSeconds: Long = 0,
) {
    val repo: RepoSummary?
        get() = repos.firstOrNull { it.id == selected }
}
