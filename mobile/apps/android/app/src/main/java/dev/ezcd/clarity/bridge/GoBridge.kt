package dev.ezcd.clarity.bridge

import dev.ezcd.clarity.core.Client
import dev.ezcd.clarity.core.Core
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

    override fun keyFingerprint(): String = client.keyFingerprint()

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

    override fun view(repoId: String, limit: Int): View =
        View.parseFrom(client.view(repoId, limit.toLong()))

    override fun elapsed(seconds: Long): String = Core.elapsed(seconds)

    companion object {
        /** Opens a bridge over the directory the platform gives us for private data. */
        fun open(dataDir: File): GoBridge = GoBridge(Core.new_(dataDir.absolutePath))
    }
}
