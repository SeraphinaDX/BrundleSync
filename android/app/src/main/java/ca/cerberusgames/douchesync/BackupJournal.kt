// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import android.content.ContentValues
import android.content.Context
import android.database.sqlite.SQLiteDatabase
import android.database.sqlite.SQLiteOpenHelper
import java.text.DateFormat
import java.util.Date

/** Bounded UI history. Upload receipts remain in BackupIndex, independently. */
class BackupJournal private constructor(context: Context) :
    SQLiteOpenHelper(context, "backup-activity.db", null, 1) {
    val prefs = context.getSharedPreferences("backup-status", Context.MODE_PRIVATE)

    override fun onCreate(db: SQLiteDatabase) {
        db.execSQL(
            "CREATE TABLE events (id INTEGER PRIMARY KEY, time INTEGER NOT NULL, message TEXT NOT NULL)"
        )
    }

    override fun onUpgrade(db: SQLiteDatabase, old: Int, new: Int) = Unit

    @Synchronized
    fun configured(scope: String) {
        if (prefs.getString("scope", "") != scope) {
            prefs.edit().clear().putString("scope", scope).apply()
        }
        report(scope, "ready", "Settings saved. Ready for a backup check.", 0, 0, 0)
        event(scope, "Backup settings saved")
        prefs.edit().putLong("config_revision", System.nanoTime()).apply()
    }

    @Synchronized
    fun begin(scope: String) {
        if (!prefs.contains("scope")) prefs.edit().putString("scope", scope).apply()
    }

    @Synchronized fun accepts(scope: String) = prefs.getString("scope", "") == scope

    @Synchronized
    fun report(
        scope: String,
        phase: String,
        message: String,
        saved: Int,
        unchanged: Int,
        skipped: Int,
    ) {
        if (!accepts(scope)) return
        val now = System.currentTimeMillis()
        val edit =
            prefs
                .edit()
                .putString("phase", phase)
                .putString("message", message.take(2000))
                .putInt("saved", saved)
                .putInt("unchanged", unchanged)
                .putInt("skipped", skipped)
                .putLong("updated", now)
        if (phase == "complete") edit.putLong("completed", now)
        edit.putLong("revision", prefs.getLong("revision", 0) + 1).apply()
    }

    @Synchronized
    fun event(scope: String, message: String) {
        if (!accepts(scope)) return
        val db = writableDatabase
        db.beginTransaction()
        try {
            db.insertOrThrow(
                "events",
                null,
                ContentValues().apply {
                    put("time", System.currentTimeMillis())
                    put("message", message.take(2000))
                },
            )
            db.execSQL(
                "DELETE FROM events WHERE id NOT IN (SELECT id FROM events ORDER BY id DESC LIMIT 100)"
            )
            db.setTransactionSuccessful()
        } finally {
            db.endTransaction()
        }
        prefs.edit().putLong("revision", prefs.getLong("revision", 0) + 1).apply()
    }

    @Synchronized
    fun history(): String {
        val lines = mutableListOf<String>()
        readableDatabase.rawQuery("SELECT time, message FROM events ORDER BY id DESC", null).use {
            while (it.moveToNext()) lines.add(
                "${DateFormat.getDateTimeInstance().format(Date(it.getLong(0)))}\n${it.getString(1)}"
            )
        }
        return lines.joinToString("\n\n").ifEmpty { "No backup activity yet." }
    }

    @Synchronized
    fun clearHistory() {
        writableDatabase.delete("events", null, null)
        prefs.edit().putLong("revision", prefs.getLong("revision", 0) + 1).apply()
    }

    companion object {
        @Volatile private var instance: BackupJournal? = null

        fun get(context: Context): BackupJournal =
            instance
                ?: synchronized(this) {
                    instance ?: BackupJournal(context.applicationContext).also { instance = it }
                }
    }
}
