package dev.ezcd.clarity.bridge

import dev.ezcd.clarity.proto.Commit
import dev.ezcd.clarity.proto.Flow
import dev.ezcd.clarity.proto.HostKey
import dev.ezcd.clarity.proto.AxisTick
import dev.ezcd.clarity.proto.Change
import dev.ezcd.clarity.proto.Changes
import dev.ezcd.clarity.proto.LeadAxis
import dev.ezcd.clarity.proto.Metrics
import dev.ezcd.clarity.proto.MetricsFlow
import dev.ezcd.clarity.proto.Outcome
import dev.ezcd.clarity.proto.Plot
import dev.ezcd.clarity.proto.Week
import dev.ezcd.clarity.proto.RepoList
import dev.ezcd.clarity.proto.RepoSummary
import dev.ezcd.clarity.proto.Section
import dev.ezcd.clarity.proto.SectionKind
import dev.ezcd.clarity.proto.Status
import dev.ezcd.clarity.proto.SyncResult
import dev.ezcd.clarity.proto.View
import dev.ezcd.clarity.proto.ViewResult

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

    /** What a fetch comes back with, for the flows that branch on it. */
    var syncOutcome: Outcome = Outcome.OUTCOME_OK
    var gitOutput: String = ""
    var failList: String? = null
    var failKey: String? = null
    var failMetrics: String? = null

    /** Weekly aggregates keyed by repo id, as [metrics] would read them. */
    val weekly = mutableMapOf<String, Metrics>()

    /** The arguments the last [metrics] call was made with. */
    var metricsWindow: Pair<Int, Int>? = null

    /** The depths [sync] has been asked for, in call order. */
    val syncDepths = mutableListOf<Int>()

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
        // Ordered the way the registry orders it: by the name a row shows, then
        // by id. Returning insertion order instead would have been a fake that
        // agreed with the model about something the real core disagrees with —
        // and "the one just added" would look like "the last one" in tests and
        // nowhere else.
        return RepoList.newBuilder()
            .addAllRepos(repos.sortedWith(compareBy({ it.name }, { it.id })))
            .build()
    }

    override fun sync(repoId: String, depth: Int, timeoutSeconds: Int): SyncResult {
        calls += "sync"
        syncDepths += depth
        onSync(repoId)
        val builder = SyncResult.newBuilder()
        failSync?.let {
            return builder.setOutcome(Outcome.OUTCOME_FAILED).setMessage(it).build()
        }
        if (syncOutcome == Outcome.OUTCOME_HOST_KEY_UNKNOWN ||
            syncOutcome == Outcome.OUTCOME_HOST_KEY_CHANGED
        ) {
            builder.hostKey = HostKey.newBuilder()
                .setHost("git.acme.dev")
                .setType("ED25519")
                .setFingerprint("SHA256:fake-fingerprint")
                .build()
        }
        return builder.setOutcome(syncOutcome).setGitOutput(gitOutput).build()
    }

    override fun trustHost(host: String, fingerprint: String) {
        calls += "trustHost"
    }

    override fun setBranch(repoId: String, branch: String) {
        calls += "setBranch"
        val i = repos.indexOfFirst { it.id == repoId }
        if (i >= 0) repos[i] = repos[i].toBuilder().setBranch(branch.ifBlank { "main" }).build()
    }

    override fun rename(repoId: String, name: String) {
        calls += "rename"
        val i = repos.indexOfFirst { it.id == repoId }
        if (i >= 0) repos[i] = repos[i].toBuilder().setAlias(name).build()
    }

    override fun metrics(repoId: String, commitLimit: Int, weeks: Int): Metrics {
        calls += "metrics"
        metricsWindow = commitLimit to weeks
        failMetrics?.let { throw RuntimeException(it) }
        return weekly[repoId]
            // What a repo that has never been fetched gives you, as [view] does.
            ?: throw RuntimeException("reference not found")
    }

    /** What the next [read] reports as having moved. Cleared once delivered. */
    var crossings: List<Change> = emptyList()

    override fun read(repoId: String, limit: Int): ViewResult {
        val result = ViewResult.newBuilder()
            .setView(view(repoId, limit))
            .addAllChanges(crossings)
            .build()
        // Consumed, as the core consumes it: a crossing is reported once.
        crossings = emptyList()
        return result
    }

    /** What the next [check] reports. */
    var changes: Changes = Changes.newBuilder().setGeneratedUnixSeconds(1).build()

    override fun check(timeoutSeconds: Int): Changes {
        calls += "check"
        return changes
    }

    // Not the real formatter — the model only passes through to it, so a test
    // that asserted the wording would be testing Go through two layers.
    override fun elapsed(seconds: Long): String = seconds.toString() + "s"

    override fun view(repoId: String, limit: Int): View {
        calls += "view"
        // Reading a view is what records its verdict, as the core does — so a
        // test can check that the list picks the verdict up.
        val i = repos.indexOfFirst { it.id == repoId }
        if (i >= 0 && views.containsKey(repoId)) {
            repos[i] = repos[i].toBuilder().setCi(Status.STATUS_PASSED).build()
        }
        return views[repoId]
            // What a repo that has never been fetched gives you: go-git has no
            // branch to resolve, so the read fails rather than returning empty.
            ?: throw RuntimeException("reference not found")
    }

    companion object {
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

        /**
         * One flow of weekly aggregates, newest week first.
         *
         * Deploy counts are the input because they are what a test asserts on;
         * the distribution is filled in consistently so a chart has something
         * to draw without every test having to describe a box plot.
         */
        fun metricsOf(vararg deploys: Int): Metrics {
            val flow = MetricsFlow.newBuilder()
                .setName("deploy")
                .setMaxDeploys(deploys.maxOrNull() ?: 0)
                .setAxis(
                    LeadAxis.newBuilder()
                        .setMaxSeconds(8 * 3600)
                        .addTicks(AxisTick.newBuilder().setFraction(0.0).setLabel("0"))
                        .addTicks(AxisTick.newBuilder().setFraction(0.25).setLabel("2h"))
                        .addTicks(AxisTick.newBuilder().setFraction(0.5).setLabel("4h"))
                        .addTicks(AxisTick.newBuilder().setFraction(0.75).setLabel("6h"))
                        .addTicks(AxisTick.newBuilder().setFraction(1.0).setLabel("8h")),
                )
            deploys.forEachIndexed { i, n ->
                val week = Week.newBuilder()
                    .setLabel("W2026-%02d".format(40 - i))
                    .setDeploys(n)
                if (n == 0) {
                    week.plot = Plot.PLOT_NONE
                } else {
                    week.plot = Plot.PLOT_BOX
                    week.n = n
                    week.minSeconds = 3600
                    week.p25Seconds = 2 * 3600
                    week.p50Seconds = 3 * 3600
                    week.p75Seconds = 4 * 3600
                    week.maxSeconds = 5 * 3600
                }
                flow.addWeeks(week)
            }
            return Metrics.newBuilder()
                .addFlows(flow)
                .setGeneratedUnixSeconds(1_700_000_000)
                .build()
        }
    }
}
