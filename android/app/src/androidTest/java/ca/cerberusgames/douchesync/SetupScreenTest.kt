// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import android.app.AlertDialog
import android.content.Intent
import android.graphics.Bitmap
import android.os.Build
import android.util.Base64
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import ca.cerberusgames.douchesync.mobile.Mobile
import java.io.File
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Assume.assumeFalse
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class SetupScreenTest {
    private val context
        get() = InstrumentationRegistry.getInstrumentation().targetContext

    private fun screenshot(name: String, scenario: ActivityScenario<MainActivity>) {
        // Main-thread idleness alone can precede the first rendered frame.
        val frame = CountDownLatch(1)
        scenario.onActivity { activity ->
            val view = activity.window.decorView
            if (Build.VERSION.SDK_INT >= 29 && view.isHardwareAccelerated) {
                view.viewTreeObserver.registerFrameCommitCallback { frame.countDown() }
                view.invalidate()
            } else {
                view.postOnAnimation { view.postOnAnimation { frame.countDown() } }
            }
        }
        assertTrue("UI frame did not render", frame.await(5, TimeUnit.SECONDS))
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        instrumentation.waitForIdleSync()
        val bitmap = instrumentation.uiAutomation.takeScreenshot() ?: error("No screenshot")
        val dir = File(context.cacheDir, "ui-previews").apply { mkdirs() }
        File(dir, name).outputStream().use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }
        bitmap.recycle()
    }

    @Test
    fun qrImportRequiresReviewAndDoesNotSaveOrReplacePhoneIdentity() {
        val config = File(context.filesDir, "config.toml")
        assumeFalse(config.exists())
        val payload =
            JSONObject()
                .put("discoveryURL", "https://sync.example.com")
                .put("discoveryToken", "t".repeat(32))
                .put("folderID", "camera")
                .put("secret", "s".repeat(32))
        val code =
            "DOUCHESYNC-CAMERA-1:" +
                Base64.encodeToString(
                    payload.toString().toByteArray(),
                    Base64.URL_SAFE or Base64.NO_WRAP or Base64.NO_PADDING,
                )
        ActivityScenario.launch<MainActivity>(Intent(context, MainActivity::class.java)).use {
            scenario ->
            var phone = ""
            scenario.onActivity { activity ->
                phone = activity.findViewById<EditText>(R.id.device_id).text.toString()
                val dialog = activity.reviewSetup(code)!!
                assertEquals("", activity.findViewById<EditText>(R.id.folder_id).text.toString())
                dialog.getButton(AlertDialog.BUTTON_POSITIVE).performClick()
            }
            InstrumentationRegistry.getInstrumentation().waitForIdleSync()
            scenario.onActivity { activity ->
                assertEquals(
                    "camera",
                    activity.findViewById<EditText>(R.id.folder_id).text.toString(),
                )
                assertEquals(phone, activity.findViewById<EditText>(R.id.device_id).text.toString())
                assertFalse(config.exists())
                assertFalse(activity.findViewById<Button>(R.id.backup_now).isEnabled)
            }
            screenshot("android-setup.png", scenario)
            scenario.onActivity { activity ->
                assertNull(activity.reviewSetup("not a setup code"))
                assertEquals(
                    "camera",
                    activity.findViewById<EditText>(R.id.folder_id).text.toString(),
                )
            }
        }
    }

    @Test
    fun missingCameraGrantIsVisible() {
        val config = File(context.filesDir, "config.toml")
        assumeFalse(config.exists())
        try {
            Mobile.configure(
                config.path,
                "phone-ui-test",
                "https://sync.example.com",
                "t".repeat(32),
                "camera",
                "s".repeat(32),
                "content://missing/tree/camera",
                false,
                true,
                false,
            )
            ActivityScenario.launch<MainActivity>(Intent(context, MainActivity::class.java)).use {
                scenario ->
                scenario.onActivity { activity ->
                    assertEquals(
                        "Camera folder permission needed",
                        activity.findViewById<TextView>(R.id.setup_status).text.toString(),
                    )
                    assertFalse(activity.findViewById<Button>(R.id.backup_now).isEnabled)
                }
                screenshot("android-status.png", scenario)
            }
        } finally {
            config.delete()
        }
    }
}
