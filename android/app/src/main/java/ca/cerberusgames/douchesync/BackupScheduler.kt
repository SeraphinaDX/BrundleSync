// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import android.content.Context
import androidx.work.*
import java.util.concurrent.TimeUnit

object BackupScheduler {
    const val PERIODIC = "camera-periodic"
    const val MANUAL = "camera-now"

    private fun constraints(unmetered: Boolean, charging: Boolean) =
        Constraints.Builder()
            .setRequiredNetworkType(if (unmetered) NetworkType.UNMETERED else NetworkType.CONNECTED)
            .setRequiresCharging(charging)
            .setRequiresBatteryNotLow(true)
            .build()

    fun configure(context: Context, automatic: Boolean, unmetered: Boolean, charging: Boolean) {
        val manager = WorkManager.getInstance(context)
        if (!automatic) {
            manager.cancelUniqueWork(PERIODIC)
            return
        }
        val request =
            PeriodicWorkRequestBuilder<CameraBackupWorker>(15, TimeUnit.MINUTES)
                .setInitialDelay(15, TimeUnit.MINUTES)
                .setConstraints(constraints(unmetered, charging))
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.SECONDS)
                .build()
        manager.enqueueUniquePeriodicWork(
            PERIODIC,
            ExistingPeriodicWorkPolicy.CANCEL_AND_REENQUEUE,
            request,
        )
    }

    fun now(
        context: Context,
        unmetered: Boolean,
        charging: Boolean,
        recheck: Boolean,
        replace: Boolean = false,
    ) {
        val request =
            OneTimeWorkRequestBuilder<CameraBackupWorker>()
                .setInputData(workDataOf("recheck" to recheck))
                .setConstraints(constraints(unmetered, charging))
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.SECONDS)
                .build()
        WorkManager.getInstance(context)
            .enqueueUniqueWork(
                MANUAL,
                if (replace) ExistingWorkPolicy.REPLACE else ExistingWorkPolicy.KEEP,
                request,
            )
    }

    // Stop disables automatic scheduling as well as cancelling active work.
    // The saved checkbox can be enabled again with Save settings.
    fun stop(context: Context) {
        WorkManager.getInstance(context).cancelUniqueWork(PERIODIC)
        WorkManager.getInstance(context).cancelUniqueWork(MANUAL)
    }
}
