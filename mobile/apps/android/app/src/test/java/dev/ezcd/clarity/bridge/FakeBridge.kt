package dev.ezcd.clarity.bridge

import dev.ezcd.clarity.proto.Commit
import dev.ezcd.clarity.proto.Flow
import dev.ezcd.clarity.proto.Section
import dev.ezcd.clarity.proto.SectionKind
import dev.ezcd.clarity.proto.RepoList
import dev.ezcd.clarity.proto.RepoSummary
import dev.ezcd.clarity.proto.View

/**
 * A bridge with no Go behind it.
 *
 * Every failure the real core can hand back is reachable by setting a field, so
 * the model's behaviour on a rejected URL or a dead connection is a unit test
 * rather than something you find out about on a train.
 */
class FakeBridge : ClarityBridge {

    /** Method names in call order, for asserting what ran and what did not. */
    val calls = mutableListOf<String>()

    /** The repositories "on disk". */
    val repos = mutableListOf<RepoSummary>()

    /** Views keyed by repo id. A missing entry behaves like a never-synced repo. */
    val views = mutableMapOf<String, View>()

    var keyExists = false
    var key = "ssh-ed25519 AAAAC3Nz fake"

    /** Set to make the matching call throw with this message. */
    var failAddRepo: String? = null
    var failSync: String? = null
    var failList: String? = null
    var failKey: String? = null

    /**
     * Runs inside [sync], before it succeeds or fails. A test uses it to observe
     * the state a fetch starts from, or to make a fetch deliver new commits.
     */
    var onSync: (String) -> Unit = {}

    override fun hasKey(): Boolean {
        calls += "hasKey"
        return keyExists
    }

    override fun publicKey(comment: String): String {
        calls += "publicKey"
        failKey?.let { throw RuntimeException(it) }
        keyExists = true
        return "$key $comment"
    }

    override fun addRepo(url: String, branch: String): String {
        calls += "addRepo"
        failAddRepo?.let { throw RuntimeException(it) }
        val id = "id-${repos.size + 1}"
        repos += RepoSummary.newBuilder()
            .setId(id)
            .setName(url.substringAfterLast('/').removeSuffix(".git"))
            .setUrl(url)
            .setBranch(branch.ifBlank { "main" })
            .build()
        return id
    }

    override fun removeRepo(repoId: String) {
        calls += "removeRepo"
        repos.removeAll { it.id == repoId }
        views -= repoId
    }

    override fun listRepos(): RepoList {
        calls += "listRepos"
        failList?.let { throw RuntimeException(it) }
        return RepoList.newBuilder().addAllRepos(repos).build()
    }

    override fun sync(repoId: String, depth: Int, timeoutSeconds: Int) {
        calls += "sync"
        onSync(repoId)
        failSync?.let { throw RuntimeException(it) }
    }

    // Not the real formatter — the model only passes through to it, so a test
    // that asserted the wording would be testing Go through two layers.
    override fun elapsed(seconds: Long): String = seconds.toString() + "s"

    override fun view(repoId: String, limit: Int): View {
        calls += "view"
        return views[repoId]
            // What a repo that has never been fetched gives you: go-git has no
            // branch to resolve, so the read fails rather than returning empty.
            ?: throw RuntimeException("reference not found")
    }

    companion object {
        /** A one-commit view, enough to tell "we have data" from "we do not". */
        fun viewOf(subject: String): View = View.newBuilder()
            .addFlows(
                Flow.newBuilder().setName("deploy").addSections(
                    Section.newBuilder()
                        .setKind(SectionKind.SECTION_KIND_HEAD)
                        .setLabel("HEAD")
                        .addCommits(
                            Commit.newBuilder().setSubject(subject).setSha("abcdef12"),
                        ),
                ),
            )
            .build()
    }
}
