// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.net.Uri
import android.os.Build
import android.os.SystemClock
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.work.Worker
import androidx.work.WorkerParameters
import ca.cerberusgames.douchesync.mobile.Mobile
import ca.cerberusgames.douchesync.mobile.Session
import java.io.File
import java.io.InputStream
import java.io.InterruptedIOException
import java.io.RandomAccessFile
import java.nio.channels.OverlappingFileLockException

class CameraBackupWorker(context: Context, params: WorkerParameters) : Worker(context, params) {
    private var scope = ""
    private var saved = 0
    private var unchangedCount = 0
    private var skipped = 0
    private var phase = "scanning"
    private var lastNotification = 0L
    private val journal
        get() = BackupJournal.get(applicationContext)

    @Volatile private var session: Session? = null
    @Volatile private var activeInput: InputStream? = null

    override fun onStopped() {
        session?.cancel()
        runCatching { activeInput?.close() }
        super.onStopped()
    }

    override fun doWork(): Result {
        val lockFile =
            RandomAccessFile(File(applicationContext.filesDir, "camera-backup.lock"), "rw")
        lockFile.use {
            val lock =
                try {
                    it.channel.tryLock()
                } catch (_: OverlappingFileLockException) {
                    null
                }
            if (lock == null) return Result.retry()
            lock.use {
                return backup()
            }
        }
    }

    private fun backup(): Result {
        val context = applicationContext
        val started = SystemClock.elapsedRealtime()
        var uploaded = 0
        var unchanged = 0
        var recent = 0
        var tooLarge = 0
        val recheck = inputData.getBoolean("recheck", false)
        scope = journal.prefs.getString("scope", "") ?: ""
        try {
            val current = Mobile.newSession(File(context.filesDir, "config.toml").path)
            session = current
            scope = current.scope()
            journal.begin(scope)
            status("Checking camera folder")
            journal.event(scope, "Backup check started")
            if (isStopped) {
                current.cancel()
                return Result.retry()
            }
            val tree = Uri.parse(current.cameraURI())
            if (
                !context.contentResolver.persistedUriPermissions.any {
                    it.uri == tree && it.isReadPermission
                }
            )
                throw SecurityException(
                    "Camera folder permission was lost. Choose the folder again."
                )
            // Only one worker owns this cache directory. Clear an interrupted
            // staging copy before starting; source photos are never modified.
            val staging = File(context.cacheDir, "camera-staging").apply { mkdirs() }
            staging.listFiles()?.forEach { it.delete() }
            BackupIndex(context).use { index ->
                for (item in CameraTree.scan(context.contentResolver, tree) { isStopped }) {
                    if (isStopped) return Result.retry()
                    // Keep jobs short and interruptible; WorkManager can retry
                    // after the next constraint window using durable receipts.
                    if (SystemClock.elapsedRealtime() - started >= 8 * 60_000L) {
                        phase = "retry"
                        status(
                            "Backup will continue: $uploaded saved, $unchanged already backed up"
                        )
                        return Result.retry()
                    }
                    if (index.contains(current.scope(), item, id.toString(), recheck)) {
                        unchanged++
                        unchangedCount = unchanged
                        continue
                    }
                    if (item.size > current.maxFileSize()) {
                        tooLarge++
                        skipped = tooLarge + recent
                        continue
                    }
                    if (
                        item.modified > 0 && System.currentTimeMillis() - item.modified in 0..30_000
                    ) {
                        recent++
                        skipped = tooLarge + recent
                        continue
                    }
                    phase = "uploading"
                    status("Backing up ${item.name} · $uploaded saved")
                    val receipt =
                        context.contentResolver.openFileDescriptor(item.uri, "r")?.use { descriptor
                            ->
                            if (descriptor.statSize >= 0) {
                                // Go takes ownership of the detached descriptor and
                                // closes it on success, cancellation and errors.
                                current.uploadDescriptor(
                                    descriptor.detachFd().toLong(),
                                    item.name,
                                    item.modified,
                                )
                            } else {
                                descriptor.close()
                                // Non-seekable providers need one temporary copy so
                                // the shared engine can hash and resume by offset.
                                stageAndUpload(item, current, staging, started)
                            }
                        } ?: error("Cannot open ${item.name}")
                    val after = CameraTree.metadata(context.contentResolver, item)
                    if (after.first != item.size || after.second != item.modified)
                        error("Camera file changed during backup: ${item.name}")
                    index.replace(current.scope(), item, receipt, id.toString())
                    uploaded++
                    saved = uploaded
                    journal.event(scope, "Saved ${item.name}")
                    status("Saved ${item.name}")
                }
            }
            if (isStopped) return Result.retry()
            phase = if (recent > 0) "retry" else if (tooLarge > 0) "attention" else "complete"
            status(
                "Backup check complete: $uploaded saved, $unchanged already backed up; $tooLarge exceed the file-size limit" +
                    if (recent > 0) "; $recent recent files pending" else ""
            )
            journal.event(
                scope,
                "Backup check finished: $uploaded saved, $unchanged unchanged, ${tooLarge + recent} skipped",
            )
            return if (recent > 0) Result.retry() else Result.success()
        } catch (e: SecurityException) {
            phase = "attention"
            journal.event(scope, "Camera folder permission needed; choose the folder again")
            status("Choose the camera folder again: ${e.message}")
            return Result.failure()
        } catch (e: Exception) {
            phase = "retry"
            if (!isStopped) {
                status("Backup will retry: ${e.message}")
                journal.event(scope, "Backup interrupted; waiting to retry")
            }
            return Result.retry()
        } finally {
            session?.cancel()
            session = null
        }
    }

