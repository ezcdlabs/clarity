package dev.ezcd.clarity.bridge

import dev.ezcd.clarity.proto.RepoList
import dev.ezcd.clarity.proto.SyncResult
import dev.ezcd.clarity.proto.View

/**
 * Everything the app can ask of the Go core.
 *
 * This exists so the UI never touches the generated bindings directly: the
 * bindings need a device and a 33 MB native library, and nothing that depends
 * on them can be tested on a JVM. Behind this interface sits either [GoBridge]
 * or a fake, and the screens cannot tell which.
 *
 * Every method blocks. The core does network and disk work synchronously on
 * purpose — the caller decides which thread that happens on, and on Android
 * that decision belongs to the model, not the bridge.
 */
interface ClarityBridge {
    /** Whether a device key exists yet, without creating one. */
    fun hasKey(): Boolean

    /** The authorized_keys line for this device, generating the key if needed. */
    fun publicKey(comment: String): String

    /** Tracks a repository, returning its id. The URL is the one you'd clone. */
    fun addRepo(url: String, branch: String): String

    /** Forgets a repository and reclaims its object store. */
    fun removeRepo(repoId: String)

    fun listRepos(): RepoList

    /**
     * Fetches a repository. Blocking, and the only method that uses network.
     *
     * Returns what the fetch did rather than throwing, because a fetch has
     * failures the UI must tell apart: an unknown host is a question with its
     * own dialog, a rejected key is a fixable setup step with its own screen,
     * and everything else is a message.
     */
    fun sync(repoId: String, depth: Int, timeoutSeconds: Int): SyncResult

    /** Records a host key the user agreed to, so a refused fetch can retry. */
    fun trustHost(host: String, fingerprint: String)

    /** Renames a repository on this device. A blank name clears the rename. */
    fun rename(repoId: String, name: String)

    /** Reads what the last [sync] fetched. Never touches the network. */
    fun view(repoId: String, limit: Int): View

    /**
     * Formats a duration the way every clarity UI formats one.
     *
     * Here rather than in Kotlin because a running timer is recomputed every
     * second on the device, and the alternative to asking Go was reimplementing
     * the rule — one more place for "3m 29s" to drift into "3:29".
     */
    fun elapsed(seconds: Long): String
}
