// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.result.contract.ActivityResultContracts
import ca.cerberusgames.douchesync.mobile.Mobile
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanOptions
import org.json.JSONObject

/** Short-lived Android permission/intent host; all application screens are Fyne. */
class SetupActivity : ComponentActivity() {
    private fun deliver(result: JSONObject) {
        getSharedPreferences("ui-results", 0).edit().putString("result", result.toString()).commit()
        contentResolver.notifyChange(UiProvider.uri(packageName), null)
        finish()
    }

    private val scanner =
        registerForActivityResult(ScanContract()) { result ->
            if (result.contents == null) finish()
            else {
                try {
                    deliver(
                        JSONObject().put("setup", JSONObject(Mobile.decodeSetup(result.contents)))
                    )
                } catch (_: Exception) {
                    deliver(
                        JSONObject()
                            .put(
                                "notice",
                                "This is not a valid DoucheSync camera setup QR. Settings were not changed.",
                            )
                    )
                }
            }
        }
    private val picker =
        registerForActivityResult(ActivityResultContracts.OpenDocumentTree()) { uri ->
            if (uri == null) finish()
            else {
                try {
                    contentResolver.takePersistableUriPermission(
                        uri,
                        Intent.FLAG_GRANT_READ_URI_PERMISSION,
                    )
                    deliver(JSONObject().put("cameraURI", uri.toString()))
                } catch (_: SecurityException) {
                    deliver(
                        JSONObject()
                            .put(
                                "notice",
                                "Folder read permission could not be retained. Choose the folder again.",
                            )
                    )
                }
            }
        }
    private val notifications =
        registerForActivityResult(ActivityResultContracts.RequestPermission()) { finish() }

    override fun onCreate(state: Bundle?) {
        super.onCreate(state)
        if (state != null) return // ActivityResultRegistry restores an in-flight request.
        when (intent.getStringExtra("action")) {
            "scan" ->
                scanner.launch(
                    ScanOptions()
                        .setDesiredBarcodeFormats(ScanOptions.QR_CODE)
                        .setPrompt("Scan your DoucheSync desktop setup QR")
                        .setBeepEnabled(false)
                        .setOrientationLocked(false)
                )
            "folder" -> picker.launch(null)
            "notifications" ->
                if (
                    Build.VERSION.SDK_INT >= 33 &&
                        checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) !=
                            PackageManager.PERMISSION_GRANTED
                )
                    notifications.launch(Manifest.permission.POST_NOTIFICATIONS)
                else finish()
            else -> finish()
        }
    }
}
