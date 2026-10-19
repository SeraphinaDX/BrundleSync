// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import android.Manifest
import android.app.AlertDialog
import android.content.Intent
import android.content.SharedPreferences
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.view.View
import android.widget.Button
import android.widget.CheckBox
import android.widget.EditText
import android.widget.ProgressBar
import android.widget.RadioGroup
import android.widget.TextView
import androidx.activity.ComponentActivity
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.widget.doAfterTextChanged
import androidx.lifecycle.Lifecycle
import androidx.work.WorkInfo
import androidx.work.WorkManager
import ca.cerberusgames.douchesync.mobile.Mobile
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanOptions
import java.io.File
import java.text.DateFormat
import java.util.Date
import java.util.concurrent.Executors
import org.json.JSONObject

class MainActivity : ComponentActivity() {
    private val io = Executors.newSingleThreadExecutor()
    private var cameraURI: Uri? = null
    private var dirty = false
    private var editVersion = 0
    private var populating = false
    private var selectedPage = 2
    private var savedSettings: JSONObject? = null
    private var manualWork = emptyList<WorkInfo>()
    private var periodicWork = emptyList<WorkInfo>()
    private val journal
        get() = BackupJournal.get(this)

    private val scanner =
        registerForActivityResult(ScanContract()) { result ->
            result.contents?.let { reviewSetup(it) }
        }
    private val refresh = Runnable { if (!isDestroyed) renderStatus() }
    private var refreshPending = false

    private fun edited() {
        if (populating) return
        dirty = true
        editVersion++
        renderStatus()
    }

    private val configFile
        get() = File(filesDir, "config.toml")

    private val statusPrefs
        get() = getSharedPreferences("backup-status", MODE_PRIVATE)

