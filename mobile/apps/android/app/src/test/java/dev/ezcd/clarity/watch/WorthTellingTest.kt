package dev.ezcd.clarity.watch

import dev.ezcd.clarity.proto.Change
import org.junit.Assert.assertEquals
import org.junit.Test

class WorthTellingTest {

    private fun change(repoId: String, stage: String = "CI", broke: Boolean = true): Change =
        Change.newBuilder()
            .setRepoId(repoId)
            .setRepoName(repoId)
            .setStage(stage)
            .setBroke(broke)
            .build()

    @Test
    fun `the repository on screen is not worth a notification`() {
        val changes = listOf(change("open"))

        // You are looking at it. The feed turned red five seconds after it
        // happened, and a buzz up to a quarter of an hour later about something
        // you watched is noise with a delay on it.
        assertEquals(emptyList<Change>(), worthTelling(changes, visibleRepoId = "open"))
    }

    @Test
    fun `the repositories you are not looking at still are`() {
        val changes = listOf(change("open"), change("other"))

        val got = worthTelling(changes, visibleRepoId = "open")

        // The whole of the WhatsApp rule: silent for the conversation you are
        // in, not for the app being open.
        assertEquals(listOf("other"), got.map { it.repoId })
    }

    @Test
    fun `with the app closed everything is worth telling`() {
        val changes = listOf(change("a"), change("b"))

        assertEquals(2, worthTelling(changes, visibleRepoId = null).size)
    }

    @Test
    fun `a recovery on screen is suppressed too`() {
        val changes = listOf(change("open", broke = false))

        // Same reasoning, and more so: good news you are already looking at is
        // the least interruptible thing there is.
        assertEquals(emptyList<Change>(), worthTelling(changes, visibleRepoId = "open"))
    }

    @Test
    fun `every stage of the repository on screen is suppressed, not just one`() {
        val changes = listOf(
            change("open", stage = "CI"),
            change("open", stage = "deploy to ios"),
            change("other", stage = "CI"),
        )

        assertEquals(listOf("other"), worthTelling(changes, visibleRepoId = "open").map { it.repoId })
    }
}