    private fun stageAndUpload(
        item: CameraItem,
        current: Session,
        staging: File,
        started: Long,
    ): String {
        if (item.size >= 0 && staging.usableSpace < item.size + (8L shl 20))
            error("Not enough temporary storage")
        val copy = File.createTempFile("photo-", ".part", staging)
        try {
            applicationContext.contentResolver.openInputStream(item.uri)?.use { input ->
                activeInput = input
                copy.outputStream().use { output ->
                    val buffer = ByteArray(128 * 1024)
                    var written = 0L
                    while (true) {
                        if (isStopped || SystemClock.elapsedRealtime() - started >= 8 * 60_000L)
                            throw InterruptedIOException(
                                "Backup stopped or continuing in the next job"
                            )
                        val n = input.read(buffer)
                        if (n < 0) break
                        written += n
                        require(written <= current.maxFileSize()) {
                            "Camera file exceeds the configured size limit"
                        }
                        if (
                            written % (8L shl 20) < buffer.size && staging.usableSpace < (8L shl 20)
                        )
                            error("Not enough temporary storage")
                        output.write(buffer, 0, n)
                    }
                    output.fd.sync()
                }
            } ?: error("Cannot read camera source")
            if (item.size >= 0 && copy.length() != item.size)
                error("Camera file changed while copying")
            return current.upload(copy.path, item.name, item.modified)
        } finally {
            activeInput = null
            copy.delete()
        }
    }

    private fun status(message: String) {
        if (isStopped || !journal.accepts(scope)) return
        journal.report(scope, phase, message, saved, unchangedCount, skipped)
        val now = SystemClock.elapsedRealtime()
        if (phase in listOf("scanning", "uploading") && now - lastNotification < 1000) return
        lastNotification = now
        val manager = applicationContext.getSystemService(NotificationManager::class.java)
        if (Build.VERSION.SDK_INT >= 26)
            manager.createNotificationChannel(
                NotificationChannel(
                    "camera-backup",
                    applicationContext.getString(R.string.backup_channel),
                    NotificationManager.IMPORTANCE_LOW,
                )
            )
        val notification =
            NotificationCompat.Builder(applicationContext, "camera-backup")
                .setSmallIcon(R.drawable.ic_notification)
                .setContentTitle(applicationContext.getString(R.string.backup_notification))
                .setContentText(message)
                .setContentIntent(
                    android.app.PendingIntent.getActivity(
                        applicationContext,
                        0,
                        android.content.Intent(applicationContext, MainActivity::class.java),
                        android.app.PendingIntent.FLAG_UPDATE_CURRENT or
                            android.app.PendingIntent.FLAG_IMMUTABLE,
                    )
                )
                .setOnlyAlertOnce(true)
                .setAutoCancel(true)
                .build()
        try {
            NotificationManagerCompat.from(applicationContext).notify(1, notification)
        } catch (_: SecurityException) {
            /* Backup works without notification permission. */
        }
    }
}
