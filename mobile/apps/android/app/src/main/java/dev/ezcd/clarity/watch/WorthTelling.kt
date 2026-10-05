package dev.ezcd.clarity.watch

import dev.ezcd.clarity.proto.Change

/**
 * Which repository the user is currently looking at, or null if the app is not
 * in front of them.
 *
 * Process-wide and deliberately not persisted. The background check runs in the
 * app's process whenever the app is alive, and when it is not, a fresh process
 * starts this at null — which is the right answer, because nothing is visible.
 */
object Visible {
    @Volatile
    var repoId: String? = null
}

/**
 * Filters a check's findings down to the ones the user cannot already see.
 *
 * The rule is the one every messaging app uses: silence for the conversation
 * you are in, not for the app being open. A repository you are not looking at
 * is one you would otherwise miss, whether or not the app happens to be
 * running — so it is worth a notification either way.
 *
 * The repository on screen is different in kind. Its feed went red within five
 * seconds of it happening, because the open repository is the one the pump
 * refetches; a notification arriving up to a quarter of an hour later is
 * telling you something you watched, late. That holds for a recovery too, and
 * more so — good news you are already looking at is the least interruptible
 * thing there is.
 */
fun worthTelling(changes: List<Change>, visibleRepoId: String?): List<Change> =
    changes.filter { it.repoId != visibleRepoId }
