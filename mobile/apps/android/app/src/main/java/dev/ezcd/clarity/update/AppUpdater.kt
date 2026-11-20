package dev.ezcd.clarity.update

import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import dev.ezcd.clarity.proto.AppUpdate
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import java.io.File
import java.net.HttpURLConnection
import java.net.URL
import java.security.MessageDigest

/**
 * Downloads a released APK and hands it to the system installer.
 *
 * This is the platform half of the self-updater. Every decision — whether an
 * update exists, which asset, what checksum — was made on the Go side; what
 * is left is by nature Android's: streaming a file, and the PackageInstaller
 * session that ends in the system's own confirmation dialog. Nothing installs
 * silently, and nothing can: the user confirms the system prompt, and the
 * system itself refuses any APK whose signing key differs from the installed
 * app's — which is the security boundary here, not this code.
 *
 * A process-wide object rather than state in the model, because an install in
 * flight belongs to the process, not to a screen: the sheet that started it
 * can close, and the phase has to survive that.
 */
object AppUpdater {

    sealed interface Phase {
        data object Idle : Phase

        /** [fraction] is 0..1, or -1 while the size is still unknown. */
        data class Downloading(val fraction: Float) : Phase

        /** Handed to the system installer; its dialog is on screen or queued. */
        data object Installing : Phase

        data class Failed(val message: String) : Phase
    }

    private val _phase = MutableStateFlow<Phase>(Phase.Idle)
    val phase: StateFlow<Phase> = _phase.asStateFlow()

    /**
     * Fetches [update] and commits an install session. Safe to call again
     * after a failure; a second call while one is already running is ignored
     * rather than queued, because both would be installing the same file.
     */
    fun run(context: Context, update: AppUpdate, scope: CoroutineScope) {
        if (_phase.value is Phase.Downloading || _phase.value is Phase.Installing) return
        val app = context.applicationContext
        _phase.value = Phase.Downloading(-1f)
        scope.launch(Dispatchers.IO) {
            try {
                val apk = download(app, update)
                _phase.value = Phase.Installing
                install(app, apk)
            } catch (e: Exception) {
                _phase.value = Phase.Failed(e.message ?: e.toString())
            }
        }
    }

    /** The install dialog's result, reported by [InstallReceiver]. */
    internal fun finished(status: Int, message: String?) {
        _phase.value = when (status) {
            PackageInstaller.STATUS_SUCCESS -> Phase.Idle // the process is about to be replaced anyway
            PackageInstaller.STATUS_FAILURE_ABORTED -> Phase.Idle // the user said no; that is not a failure to report
            else -> Phase.Failed(message ?: "install failed ($status)")
        }
    }

    private fun download(context: Context, update: AppUpdate): File {
        val dest = File(context.cacheDir, "clarity-update.apk")
        val conn = URL(update.downloadUrl).openConnection() as HttpURLConnection
        conn.setRequestProperty("User-Agent", "git-clarity-app")
        try {
            if (conn.responseCode != 200) throw RuntimeException("download: HTTP ${conn.responseCode}")
            val total = if (update.sizeBytes > 0) update.sizeBytes else conn.contentLengthLong
            val digest = MessageDigest.getInstance("SHA-256")
            conn.inputStream.use { input ->
                dest.outputStream().use { out ->
                    val buf = ByteArray(256 * 1024)
                    var done = 0L
                    while (true) {
                        val n = input.read(buf)
                        if (n < 0) break
                        out.write(buf, 0, n)
                        digest.update(buf, 0, n)
                        done += n
                        if (total > 0) _phase.value = Phase.Downloading(done.toFloat() / total)
                    }
                }
            }
            // An empty checksum means the release published none; the download
            // goes to the installer unverified, whose signature check is the
            // boundary that actually matters.
            if (update.sha256.isNotEmpty()) {
                val got = digest.digest().joinToString("") { "%02x".format(it) }
                if (!got.equals(update.sha256, ignoreCase = true)) {
                    dest.delete()
                    throw RuntimeException("download did not match its published checksum")
                }
            }
            return dest
        } finally {
            conn.disconnect()
        }
    }

    private fun install(context: Context, apk: File) {
        val installer = context.packageManager.packageInstaller
        val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL)
        params.setAppPackageName(context.packageName)
        val sessionId = installer.createSession(params)
        installer.openSession(sessionId).use { session ->
            session.openWrite("clarity-update.apk", 0, apk.length()).use { out ->
                apk.inputStream().use { it.copyTo(out) }
                session.fsync(out)
            }
            val intent = Intent(context, InstallReceiver::class.java)
            val pending = PendingIntent.getBroadcast(
                context, sessionId, intent,
                // Mutable by requirement: the installer writes its status into
                // this very intent.
                PendingIntent.FLAG_MUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
            )
            session.commit(pending.intentSender)
        }
        apk.delete()
    }
}

/**
 * Where the install session reports back. STATUS_PENDING_USER_ACTION carries
 * the system's own confirmation dialog (and, on a first install from this
 * app, the one-time "allow updates from clarity" settings hop) as an intent
 * to launch; everything else is a verdict.
 */
class InstallReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        when (val status = intent.getIntExtra(PackageInstaller.EXTRA_STATUS, Int.MIN_VALUE)) {
            PackageInstaller.STATUS_PENDING_USER_ACTION -> {
                @Suppress("DEPRECATION")
                val confirm = intent.getParcelableExtra<Intent>(Intent.EXTRA_INTENT) ?: return
                confirm.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                context.startActivity(confirm)
            }
            else -> AppUpdater.finished(
                status,
                intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE),
            )
        }
    }
}
