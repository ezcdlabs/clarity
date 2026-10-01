package dev.ezcd.clarity

import dev.ezcd.clarity.bridge.FakeBridge
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class ClarityModelTest {

    /**
     * Everything runs eagerly on one unconfined dispatcher, so state is settled
     * the moment a call returns and no test has to guess at a delay.
     */
    private fun TestBody.model() = ClarityModel(bridge, scope, dispatcher)

    @Test
    fun `the menu shows what was already tracked`() = test {
        bridge.addRepo("git@github.com:ezcdlabs/clarity.git", "main")
        bridge.calls.clear()

        val model = model()
        model.start()

        assertEquals(listOf("clarity"), model.state.value.repos.map { it.name })
        assertEquals(Screen.Repos, model.state.value.screen)
    }

    @Test
    fun `adding a repository lists it and returns to the menu`() = test {
        val model = model()
        model.start()
        model.showAddRepo()

        model.addRepo("git@github.com:ezcdlabs/clarity.git", "")

        assertEquals(Screen.Repos, model.state.value.screen)
        assertEquals(listOf("clarity"), model.state.value.repos.map { it.name })
        assertEquals("main", model.state.value.repos.single().branch)
        assertNull(model.state.value.error)
    }

    @Test
    fun `a rejected URL keeps you on the add screen with the reason`() = test {
        bridge.failAddRepo = "\"nonsense\" does not look like a git remote — paste the URL you would clone"

        val model = model()
        model.start()
        model.showAddRepo()
        model.addRepo("nonsense", "")

        // Staying put matters: navigating away would throw away what was typed,
        // and a paste of a clone URL is not something anyone wants to redo.
        assertEquals(Screen.AddRepo, model.state.value.screen)
        assertTrue(model.state.value.error!!.contains("does not look like a git remote"))
        assertTrue(model.state.value.repos.isEmpty())
    }

    @Test
    fun `opening a repository shows what is on disk before it fetches`() = test {
        val model = model()
        model.start()
        model.addRepo("git@github.com:ezcdlabs/clarity.git", "main")
        val id = model.state.value.repos.single().id
        bridge.views[id] = FakeBridge.viewOf("what we already had")

        var atFetch: String? = null
        bridge.onSync = { atFetch = subjectOf(model.state.value) }

        model.openRepo(id)

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
        model.openRepo(id)

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
        val id = model.state.value.repos.single().id

        model.openRepo(id)

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
        model.openRepo(id)

        bridge.onSync = { bridge.views[it] = FakeBridge.viewOf("today") }
        model.refresh()

        assertEquals("today", subjectOf(model.state.value))
    }

    @Test
    fun `leaving a repository drops its view`() = test {
        val model = model()
        model.start()
        model.addRepo("git@github.com:ezcdlabs/clarity.git", "main")
        val id = model.state.value.repos.single().id
        bridge.views[id] = FakeBridge.viewOf("yesterday")
        model.openRepo(id)

        model.back()

        // Otherwise opening a second, never-fetched repo would briefly show the
        // first one's commits.
        assertEquals(Screen.Repos, model.state.value.screen)
        assertNull(model.state.value.view)
    }

    @Test
    fun `removing a repository drops it from the menu`() = test {
        val model = model()
        model.start()
        model.addRepo("git@github.com:ezcdlabs/clarity.git", "main")
        model.addRepo("git@github.com:ezcdlabs/other.git", "main")
        val id = model.state.value.repos.first().id

        model.removeRepo(id)

        assertEquals(listOf("other"), model.state.value.repos.map { it.name })
    }

    @Test
    fun `the key screen generates a key on demand`() = test {
        val model = model()
        model.start()
        assertEquals(false, bridge.keyExists)

        model.showKey()

        assertEquals(Screen.Key, model.state.value.screen)
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

    // --- harness -------------------------------------------------------------

    private class TestBody(
        val bridge: FakeBridge,
        val dispatcher: CoroutineDispatcher,
        val scope: CoroutineScope,
    )

    private fun test(body: TestBody.() -> Unit) = runTest {
        val dispatcher = UnconfinedTestDispatcher(testScheduler)
        TestBody(FakeBridge(), dispatcher, CoroutineScope(dispatcher)).body()
    }

    private fun subjectOf(state: AppState): String? =
        state.view?.getFlows(0)?.getGroups(0)?.getCommits(0)?.subject
}
