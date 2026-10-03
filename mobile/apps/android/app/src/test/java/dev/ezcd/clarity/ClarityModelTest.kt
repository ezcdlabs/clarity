package dev.ezcd.clarity

import dev.ezcd.clarity.bridge.FakeBridge
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
    fun `adding a repository selects it and closes the form`() = test {
        val model = model()
        model.start()
        model.showAddRepo()

        model.addRepo("git@github.com:ezcdlabs/clarity.git", "")

        assertNull(model.state.value.overlay)
        assertEquals(listOf("clarity"), model.state.value.repos.map { it.name })
        assertEquals("main", model.state.value.repos.single().branch)
        assertEquals(model.state.value.repos.single().id, model.state.value.selected)
        assertNull(model.state.value.error)
    }

    @Test
    fun `a rejected URL keeps the form open with the reason`() = test {
        bridge.failAddRepo = "\"nonsense\" does not look like a git remote — paste the URL you would clone"

        val model = model()
        model.start()
        model.showAddRepo()
        model.addRepo("nonsense", "")

        // Closing it would throw away what was typed, and a paste of a clone
        // URL is not something anyone wants to redo.
        assertEquals(Overlay.AddRepo, model.state.value.overlay)
        assertTrue(model.state.value.error!!.contains("does not look like a git remote"))
        assertTrue(model.state.value.repos.isEmpty())
    }

    @Test
    fun `opening a repository shows what is on disk before it fetches`() = test {
        val model = model()
        model.start()
        model.addRepo("git@github.com:ezcdlabs/clarity.git", "main")
        model.addRepo("git@github.com:ezcdlabs/other.git", "main")
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
        model.addRepo("git@github.com:ezcdlabs/clarity.git", "main")
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
        model.addRepo("git@github.com:ezcdlabs/clarity.git", "main")

        // Reading a never-fetched repo fails too, but "reference not found" is
        // not news on a first open — it is the expected state, and surfacing it
        // would bury the reason the fetch did not fix it.
        assertEquals("dial tcp: network is unreachable", model.state.value.error)
        assertNull(model.state.value.view)
    }

    @Test
    fun `a fetch that brings new commits replaces the view`() = test {
        val model = model()
        model.start()
        model.addRepo("git@github.com:ezcdlabs/clarity.git", "main")
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
        model.addRepo("git@github.com:ezcdlabs/clarity.git", "main")
        model.addRepo("git@github.com:ezcdlabs/other.git", "main")
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
        model.addRepo("git@github.com:ezcdlabs/clarity.git", "main")
        val id = model.state.value.repos.single().id
        bridge.views[id] = FakeBridge.viewOf("still here")
        model.select(id)
        model.refresh()

        // The list is a drawer, not a destination: there is no model call for
        // opening it, and nothing about the repository changes when it does.
        model.showAddRepo()
        model.closeOverlay()

        assertEquals(id, model.state.value.selected)
        assertEquals("still here", subjectOf(model.state.value))
    }

    @Test
    fun `removing the open repository falls back to another`() = test {
        val model = model()
        model.start()
        model.addRepo("git@github.com:ezcdlabs/clarity.git", "main")
        model.addRepo("git@github.com:ezcdlabs/other.git", "main")
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
    fun `the next thing that works clears the error`() = test {
        bridge.failAddRepo = "nope"
        val model = model()
        model.start()
        model.showAddRepo()
        model.addRepo("nonsense", "")
        assertEquals("nope", model.state.value.error)

        bridge.failAddRepo = null
        model.addRepo("git@github.com:ezcdlabs/clarity.git", "main")

        assertNull(model.state.value.error)
    }

    @Test
    fun `a menu that cannot be read is reported rather than shown empty`() = test {
        bridge.failList = "open repos.json: permission denied"

        val model = model()
        model.start()

        assertTrue(model.state.value.error!!.contains("permission denied"))
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
        model.addRepo("git@github.com:ezcdlabs/clarity.git", "main")
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
        model.addRepo("git@github.com:ezcdlabs/clarity.git", "main")
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
        model.addRepo("git@github.com:ezcdlabs/clarity.git", "main")
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
        bridge.failSync = "dial tcp: network is unreachable"
        val model = model()
        model.start()
        model.addRepo("git@github.com:ezcdlabs/clarity.git", "main")
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