    private val statusListener = SharedPreferences.OnSharedPreferenceChangeListener { _, key ->
        if (key == "config_revision") refreshSavedSettings()
        if (key == "revision" && !refreshPending) {
            refreshPending = true
            window.decorView.postDelayed(
                {
                    refreshPending = false
                    refresh.run()
                },
                300,
            )
        }
    }
    private val notifications =
        registerForActivityResult(ActivityResultContracts.RequestPermission()) {}
    private val folderPicker =
        registerForActivityResult(ActivityResultContracts.OpenDocumentTree()) { uri ->
            if (uri != null) {
                try {
                    contentResolver.takePersistableUriPermission(
                        uri,
                        Intent.FLAG_GRANT_READ_URI_PERMISSION,
                    )
                    cameraURI = uri
                    edited()
                    findViewById<TextView>(R.id.source_folder).text = uri.toString()
                    renderStatus()
                } catch (e: SecurityException) {
                    showStatus("Folder permission could not be retained: ${e.message}")
                }
            }
        }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Target SDK 36 enforces edge-to-edge. Apply system insets instead of
        // letting editable controls disappear behind status/navigation bars.
        setContentView(R.layout.activity_main)
        findViewById<android.view.View>(android.R.id.content).setOnApplyWindowInsetsListener {
            view,
            insets ->
            if (Build.VERSION.SDK_INT >= 30) {
                val bars = insets.getInsets(android.view.WindowInsets.Type.systemBars())
                view.setPadding(bars.left, bars.top, bars.right, bars.bottom)
            } else {
                @Suppress("DEPRECATION")
                view.setPadding(
                    insets.systemWindowInsetLeft,
                    insets.systemWindowInsetTop,
                    insets.systemWindowInsetRight,
                    insets.systemWindowInsetBottom,
                )
            }
            insets
        }
        if (configFile.exists()) {
            try {
                savedSettings = JSONObject(Mobile.settings(configFile.path))
                populate(savedSettings!!)
                selectedPage = 0
            } catch (e: Exception) {
                showStatus("Cannot load configuration: ${e.message}")
            }
        } else {
            edit(R.id.device_id).setText(Mobile.newDeviceID())
        }
        findViewById<RadioGroup>(R.id.page_navigation).setOnCheckedChangeListener { _, id ->
            val index =
                when (id) {
                    R.id.nav_status -> 0
                    R.id.nav_activity -> 1
                    else -> 2
                }
            selectedPage = index
            listOf(R.id.status_page, R.id.activity_page, R.id.settings_page).forEachIndexed {
                i,
                page ->
                findViewById<View>(page).visibility = if (i == index) View.VISIBLE else View.GONE
            }
        }
        findViewById<Button>(R.id.open_settings).setOnClickListener { showPage(2) }
        findViewById<Button>(R.id.scan_setup).setOnClickListener {
            scanner.launch(
                ScanOptions()
                    .setDesiredBarcodeFormats(ScanOptions.QR_CODE)
                    .setPrompt("Scan your DoucheSync desktop setup QR")
                    .setBeepEnabled(false)
                    .setOrientationLocked(false)
            )
        }
        findViewById<Button>(R.id.manual_settings).setOnClickListener {
            val fields = findViewById<View>(R.id.connection_fields)
            fields.visibility = if (fields.visibility == View.VISIBLE) View.GONE else View.VISIBLE
        }
        findViewById<Button>(R.id.clear_activity).setOnClickListener {
            io.execute { journal.clearHistory() }
        }
        val manager = WorkManager.getInstance(this)
        manager.getWorkInfosForUniqueWorkLiveData(BackupScheduler.MANUAL).observe(this) {
            manualWork = it
            renderStatus()
        }
        manager.getWorkInfosForUniqueWorkLiveData(BackupScheduler.PERIODIC).observe(this) {
            periodicWork = it
            renderStatus()
        }
        selectedPage = savedInstanceState?.getInt("page", selectedPage) ?: selectedPage
        showPage(selectedPage)
        if (savedInstanceState?.containsKey("cameraURI") == true) {
            cameraURI = savedInstanceState.getString("cameraURI")?.let(Uri::parse)
            findViewById<TextView>(R.id.source_folder).text =
                cameraURI?.toString() ?: getString(R.string.no_folder)
        }
        findViewById<Button>(R.id.choose_folder).setOnClickListener {
            folderPicker.launch(cameraURI)
        }
        findViewById<Button>(R.id.save).setOnClickListener { save() }
        findViewById<Button>(R.id.backup_now).setOnClickListener { enqueue(false) }
        findViewById<Button>(R.id.recheck).setOnClickListener { enqueue(true) }
        findViewById<Button>(R.id.stop).setOnClickListener {
            BackupScheduler.stop(this)
            populating = true
            findViewById<CheckBox>(R.id.automatic).isChecked = false
            populating = false
            io.execute {
                try {
                    if (configFile.exists()) {
                        Mobile.setAutomatic(configFile.path, false)
                        val settings = JSONObject(Mobile.settings(configFile.path))
                        onScreen {
                            if (!isDestroyed) {
                                savedSettings = settings
                                renderStatus()
                            }
                        }
                    }
                    BackupScheduler.stop(this)
                } catch (e: Exception) {
                    onScreen {
                        showStatus("Could not save disabled automatic backup: ${e.message}")
                    }
                }
            }
            showStatus("Backups stopped. Save settings to enable automatic backup again.")
        }
        for (id in
            intArrayOf(
                R.id.device_id,
                R.id.discovery_url,
                R.id.discovery_token,
                R.id.folder_id,
                R.id.folder_secret,
            )) {
            edit(id).doAfterTextChanged { edited() }
        }
        for (id in intArrayOf(R.id.automatic, R.id.unmetered, R.id.charging)) {
            val checkbox = findViewById<CheckBox>(id)
            checkbox.setOnCheckedChangeListener { _, _ -> edited() }
        }
    }

    override fun onSaveInstanceState(outState: Bundle) {
        outState.putInt("page", selectedPage)
        outState.putString("cameraURI", cameraURI?.toString())
        outState.putBoolean("dirty", dirty)
        super.onSaveInstanceState(outState)
    }

    override fun onRestoreInstanceState(state: Bundle) {
        super.onRestoreInstanceState(state)
        dirty = state.getBoolean("dirty")
        renderStatus()
    }

    override fun onResume() {
        super.onResume()
        statusPrefs.registerOnSharedPreferenceChangeListener(statusListener)
        refreshSavedSettings()
        renderStatus()
    }

    override fun onPause() {
        statusPrefs.unregisterOnSharedPreferenceChangeListener(statusListener)
        super.onPause()
    }

    override fun onDestroy() {
        io.shutdown()
        super.onDestroy()
    }

    private fun onScreen(action: () -> Unit) {
        runOnUiThread { if (!isDestroyed) action() }
    }

    private fun refreshSavedSettings() {
        val version = editVersion
        io.execute {
            try {
                val settings =
                    if (configFile.exists()) JSONObject(Mobile.settings(configFile.path)) else null
                onScreen {
                    savedSettings = settings
                    if (!dirty && version == editVersion && settings != null) populate(settings)
                    renderStatus()
                }
            } catch (_: Exception) {
                onScreen {
                    showStatus("Cannot read saved configuration. Review settings and save again.")
                }
            }
        }
    }

    private fun edit(id: Int) = findViewById<EditText>(id)

    private fun checked(id: Int) = findViewById<CheckBox>(id).isChecked

    private fun showStatus(message: String) {
        findViewById<TextView>(R.id.settings_message).text = message
    }

    private fun populate(s: JSONObject) {
        populating = true
        edit(R.id.device_id).setText(s.getString("deviceID"))
        edit(R.id.discovery_url).setText(s.getString("discoveryURL"))
        edit(R.id.discovery_token).setText(s.getString("discoveryToken"))
        edit(R.id.folder_id).setText(s.getString("folderID"))
        edit(R.id.folder_secret).setText(s.getString("secret"))
        cameraURI = Uri.parse(s.getString("cameraURI"))
        findViewById<TextView>(R.id.source_folder).text = cameraURI.toString()
        findViewById<CheckBox>(R.id.automatic).isChecked = s.getBoolean("automatic")
        findViewById<CheckBox>(R.id.unmetered).isChecked = s.getBoolean("unmetered")
        findViewById<CheckBox>(R.id.charging).isChecked = s.getBoolean("charging")
        populating = false
    }

    private fun save() {
        val uri = cameraURI ?: return showStatus(getString(R.string.no_folder))
        if (!hasGrant(uri))
            return showStatus("Choose the camera folder again to restore permission.")
        val args =
            arrayOf(
                edit(R.id.device_id).text.toString().trim(),
                edit(R.id.discovery_url).text.toString().trim(),
                edit(R.id.discovery_token).text.toString(),
                edit(R.id.folder_id).text.toString().trim(),
                edit(R.id.folder_secret).text.toString(),
            )
        val automatic = checked(R.id.automatic)
        val unmetered = checked(R.id.unmetered)
        val charging = checked(R.id.charging)
        val version = editVersion
        findViewById<Button>(R.id.save).isEnabled = false
        io.execute {
            try {
                Mobile.configure(
                    configFile.path,
                    args[0],
                    args[1],
                    args[2],
                    args[3],
                    args[4],
                    uri.toString(),
                    automatic,
                    unmetered,
                    charging,
                )
                val settings = JSONObject(Mobile.settings(configFile.path))
                BackupScheduler.stop(this)
                journal.configured(settings.getString("scope"))
                BackupScheduler.configure(this, automatic, unmetered, charging)
                BackupScheduler.now(this, unmetered, charging, false, replace = true)
                onScreen {
                    if (isDestroyed) return@onScreen
                    savedSettings = settings
                    if (editVersion == version) dirty = false
                    renderStatus()
                    showPage(0)
                    requestNotifications()
                    showStatus(getString(R.string.saved))
                }
            } catch (e: Exception) {
                onScreen { showStatus("Settings were not saved: ${e.message}") }
            } finally {
                onScreen { findViewById<Button>(R.id.save).isEnabled = true }
            }
        }
    }

    private fun enqueue(recheck: Boolean) {
        if (dirty) return showStatus("Save your changed settings before starting a backup.")
        if (!configFile.exists()) return showStatus(getString(R.string.not_configured))
        requestNotifications()
        io.execute {
            try {
                val s = JSONObject(Mobile.settings(configFile.path))
                BackupScheduler.now(
                    this,
                    s.getBoolean("unmetered"),
                    s.getBoolean("charging"),
                    recheck,
                )
                onScreen {
                    if (!isDestroyed) {
                        showStatus(getString(R.string.queued))
                        showPage(0)
                    }
                }
            } catch (e: Exception) {
                onScreen { showStatus("Cannot queue backup: ${e.message}") }
            }
        }
    }

    private fun requestNotifications() {
        if (
            lifecycle.currentState.isAtLeast(Lifecycle.State.RESUMED) &&
                Build.VERSION.SDK_INT >= 33 &&
                checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) !=
                    android.content.pm.PackageManager.PERMISSION_GRANTED
        )
            notifications.launch(Manifest.permission.POST_NOTIFICATIONS)
    }

    private fun showPage(index: Int) {
        findViewById<RadioGroup>(R.id.page_navigation)
            .check(listOf(R.id.nav_status, R.id.nav_activity, R.id.nav_settings)[index])
    }

    internal fun reviewSetup(code: String): AlertDialog? {
        return try {
            val setup = JSONObject(Mobile.decodeSetup(code))
            AlertDialog.Builder(this)
                .setTitle("Use this backup destination?")
                .setMessage(
                    "Server: ${setup.getString("discoveryURL")}\nFolder: ${setup.getString("folderID")}\n\nChoose a camera folder and save to finish setup."
                )
                .setNegativeButton("Cancel", null)
                .setPositiveButton("Use setup") { _, _ ->
                    edit(R.id.discovery_url).setText(setup.getString("discoveryURL"))
                    edit(R.id.discovery_token).setText(setup.getString("discoveryToken"))
                    edit(R.id.folder_id).setText(setup.getString("folderID"))
                    edit(R.id.folder_secret).setText(setup.getString("secret"))
                    findViewById<View>(R.id.connection_fields).visibility = View.GONE
                    showStatus(
                        "Destination imported. Choose your camera folder, then Save & start backup."
                    )
                    renderStatus()
                }
                .show()
        } catch (_: Exception) {
            showStatus(
                "This is not a valid DoucheSync camera setup QR code. Settings were not changed."
            )
            null
        }
    }

    private fun hasGrant(uri: Uri) =
        contentResolver.persistedUriPermissions.any {
            it.uri == uri && it.isReadPermission
        }

    private fun renderStatus() {
        if (isDestroyed) return
        val settings = savedSettings
        val savedURI = settings?.optString("cameraURI")?.let(Uri::parse)
        val ready = savedURI != null && hasGrant(savedURI)
        val headline =
            when {
                settings == null -> "Setup needed"
                !ready -> "Camera folder permission needed"
                else -> "✓ Configured"
            }
        findViewById<TextView>(R.id.setup_status).text = headline
        findViewById<TextView>(R.id.setup_summary).text =
            if (settings == null)
                "Open Settings, scan your desktop QR, choose a camera folder, then save."
            else
                "Destination: ${settings.optString("folderID")}\nServer: ${settings.optString("discoveryURL")}\nCamera: $savedURI" +
                    if (dirty) "\nUnsaved changes — save before backing up." else ""
        val imported =
            edit(R.id.discovery_url).text.isNotBlank() &&
                edit(R.id.folder_id).text.isNotBlank() &&
                edit(R.id.discovery_token).text.isNotBlank() &&
                edit(R.id.folder_secret).text.isNotBlank()
        findViewById<TextView>(R.id.setup_steps).text =
            "${if (imported) "✓" else "1."} Connection details ${if (imported) "entered" else "needed — scan a desktop QR"}\n" +
                "${if (cameraURI != null && hasGrant(cameraURI!!)) "✓ Camera folder chosen" else "2. Choose your camera folder"}\n" +
                if (settings != null && ready && !dirty) "✓ Settings saved"
                else "3. Save & start backup"
        val running = (manualWork + periodicWork).any { it.state == WorkInfo.State.RUNNING }
        val queued = manualWork.any {
            it.state == WorkInfo.State.ENQUEUED || it.state == WorkInfo.State.BLOCKED
        }
        val periodic = periodicWork.any {
            it.state == WorkInfo.State.ENQUEUED || it.state == WorkInfo.State.BLOCKED
        }
        val scopeMatches =
            settings != null && statusPrefs.getString("scope", "") == settings.optString("scope")
        val phase = if (scopeMatches) statusPrefs.getString("phase", "") ?: "" else ""
        findViewById<TextView>(R.id.work_status).text =
            if (ready)
                backupState(running, queued, periodic, settings!!.optBoolean("automatic"), phase)
            else "Finish setup in Settings"
        findViewById<ProgressBar>(R.id.backup_progress).visibility =
            if (running) View.VISIBLE else View.GONE
        findViewById<TextView>(R.id.status).text =
            if (scopeMatches) statusPrefs.getString("message", "No backup check yet.")
            else "No backup check yet."
        findViewById<TextView>(R.id.backup_counts).text =
            if (scopeMatches)
                "${statusPrefs.getInt("saved", 0)} saved · ${statusPrefs.getInt("unchanged", 0)} already backed up · ${statusPrefs.getInt("skipped", 0)} skipped"
            else ""
        val completed = if (scopeMatches) statusPrefs.getLong("completed", 0) else 0
        findViewById<TextView>(R.id.last_backup).text =
            if (completed > 0)
                "Last completed backup check: ${DateFormat.getDateTimeInstance().format(Date(completed))}"
            else "No completed backup check yet."
        findViewById<TextView>(R.id.constraints).text =
            if (settings != null)
                "Requires ${if (settings.optBoolean("unmetered")) "unmetered network" else "network"}" +
                    (if (settings.optBoolean("charging")) ", charging" else "") +
                    ", and battery not low. Android controls scheduling."
            else ""
        for (id in listOf(R.id.backup_now, R.id.recheck)) findViewById<Button>(id).isEnabled =
            ready && !dirty && !running
        io.execute {
            val history = journal.history()
            onScreen {
                if (!isDestroyed) findViewById<TextView>(R.id.activity_log).text = history
            }
        }
    }
}
