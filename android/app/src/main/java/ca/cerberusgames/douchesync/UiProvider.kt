// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import android.content.ContentProvider
import android.content.ContentValues
import android.content.SharedPreferences
import android.database.Cursor
import android.net.Uri
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import androidx.work.WorkInfo
import androidx.work.WorkManager
import ca.cerberusgames.douchesync.mobile.Mobile
import java.io.File
import java.text.DateFormat
import java.util.Date
import org.json.JSONObject

/**
 * Private, same-UID bridge. All Go engine bindings and WorkManager operations stay in the normal
 * app process; Fyne's renderer lives in its own UI process.
 */
class UiProvider : ContentProvider() {
    private val app
        get() = requireNotNull(context).applicationContext

    private val config
        get() = File(app.filesDir, "config.toml")

    private val journal
        get() = BackupJournal.get(app)

    @Volatile private var manual = emptyList<WorkInfo>()
    @Volatile private var periodic = emptyList<WorkInfo>()
    private val listener = SharedPreferences.OnSharedPreferenceChangeListener { _, key ->
        if (key == "revision" || key == "config_revision") changed()
    }

    override fun onCreate(): Boolean {
        journal.prefs.registerOnSharedPreferenceChangeListener(listener)
        Handler(Looper.getMainLooper()).post {
            val manager = WorkManager.getInstance(app)
            manager.getWorkInfosForUniqueWorkLiveData(BackupScheduler.MANUAL).observeForever {
                manual = it
                changed()
            }
            manager.getWorkInfosForUniqueWorkLiveData(BackupScheduler.PERIODIC).observeForever {
                periodic = it
                changed()
            }
        }
        return true
    }

    private fun changed() = app.contentResolver.notifyChange(uri(app.packageName), null)

    private fun hasGrant(value: String) =
        app.contentResolver.persistedUriPermissions.any {
            it.uri == Uri.parse(value) && it.isReadPermission
        }

    private fun settings(): JSONObject {
        if (config.exists()) return JSONObject(Mobile.settings(config.path))
        val prefs = app.getSharedPreferences("ui-draft", 0)
        val id =
            prefs.getString("device", null)
                ?: Mobile.newDeviceID().also {
                    prefs.edit().putString("device", it).apply()
                }
        return JSONObject().put("deviceID", id).put("unmetered", true)
    }

    @Synchronized
    override fun call(method: String, arg: String?, extras: Bundle?): Bundle {
        val result =
            try {
                when (method) {
                    "snapshot" -> snapshot()
                    "results" -> {
                        val prefs = app.getSharedPreferences("ui-results", 0)
                        val pending = JSONObject(prefs.getString("result", "{}") ?: "{}")
                        prefs.edit().remove("result").commit()
                        pending
                    }
                    "save" -> {
                        val s =
                            JSONObject(extras?.getString("payload") ?: error("Missing settings"))
                        val camera = s.getString("cameraURI")
                        require(hasGrant(camera)) {
                            "Choose your camera folder to retain read permission."
                        }
                        Mobile.configure(
                            config.path,
                            s.getString("deviceID").trim(),
                            s.getString("discoveryURL").trim(),
                            s.getString("discoveryToken"),
                            s.getString("folderID").trim(),
                            s.getString("secret"),
                            camera,
                            s.getBoolean("automatic"),
                            s.getBoolean("unmetered"),
                            s.getBoolean("charging"),
                        )
                        val saved = settings()
                        BackupScheduler.stop(app)
                        journal.configured(saved.getString("scope"))
                        BackupScheduler.configure(
                            app,
                            saved.getBoolean("automatic"),
                            saved.getBoolean("unmetered"),
                            saved.getBoolean("charging"),
                        )
                        BackupScheduler.now(
                            app,
                            saved.getBoolean("unmetered"),
                            saved.getBoolean("charging"),
                            false,
                            replace = true,
                        )
                        changed()
                        JSONObject()
                    }
                    "backup",
                    "recheck" -> {
                        require(config.exists()) { "Finish setup and save first." }
                        val s = settings()
                        require(hasGrant(s.getString("cameraURI"))) {
                            "Choose the camera folder again."
                        }
                        BackupScheduler.now(
                            app,
                            s.getBoolean("unmetered"),
                            s.getBoolean("charging"),
                            method == "recheck",
                        )
                        JSONObject()
                    }
                    "stop" -> {
                        BackupScheduler.stop(app)
                        if (config.exists()) {
                            Mobile.setAutomatic(config.path, false)
                            val scope = settings().getString("scope")
                            val prefs = journal.prefs
                            journal.report(
                                scope,
                                "stopped",
                                "Backups stopped. Enable automatic backup in Settings to resume scheduling.",
                                prefs.getInt("saved", 0),
                                prefs.getInt("unchanged", 0),
                                prefs.getInt("skipped", 0),
                            )
                            journal.event(scope, "Backups stopped")
                        }
                        changed()
                        JSONObject()
                    }
                    "clear" -> {
                        journal.clearHistory()
                        JSONObject()
                    }
                    else -> error("Unknown UI action")
                }
            } catch (e: Exception) {
                JSONObject().put("error", e.message ?: "Android operation failed")
            }
        return Bundle().apply { putString("json", result.toString()) }
    }

    private fun snapshot(): JSONObject {
        val s = settings()
        val configured = config.exists()
        val ready = configured && hasGrant(s.getString("cameraURI"))
        val running = (manual + periodic).any { it.state == WorkInfo.State.RUNNING }
        val queued = manual.any { !it.state.isFinished }
        val scheduled = periodic.any { !it.state.isFinished }
        val prefs = journal.prefs
        val matches = configured && prefs.getString("scope", "") == s.optString("scope")
        val phase = if (matches) prefs.getString("phase", "") ?: "" else ""
        val completed = if (matches) prefs.getLong("completed", 0) else 0
        return JSONObject()
            .put("settings", s)
            .put("configured", configured)
            .put("ready", ready)
            .put("running", running)
            .put(
                "state",
                if (ready) backupState(running, queued, scheduled, s.optBoolean("automatic"), phase)
                else "Finish setup in Settings",
            )
            .put(
                "message",
                if (matches) prefs.getString("message", "No backup check yet.")
                else "No backup check yet.",
            )
            .put("saved", if (matches) prefs.getInt("saved", 0) else 0)
            .put("unchanged", if (matches) prefs.getInt("unchanged", 0) else 0)
            .put("skipped", if (matches) prefs.getInt("skipped", 0) else 0)
            .put(
                "completed",
                if (completed > 0) DateFormat.getDateTimeInstance().format(Date(completed)) else "",
            )
            .put("history", journal.history())
    }

    override fun query(
        u: Uri,
        p: Array<out String>?,
        s: String?,
        a: Array<out String>?,
        sort: String?,
    ): Cursor? = null

    override fun getType(u: Uri): String? = null

    override fun insert(u: Uri, v: ContentValues?): Uri? = throw UnsupportedOperationException()

    override fun delete(u: Uri, s: String?, a: Array<out String>?): Int =
        throw UnsupportedOperationException()

    override fun update(u: Uri, v: ContentValues?, s: String?, a: Array<out String>?): Int =
        throw UnsupportedOperationException()

    companion object {
        fun uri(packageName: String) = Uri.parse("content://$packageName.ui")
    }
}
