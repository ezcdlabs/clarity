package dev.ezcd.clarity.bridge

import dev.ezcd.clarity.core.Client
import dev.ezcd.clarity.core.Core
import dev.ezcd.clarity.proto.Changes
import dev.ezcd.clarity.proto.Metrics
import dev.ezcd.clarity.proto.RepoList
import dev.ezcd.clarity.proto.SyncResult
import dev.ezcd.clarity.proto.View
import java.io.File

/**
 * The real bridge: gomobile's generated [Client], with protobuf bytes decoded
 * on the way back.
 *
 * There is no logic here beyond that decoding. Anything that needed a decision
 * would be untestable at this layer, so it lives in Go (where mobile/internal
 * tests cover it) or in the model (where a fake covers it).
 */
class GoBridge(private val client: Client) : ClarityBridge {

    override fun hasKey(): Boolean = client.hasKey()

    override fun publicKey(comment: String): String = client.publicKey(comment)

    override fun addRepo(url: String, branch: String): String = client.addRepo(url, branch)

    override fun removeRepo(repoId: String) {
        val store = File(client.storePath(repoId))
        client.removeRepo(repoId)
        // The core deliberately leaves the objects behind rather than deleting
        // user data as a side effect of forgetting a name. Someone has to, and
        // the app is the side that knows the repo is really gone.
        store.deleteRecursively()
    }

    override fun listRepos(): RepoList = RepoList.parseFrom(client.listRepos())

    override fun sync(repoId: String, depth: Int, timeoutSeconds: Int): SyncResult =
        SyncResult.parseFrom(client.sync(repoId, depth.toLong(), timeoutSeconds.toLong()))

    override fun trustHost(host: String, fingerprint: String) = client.trustHost(host, fingerprint)

    override fun rename(repoId: String, name: String) = client.rename(repoId, name)

    override fun setBranch(repoId: String, branch: String) = client.setBranch(repoId, branch)

    override fun view(repoId: String, limit: Int): View =
        View.parseFrom(client.view(repoId, limit.toLong()))

    override fun metrics(repoId: String, commitLimit: Int, weeks: Int): Metrics =
        Metrics.parseFrom(client.metrics(repoId, commitLimit.toLong(), weeks.toLong()))

    override fun check(timeoutSeconds: Int): Changes =
        Changes.parseFrom(client.check(timeoutSeconds.toLong()))

    override fun elapsed(seconds: Long): String = Core.elapsed(seconds)

    companion object {
        private val instances = mutableMapOf<String, GoBridge>()

        /**
         * The bridge over the directory the platform gives us for private data.
         *
         * One per directory for the whole process, and that is not an
         * optimisation. The key, the registry and every object store live under
         * that one directory, and a second client over it is a second writer:
         * the Go side serialises its own work per repository, and two clients
         * have two sets of locks that know nothing about each other.
         *
         * It became load-bearing when the background check arrived. That runs
         * in the same process as the UI whenever the app happens to be alive,
         * and it was opening a client of its own — which is exactly the case
         * the locking was added to prevent.
         */
        @Synchronized
        fun open(dataDir: File): GoBridge {
            val path = dataDir.absolutePath
            return instances.getOrPut(path) { GoBridge(Core.new_(path)) }
        }
    }
}
