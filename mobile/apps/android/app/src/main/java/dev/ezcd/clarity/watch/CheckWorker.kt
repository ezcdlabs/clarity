package dev.ezcd.clarity.watch

import android.content.Context
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import dev.ezcd.clarity.bridge.GoBridge
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File
import java.util.concurrent.TimeUnit

/**
 * The background check: fetch every tracked repository, and say what moved.
 *
 * All of the deciding is in Go — which pipelines crossed between green and red,
 * what to call them, and what counts as a crossing at all. This is the part
 * Android has to own: when to run, and how to tell someone.
 */
class CheckWorker(context: Context, params: WorkerParameters) :
    CoroutineWorker(context, params) {

    override suspend fun doWork(): Result = withContext(Dispatchers.IO) {
        if (!Notifications.permitted(applicationContext)) {
            // Nothing to say and no way to say it. Not a failure: the user may
            // grant it later, and a worker that reported failure would be
            // backed off and eventually dropped.
            return@withContext Result.success()
        }

        // Its own bridge. The one the UI holds belongs to a ViewModel that does
        // not exist while the app is closed, which is exactly when this runs.
        val dir = File(applicationContext.filesDir, "clarity").apply { mkdirs() }
        val changes = try {
            GoBridge.open(dir).check(FETCH_TIMEOUT_SECONDS)
        } catch (_: Exception) {
            // A check that cannot run at all is worth retrying on the usual
            // backoff rather than reporting to the user: there is nothing they
            // can do about it, and a notification saying "could not check" every
            // quarter of an hour is the definition of noise.
            return@withContext Result.retry()
        }

        Notifications.ensureChannels(applicationContext)
        for (change in changes.changesList) {
            Notifications.post(applicationContext, change)
        }
        Result.success()
    }

    companion object {
        private const val NAME = "clarity-check"

        /**
         * Fifteen minutes, which is WorkManager's floor rather than a choice.
         * The system may run it later than that and routinely does when the
         * phone is idle; the alternative is a foreground service holding a
         * permanent notification, which is a steep price for a dashboard.
         */
        private const val INTERVAL_MINUTES = 15L

        /** Shorter than the foreground fetch: a worker has a budget to respect. */
        private const val FETCH_TIMEOUT_SECONDS = 45

        /**
         * Schedules the check, or leaves the existing schedule alone.
         *
         * KEEP rather than REPLACE: replacing restarts the interval, so an app
         * opened every few minutes would reset the timer every time and the
         * check would never actually run.
         */
        fun schedule(context: Context) {
            val request = PeriodicWorkRequestBuilder<CheckWorker>(
                INTERVAL_MINUTES, TimeUnit.MINUTES,
            ).setConstraints(
                // No point waking up to fail: every repository is a network
                // fetch, and an offline check is a battery cost with no
                // possible finding.
                Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build(),
            ).build()

            WorkManager.getInstance(context)
                .enqueueUniquePeriodicWork(NAME, ExistingPeriodicWorkPolicy.KEEP, request)
        }

        /** Stops the check, for when there is nothing to watch or no permission. */
        fun cancel(context: Context) {
            WorkManager.getInstance(context).cancelUniqueWork(NAME)
        }
    }
}
