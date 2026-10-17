// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import android.content.ContentValues
import android.content.Context
import android.database.sqlite.SQLiteDatabase
import android.database.sqlite.SQLiteOpenHelper

/** Record success only after Go validates the pinned peer's durable receipt. */
class BackupIndex(context: Context) : SQLiteOpenHelper(context, "camera-backups.db", null, 1) {
    override fun onCreate(db: SQLiteDatabase) {
        db.execSQL(
            "CREATE TABLE receipts(scope TEXT NOT NULL, uri TEXT NOT NULL, size INTEGER NOT NULL, modified INTEGER NOT NULL, receipt TEXT NOT NULL, checked_run TEXT NOT NULL, PRIMARY KEY(scope,uri))"
        )
    }

    override fun onUpgrade(db: SQLiteDatabase, oldVersion: Int, newVersion: Int) {
        error("Unsupported backup index migration")
    }

    fun contains(scope: String, item: CameraItem, run: String, recheck: Boolean): Boolean {
        // Providers without trustworthy metadata must be re-read; never cache
        // every unknown-size or zero-timestamp file as if it were unchanged.
        readableDatabase
            .rawQuery(
                "SELECT size, modified, checked_run FROM receipts WHERE scope=? AND uri=?",
                arrayOf(scope, item.uri.toString()),
            )
            .use {
                if (!it.moveToFirst()) return false
                if (item.size < 0 || item.modified <= 0) return it.getString(2) == run
                return it.getLong(0) == item.size &&
                    it.getLong(1) == item.modified &&
                    (!recheck || it.getString(2) == run)
            }
    }

    fun replace(scope: String, item: CameraItem, receipt: String, run: String) {
        val values =
            ContentValues().apply {
                put("scope", scope)
                put("uri", item.uri.toString())
                put("size", item.size)
                put("modified", item.modified)
                put("receipt", receipt)
                put("checked_run", run)
            }
        check(
            writableDatabase.insertWithOnConflict(
                "receipts",
                null,
                values,
                SQLiteDatabase.CONFLICT_REPLACE,
            ) != -1L
        )
    }
}
