package dev.ezcd.clarity

import dev.ezcd.clarity.proto.HostKey
import dev.ezcd.clarity.proto.Metrics
import dev.ezcd.clarity.proto.RepoSummary
import dev.ezcd.clarity.proto.View

/** A screen that sits over the repository, rather than beside it. */
enum class Overlay {
    /** Paste a clone address and connect. One full-screen flow. */
    Connect,

    /** This device's public key, on its own. */
    Key,

    /** The weekly aggregates, with a back arrow. */
    Metrics,
}

/**
 * Where the connect flow has got to.
 *
 * Modelled as states rather than as a pair of booleans because they are
 * genuinely exclusive and each one draws a different screen — and because
 * two of them carry something the screen cannot work out for itself: a
 * fingerprint to show, or the raw thing git said.
 */
sealed interface Connect {
    data object Idle : Connect

    data object Working : Connect

    /** The host is not one this device has agreed to. Ask, then trust. */
    data class AskHost(val key: HostKey, val changed: Boolean) : Connect

    /** The host refused the device key. Fixable, with its own screen. */
    data class Denied(val gitOutput: String) : Connect

    data class Failed(val message: String) : Connect
}

/**
 * The whole of what the UI draws, in one value.
 *
 * [selected] outlives everything that opens over it. The repository is the app;
 * the switcher, the connect flow and the key screen are things that happen in
 * front of it, and closing one puts you back where you were.
 *
 * [view] is deliberately kept across a failed refresh: a phone loses its
 * connection constantly, and yesterday's pipeline is more use than an empty
 * screen with an error on it.
 */
data class AppState(
    val repos: List<RepoSummary> = emptyList(),
    val selected: String? = null,
    val view: View? = null,
    /**
     * The weekly aggregates, read when the metrics screen opens and dropped
     * when it closes.
     *
     * Unlike [view] this is not held across anything. [view] survives a failed
     * refresh because yesterday's pipeline still answers "is main green?"
     * usefully; a chart of history held over from before a fetch would be
     * answering "are we getting better?" with an older answer than the one on
     * disk, and nothing on it would say so.
     */
    val metrics: Metrics? = null,
    /** Which flow the metrics screen is showing, carried in from the feed. */
    val metricsFlow: Int = 0,
    val overlay: Overlay? = null,
    val connect: Connect = Connect.Idle,
    /** True while a network fetch is in flight. */
    val syncing: Boolean = false,
    /** True while a local action (add, remove, key generation) is in flight. */
    val busy: Boolean = false,
    val publicKey: String? = null,
    /**
     * Whether any repository has ever connected with this device's key. It
     * decides whether the connect screen opens with the key on show or folded
     * away — the first time you need it, and after that you do not.
     */
    val keyHasConnected: Boolean = false,
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

    /** True when there is nothing to show yet, which is its own screen. */
    val empty: Boolean
        get() = repos.isEmpty()
}

/** The title a repository is shown under, and the namespace in front of it. */
val RepoSummary.title: String
    get() = alias.ifEmpty { name }

/**
 * The dimmed prefix before the title, with its trailing slash.
 *
 * A renamed repository has none: the alias replaces the whole label and the
 * derived namespace/name moves to the subtitle, so showing a namespace in
 * front of a name the user chose would attach it to the wrong thing.
 */
val RepoSummary.titlePrefix: String
    get() = if (alias.isEmpty() && namespace.isNotEmpty()) "$namespace/" else ""

/** The line under the title: what it is, where it came from. */
val RepoSummary.subtitle: String
    get() = buildString {
        if (alias.isNotEmpty()) {
            if (namespace.isNotEmpty()) append("$namespace/")
            append(name)
            append(" · ")
        }
        append(branch)
        if (host.isNotEmpty()) append(" · ").append(host)
    }
