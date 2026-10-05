package dev.ezcd.clarity.watch

import android.Manifest
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import dev.ezcd.clarity.MainActivity
import dev.ezcd.clarity.R
import dev.ezcd.clarity.proto.Change

/**
 * What a background check tells the user, and how loudly.
 *
 * Two channels rather than one, because the two directions are not worth saying
 * the same way. A pipeline going red is the thing you asked to be told about and
 * it may want to interrupt; a pipeline going green again is the thing that says
 * stop worrying, and a buzz for that is a buzz you did not need. Splitting them
 * also puts the dial where Android wants it — the user can mute recoveries, or
 * quieten breakages, in system settings, per channel, without the app growing a
 * settings screen of its own.
 *
 * Deliberately not a full-screen intent, and deliberately nothing that brings
 * the app to the foreground. A periodic check is up to a quarter of an hour
 * behind and Doze can stretch that further, so an alarm would be waking someone
 * at three in the morning over something that broke twenty minutes ago. Android
 * agrees: since 14 the full-screen permission is granted by default only to
 * calling and alarm apps, and a CI dashboard claiming to be one would be a
 * misuse of the thing that gets a phone to ring.
 */
object Notifications {
    const val BROKEN_CHANNEL = "pipeline-broken"
    const val RECOVERED_CHANNEL = "pipeline-recovered"

    /**
     * Declares the channels. Safe to call repeatedly — creating one that exists
     * is a no-op, and the user's own changes to it survive.
     */
    fun ensureChannels(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = context.getSystemService(NotificationManager::class.java) ?: return

        manager.createNotificationChannel(
            NotificationChannel(
                BROKEN_CHANNEL,
                "Pipeline broken",
                // High, so it arrives as a heads-up with a sound. This is the
                // one the feature exists for; anything quieter and you find out
                // when you next open the app, which you could have done anyway.
                NotificationManager.IMPORTANCE_HIGH,
            ).apply { description = "When CI or a deploy starts failing." },
        )
        manager.createNotificationChannel(
            NotificationChannel(
                RECOVERED_CHANNEL,
                "Pipeline recovered",
                // Default: it appears in the shade without a sound. Good news
                // can wait until you look.
                NotificationManager.IMPORTANCE_DEFAULT,
            ).apply { description = "When CI or a deploy goes green again." },
        )
    }

    /** Whether the user has agreed to be told. */
    fun permitted(context: Context): Boolean =
        Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
            ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) ==
            PackageManager.PERMISSION_GRANTED

    /**
     * Posts one change.
     *
     * Keyed on the repository and stage, so a pipeline that breaks and recovers
     * replaces its own notification rather than stacking two contradictory ones
     * in the shade.
     */
    fun post(context: Context, change: Change) {
        if (!permitted(context)) return

        val channel = if (change.broke) BROKEN_CHANNEL else RECOVERED_CHANNEL
        val title = "${change.repoName} · ${change.stage}"
        val body = if (change.broke) "is failing" else "is passing again"

        val open = PendingIntent.getActivity(
            context,
            0,
            Intent(context, MainActivity::class.java)
                .addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )

        val notification = NotificationCompat.Builder(context, channel)
            .setSmallIcon(R.drawable.ic_stat_clarity)
            .setContentTitle(title)
            .setContentText(body)
            .setPriority(
                if (change.broke) NotificationCompat.PRIORITY_HIGH else NotificationCompat.PRIORITY_DEFAULT,
            )
            .setCategory(NotificationCompat.CATEGORY_STATUS)
            .setAutoCancel(true)
            .setContentIntent(open)
            .build()

        try {
            NotificationManagerCompat.from(context).notify(tag(change), 1, notification)
        } catch (_: SecurityException) {
            // The permission was revoked between the check above and here.
            // Nothing to do but not crash a background worker over it.
        }
    }

    /** One slot per repository and stage, so a later verdict replaces an earlier. */
    private fun tag(change: Change): String = "${change.repoId}/${change.stage}"
}
