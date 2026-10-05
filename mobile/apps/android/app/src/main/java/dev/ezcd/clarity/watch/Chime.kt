package dev.ezcd.clarity.watch

import android.content.Context
import android.media.AudioManager
import android.media.ToneGenerator
import android.os.Build
import android.os.VibrationEffect
import android.os.Vibrator
import android.os.VibratorManager
import dev.ezcd.clarity.proto.Change

/**
 * The small noise a screen you are already looking at deserves.
 *
 * The counterpart to a notification rather than a quieter version of one. A
 * messaging app chimes for the conversation you are in and notifies for the
 * ones you are not, and the distinction is the point: a notification is for
 * something you would otherwise miss, and you are not going to miss a feed that
 * just turned red in front of you. What you might miss is the *moment* — the
 * app is open on a desk and the column changes while you are looking elsewhere.
 *
 * Deliberately not the notification tone. Hearing that and finding nothing in
 * the shade is worse than hearing nothing, and reusing it would make the two
 * cases indistinguishable to the one sense that was supposed to tell them
 * apart.
 */
object Chime {

    /**
     * Sounds for what a refresh found.
     *
     * One cue for the batch, not one per crossing: two pipelines that break in
     * the same refresh are one piece of news, and two overlapping tones are an
     * error sound. Breaking wins the tie — it is the half you must not miss.
     */
    fun forCrossings(context: Context, changes: List<Change>) {
        if (changes.isEmpty()) return
        play(context, broke = changes.any { it.broke })
    }

    private fun play(context: Context, broke: Boolean) {
        buzz(context, broke)
        beep(broke)
    }

    /**
     * On the notification stream, so the phone's own rules apply: silenced by
     * silent mode and by Do Not Disturb, and turned down by the volume the user
     * already set for this kind of thing. An app that routes a cue around those
     * is an app that gets uninstalled on a train.
     */
    private fun beep(broke: Boolean) {
        val tone = try {
            ToneGenerator(AudioManager.STREAM_NOTIFICATION, VOLUME)
        } catch (_: RuntimeException) {
            // The stream can be unavailable — another app holding it, or an
            // emulator with no audio. Not worth failing a refresh over.
            return
        }
        // Low for broken, high for recovered, which is the direction the news
        // goes and needs no learning.
        tone.startTone(
            if (broke) ToneGenerator.TONE_PROP_NACK else ToneGenerator.TONE_PROP_ACK,
            DURATION_MS,
        )
        // Released on a delay rather than at once: releasing mid-tone cuts it.
        android.os.Handler(android.os.Looper.getMainLooper())
            .postDelayed({ tone.release() }, (DURATION_MS + 150).toLong())
    }

    /**
     * A buzz as well as a tone, because the case this exists for is a phone
     * face-up on a desk next to someone looking at a different screen — and
     * because it is the half that still works when the volume is down.
     */
    private fun buzz(context: Context, broke: Boolean) {
        val vibrator = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            (context.getSystemService(VibratorManager::class.java))?.defaultVibrator
        } else {
            @Suppress("DEPRECATION")
            context.getSystemService(Vibrator::class.java)
        } ?: return
        if (!vibrator.hasVibrator()) return

        val effect = if (broke) {
            // Two taps for trouble, one for relief. Distinguishable in a
            // pocket, which is the only place a haptic has to work.
            VibrationEffect.createWaveform(longArrayOf(0, 40, 90, 40), -1)
        } else {
            VibrationEffect.createOneShot(30, VibrationEffect.DEFAULT_AMPLITUDE)
        }
        vibrator.vibrate(effect)
    }

    /** Of ToneGenerator's 0-100. Quiet: this is a cue, not an announcement. */
    private const val VOLUME = 55
    private const val DURATION_MS = 160
}
