// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

/** WorkManager is authoritative; persisted progress alone never means running. */
internal fun backupState(
    running: Boolean,
    queued: Boolean,
    periodic: Boolean,
    automatic: Boolean,
    phase: String,
): String =
    when {
        running -> if (phase == "uploading") "Backing up" else "Checking camera folder"
        phase == "attention" -> "Needs attention"
        phase == "retry" && (queued || periodic) -> "Waiting to retry · Android constraints apply"
        queued -> "Queued · waiting for Android, network or charging"
        automatic && periodic -> "Automatic backup enabled · waiting for the next run"
        phase == "stopped" -> "Backups stopped"
        phase == "complete" -> "Backup check complete"
        phase == "uploading" || phase == "scanning" -> "Backup interrupted · ready to check again"
        else -> "Ready for a backup check"
    }
