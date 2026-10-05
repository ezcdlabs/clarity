package dev.ezcd.clarity

import dev.ezcd.clarity.bridge.FakeBridge
import dev.ezcd.clarity.proto.Outcome
import dev.ezcd.clarity.proto.Status
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.Job
import kotlinx.coroutines.test.TestCoroutineScheduler
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class ClarityModelTest {

    @Test
    fun `launching lands in a repository rather than on a menu`() = test {
        bridge.addRepo("git@github.com:ezcdlabs/clarity.git", "main")
        bridge.addRepo("git@github.com:ezcdlabs/other.git", "main")
        bridge.calls.clear()

        val model = model()
        model.start()

        assertEquals(listOf("clarity", "other"), model.state.value.repos.map { it.name })
        // There is nothing to read on a menu, and one commit list is the thing
        // the app exists to show.
        assertEquals(model.state.value.repos.first().id, model.state.value.selected)
    }

    @Test
    fun `connecting a repository selects it and closes the flow`() = test {
        val model = model()
        model.start()
        model.showConnect()

        model.connect("git@github.com:ezcdlabs/clarity.git", "")

        assertNull(model.state.value.overlay)
        assertEquals(listOf("clarity"), model.state.value.repos.map { it.name })
        assertEquals("main", model.state.value.repos.single().branch)
        assertEquals(model.state.value.repos.single().id, model.state.value.selected)
        assertNull(model.state.value.error)
    }

    @Test
    fun `a rejected URL keeps the flow open with the reason`() = test {
        bridge.failAddRepo = "\"nonsense\" does not look like a git remote — paste the URL you would clone"

        val model = model()
        model.start()
        model.showConnect()
        model.connect("nonsense", "")

        // Closing it would throw away what was typed, and a paste of a clone
        // URL is not something anyone wants to redo. The reason belongs to the
        // flow rather than to the error bar, which is behind it.
        assertEquals(Overlay.Connect, model.state.value.overlay)
        val failed = model.state.value.connect as Connect.Failed
        assertTrue(failed.message.contains("does not look like a git remote"))
        assertTrue(model.state.value.repos.isEmpty())
    }

    @Test
    fun `opening a repository shows what is on disk before it fetches`() = test {
        val model = model()
        model.start()
        model.connect("git@github.com:ezcdlabs/clarity.git", "main")
        model.connect("git@github.com:ezcdlabs/other.git", "main")
        val first = model.state.value.repos.first().id
        bridge.views[first] = FakeBridge.viewOf("what we already had")

        var atFetch: String? = null
        bridge.onSync = { atFetch = subjectOf(model.state.value) }
        model.select(first)

        // The point of a local object store: offline, the last fetch is still
        // the dashboard. A spinner over an empty screen would waste it.
        assertEquals("what we already had", atFetch)
    }

    @Test
    fun `a failed refresh keeps the last view and says what happened`() = test {
        val model = model()
        model.start()
        model.connect("git@github.com:ezcdlabs/clarity.git", "main")
        val id = model.state.value.repos.single().id
        bridge.views[id] = FakeBridge.viewOf("yesterday")
        model.refresh()

        bridge.failSync = "dial tcp: network is unreachable"
        model.refresh()

        assertEquals("yesterday", subjectOf(model.state.value))
        assertTrue(model.state.value.error!!.contains("network is unreachable"))
        assertEquals(false, model.state.value.syncing)
    }

    @Test
    fun `a first open with nothing on disk reports only the fetch failure`() = test {
        bridge.failSync = "dial tcp: network is unreachable"

        val model = model()
        model.start()
        model.showConnect()
        model.connect("git@github.com:ezcdlabs/clarity.git", "main")

        // The flow stays open on the reason. Reading a never-fetched repo fails
        // too, but "reference not found" is not news on a first connection —
        // it is the expected state, and surfacing it would bury the reason the
        // fetch did not fix it.
        assertEquals(Overlay.Connect, model.state.value.overlay)
        assertEquals(
            "dial tcp: network is unreachable",
            (model.state.value.connect as Connect.Failed).message,
        )
        assertNull(model.state.value.view)
    }

    @Test
    fun `a fetch that brings new commits replaces the view`() = test {
        val model = model()
        model.start()
        model.connect("git@github.com:ezcdlabs/clarity.git", "main")
        val id = model.state.value.repos.single().id
        bridge.views[id] = FakeBridge.viewOf("yesterday")
        model.refresh()

        bridge.onSync = { bridge.views[it] = FakeBridge.viewOf("today") }
        model.refresh()

        assertEquals("today", subjectOf(model.state.value))
    }

    @Test
    fun `switching repositories drops the previous one's commits`() = test {
        val model = model()
        model.start()
        model.connect("git@github.com:ezcdlabs/clarity.git", "main")
        model.connect("git@github.com:ezcdlabs/other.git", "main")
        val (first, second) = model.state.value.repos.map { it.id }
        bridge.views[first] = FakeBridge.viewOf("clarity's work")
        model.select(first)
        assertEquals("clarity's work", subjectOf(model.state.value))

        model.select(second)

        // Showing one repository's commits under another's name is worse than
        // showing nothing at all.
        assertEquals(second, model.state.value.selected)
        assertNull(model.state.value.view)
    }

    @Test
    fun `the selection survives opening the repository list`() = test {
        val model = model()
        model.start()
        model.connect("git@github.com:ezcdlabs/clarity.git", "main")
        val id = model.state.value.repos.single().id
        bridge.views[id] = FakeBridge.viewOf("still here")
        model.select(id)
        model.refresh()

        // The list is a drawer, not a destination: there is no model call for
        // opening it, and nothing about the repository changes when it does.
        model.showConnect()
        model.closeOverlay()

        assertEquals(id, model.state.value.selected)
        assertEquals("still here", subjectOf(model.state.value))
    }

    @Test
    fun `removing the open repository falls back to another`() = test {
        val model = model()
        model.start()
        model.connect("git@github.com:ezcdlabs/clarity.git", "main")
        model.connect("git@github.com:ezcdlabs/other.git", "main")
        val open = model.state.value.selected!!

        model.removeRepo(open)

        assertEquals(listOf("clarity"), model.state.value.repos.map { it.name })
        assertNotEquals(open, model.state.value.selected)
        assertEquals(model.state.value.repos.single().id, model.state.value.selected)
    }

    @Test
    fun `the key screen generates a key on demand`() = test {
        val model = model()
        model.start()
        assertEquals(false, bridge.keyExists)

        model.showKey()

        assertEquals(Overlay.Key, model.state.value.overlay)
        assertTrue(model.state.value.publicKey!!.startsWith("ssh-ed25519 "))
        assertTrue(bridge.keyExists)
    }

    @Test
    fun `the next attempt that works clears the reason the last one failed`() = test {
        bridge.failAddRepo = "nope"
        val model = model()
        model.start()
        model.showConnect()
        model.connect("nonsense", "")
        assertEquals("nope", (model.state.value.connect as Connect.Failed).message)

        bridge.failAddRepo = null
        model.connect("git@github.com:ezcdlabs/clarity.git", "main")

        // Connected: the flow closes, and nothing is left saying it did not.
        assertEquals(Connect.Idle, model.state.value.connect)
        assertNull(model.state.value.overlay)
        assertNull(model.state.value.error)
    }

    @Test
    fun `an unknown host stops the flow and offers the fingerprint`() = test {
        bridge.syncOutcome = Outcome.OUTCOME_HOST_KEY_UNKNOWN

        val model = model()
        model.start()
        model.showConnect()
        model.connect("git@git.acme.dev:acme/thing.git", "main")

        // Nothing is trusted yet — the whole point of asking is that it can be
        // answered no.
        val asking = model.state.value.connect as Connect.AskHost
        assertEquals("git.acme.dev", asking.key.host)
        assertTrue(asking.key.fingerprint.startsWith("SHA256:"))
        assertTrue("trusted without being asked", !bridge.calls.contains("trustHost"))

        bridge.syncOutcome = Outcome.OUTCOME_OK
        model.trustHost()

        assertTrue(bridge.calls.contains("trustHost"))
        assertEquals(Connect.Idle, model.state.value.connect)
        assertNull(model.state.value.overlay)
        assertEquals(model.state.value.repos.single().id, model.state.value.selected)
    }

    @Test
    fun `a refused key lands on the screen that explains it`() = test {
        bridge.syncOutcome = Outcome.OUTCOME_AUTH_DENIED
        bridge.gitOutput = "Permission denied (publickey)."

        val model = model()
        model.start()
        model.showConnect()
        model.connect("git@github.com:acme/thing.git", "main")

        // The raw words, so whoever is debugging a key sees what the host said.
        val denied = model.state.value.connect as Connect.Denied
        assertEquals("Permission denied (publickey).", denied.gitOutput)
        assertEquals(Overlay.Connect, model.state.value.overlay)

        // Retrying after fixing it on the host picks up where it stopped.
        bridge.syncOutcome = Outcome.OUTCOME_OK
        model.retryConnect()
        assertNull(model.state.value.overlay)
    }

    @Test
    fun `connecting once is what folds the key away next time`() = test {
        val model = model()
        model.start()
        assertTrue("the key should start on show", !model.state.value.keyHasConnected)

        model.showConnect()
        model.connect("git@github.com:acme/thing.git", "main")

        // After a host has accepted it, the key is a fact you occasionally
        // check rather than the thing you are here to copy.
        assertTrue(model.state.value.keyHasConnected)
    }

    @Test
    fun `renaming is local and clearable`() = test {
        val model = model()
        model.start()
        model.connect("git@github.com:acme/web-platform.git", "main")
        val id = model.state.value.repos.single().id

        model.rename(id, "The Platform")
        assertEquals("The Platform", model.state.value.repos.single().alias)

        model.rename(id, "")
        assertEquals("", model.state.value.repos.single().alias)
    }

    @Test
    fun `a menu that cannot be read is reported rather than shown empty`() = test {
        bridge.failList = "open repos.json: permission denied"

        val model = model()
        model.start()

        assertTrue(model.state.value.error!!.contains("permission denied"))
    }

    @Test
    fun `the switcher sees what the last view said`() = test {
        val model = model()
        model.start()
        model.connect("git@github.com:acme/thing.git", "main")
        val id = model.state.value.repos.single().id
        bridge.views[id] = FakeBridge.viewOf("something")

        model.refresh()

        // The verdict is written down when a view is read, so the list that
        // shows it has to be re-read afterwards. Without that the switcher
        // says "nothing reported" about a repository whose own screen is
        // showing a green tick.
        assertEquals(Status.STATUS_PASSED, model.state.value.repos.single().ci)
    }

    @Test
    fun `changing branch throws the old branch's commits away`() = test {
        val model = model()
        model.start()
        model.connect("git@github.com:acme/thing.git", "main")
        val id = model.state.value.repos.single().id
        bridge.views[id] = FakeBridge.viewOf("on main")
        model.refresh()
        assertEquals("on main", subjectOf(model.state.value))

        bridge.views[id] = FakeBridge.viewOf("on release")
        model.changeBranch(id, "release")

        assertEquals("release", model.state.value.repos.single().branch)
        // Leaving main's commits under release's name would be the most
        // confusing possible outcome.
        assertEquals("on release", subjectOf(model.state.value))
    }

    // --- the clock and the automatic refresh ---------------------------------

    @Test
    fun `the clock advances while the UI is visible`() = test {
        val model = model()
        model.start()
        model.resume()
        assertEquals(now, model.state.value.nowSeconds)

        now += 5
        scheduler.advanceTimeBy(5_000)
        scheduler.runCurrent()

        // Every timer on screen is formatted against this, so they tick
        // together and the whole screen stays consistent with itself.
        assertEquals(now, model.state.value.nowSeconds)
    }

    @Test
    fun `the open repository refetches on its own`() = test {
        val model = model()
        model.start()
        model.connect("git@github.com:ezcdlabs/clarity.git", "main")
        model.resume()
        bridge.calls.clear()

        scheduler.advanceTimeBy(31_000)
        scheduler.runCurrent()

        assertTrue("expected an unprompted fetch, got ${bridge.calls}", bridge.calls.contains("sync"))
    }

    @Test
    fun `nothing ticks or fetches while the app is in the background`() = test {
        val model = model()
        model.start()
        model.connect("git@github.com:ezcdlabs/clarity.git", "main")
        model.resume()
        val frozen = model.state.value.nowSeconds

        model.pause()
        now += 60
        bridge.calls.clear()
        scheduler.advanceTimeBy(61_000)
        scheduler.runCurrent()

        // A timer nobody can see only spends battery, and a fetch nobody asked
        // for spends their data too.
        assertEquals(frozen, model.state.value.nowSeconds)
        assertTrue("fetched in the background: ${bridge.calls}", !bridge.calls.contains("sync"))
    }

    @Test
    fun `an automatic refresh that fails does not nag over data you can read`() = test {
        val model = model()
        model.start()
        model.connect("git@github.com:ezcdlabs/clarity.git", "main")
        val id = model.state.value.selected!!
        bridge.views[id] = FakeBridge.viewOf("yesterday")
        model.refresh()

        bridge.failSync = "dial tcp: network is unreachable"
        model.resume()
        scheduler.advanceTimeBy(31_000)
        scheduler.runCurrent()

        // Otherwise the error bar reappears every thirty seconds for as long as
        // you are on a train, over a dashboard that reads perfectly well.
        assertNull(model.state.value.error)
        assertEquals("yesterday", subjectOf(model.state.value))
    }

    @Test
    fun `an automatic refresh that fails is reported when there is nothing to read`() = test {
        val model = model()
        model.start()
        // Connected, but nothing on disk to read — so the screen behind the
        // failure is empty, which is the case that has to speak up.
        model.connect("git@github.com:ezcdlabs/clarity.git", "main")
        bridge.failSync = "dial tcp: network is unreachable"
        model.dismissError()

        model.resume()
        scheduler.advanceTimeBy(31_000)
        scheduler.runCurrent()

        // With no view behind it, silence would leave an empty screen and no
        // reason for it.
        assertEquals("dial tcp: network is unreachable", model.state.value.error)
    }

    // --- harness -------------------------------------------------------------

    private class TestBody(
        val bridge: FakeBridge,
        val dispatcher: CoroutineDispatcher,
        val scope: CoroutineScope,
        val scheduler: TestCoroutineScheduler,
    ) {
        var now: Long = 1_700_000_000

        /**
         * Everything runs eagerly on one unconfined dispatcher, so state is
         * settled the moment a call returns — except delays, which the
         * scheduler advances explicitly, which is what makes the clock and the
         * refresh interval testable at all.
         */
        fun model() = ClarityModel(bridge, scope, dispatcher, clock = { now })

        fun subjectOf(state: AppState): String? =
            state.view?.getFlows(0)?.getSections(0)?.getCommits(0)?.subject
    }

    private fun test(body: TestBody.() -> Unit) = runTest {
        val dispatcher = UnconfinedTestDispatcher(testScheduler)
        val job = Job()
        val body2 = TestBody(FakeBridge(), dispatcher, CoroutineScope(dispatcher + job), testScheduler)
        try {
            body2.body()
        } finally {
            // In a finally, not after the body: the pump is an endless delayed
            // loop, and runTest drains the scheduler when the body returns. A
            // test that failed with the pump still running would hang on that
            // drain forever instead of reporting its assertion — which is a
            // harness that punishes you for breaking the thing it tests.
            job.cancel()
        }
    }
}
