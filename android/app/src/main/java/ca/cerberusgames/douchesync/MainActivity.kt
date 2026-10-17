// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import android.Manifest
import android.content.Intent
import android.content.SharedPreferences
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.widget.Button
import android.widget.CheckBox
import android.widget.EditText
import android.widget.TextView
import androidx.activity.ComponentActivity
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.widget.doAfterTextChanged
import ca.cerberusgames.douchesync.mobile.Mobile
import java.io.File
import java.util.concurrent.Executors
import org.json.JSONObject

class MainActivity : ComponentActivity() {
    private val io = Executors.newSingleThreadExecutor()
    private var cameraURI: Uri? = null
    private var dirty = false
    private var editVersion = 0

    private fun edited() {
        dirty = true
        editVersion++
    }

    private val configFile
        get() = File(filesDir, "config.toml")

    private val statusPrefs
        get() = getSharedPreferences("backup-status", MODE_PRIVATE)

    private val statusListener = SharedPreferences.OnSharedPreferenceChangeListener { _, key ->
        if (key == "message")
            runOnUiThread { showStatus(statusPrefs.getString("message", "") ?: "") }
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
                populate(JSONObject(Mobile.settings(configFile.path)))
            } catch (e: Exception) {
                showStatus("Cannot load configuration: ${e.message}")
            }
        } else {
            edit(R.id.device_id).setText(Mobile.newDeviceID())
        }
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
            findViewById<CheckBox>(R.id.automatic).isChecked = false
            io.execute {
                try {
                    if (configFile.exists()) Mobile.setAutomatic(configFile.path, false)
                    BackupScheduler.stop(this)
                } catch (e: Exception) {
                    runOnUiThread {
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
        outState.putString("cameraURI", cameraURI?.toString())
        outState.putBoolean("dirty", dirty)
        super.onSaveInstanceState(outState)
    }

    override fun onRestoreInstanceState(state: Bundle) {
        super.onRestoreInstanceState(state)
        dirty = state.getBoolean("dirty")
    }

    override fun onResume() {
        super.onResume()
        statusPrefs.registerOnSharedPreferenceChangeListener(statusListener)
        statusPrefs.getString("message", null)?.let(::showStatus)
    }

    override fun onPause() {
        statusPrefs.unregisterOnSharedPreferenceChangeListener(statusListener)
        super.onPause()
    }

    override fun onDestroy() {
        io.shutdown()
        super.onDestroy()
    }

    private fun edit(id: Int) = findViewById<EditText>(id)

    private fun checked(id: Int) = findViewById<CheckBox>(id).isChecked

    private fun showStatus(message: String) {
        findViewById<TextView>(R.id.status).text = message
    }

    private fun populate(s: JSONObject) {
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
    }

    private fun save() {
        val uri = cameraURI ?: return showStatus(getString(R.string.no_folder))
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
                BackupScheduler.configure(this, automatic, unmetered, charging)
                runOnUiThread {
                    if (editVersion == version) dirty = false
                    showStatus(getString(R.string.saved))
                }
            } catch (e: Exception) {
                runOnUiThread { showStatus("Settings were not saved: ${e.message}") }
            } finally {
                runOnUiThread { findViewById<Button>(R.id.save).isEnabled = true }
            }
        }
    }

    private fun enqueue(recheck: Boolean) {
        if (dirty) return showStatus("Save your changed settings before starting a backup.")
        if (!configFile.exists()) return showStatus(getString(R.string.not_configured))
        if (
            Build.VERSION.SDK_INT >= 33 &&
                checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) !=
                    android.content.pm.PackageManager.PERMISSION_GRANTED
        )
            notifications.launch(Manifest.permission.POST_NOTIFICATIONS)
        io.execute {
            try {
                val s = JSONObject(Mobile.settings(configFile.path))
                BackupScheduler.now(
                    this,
                    s.getBoolean("unmetered"),
                    s.getBoolean("charging"),
                    recheck,
                )
                runOnUiThread { showStatus(getString(R.string.queued)) }
            } catch (e: Exception) {
                runOnUiThread { showStatus("Cannot queue backup: ${e.message}") }
            }
        }
    }
}
