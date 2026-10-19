// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import android.content.Intent
import android.os.Bundle
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import ca.cerberusgames.douchesync.mobile.Mobile
import java.io.File
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Assume.assumeFalse
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class FyneUiTest {
    private val context
        get() = InstrumentationRegistry.getInstrumentation().targetContext

    private fun request(action: String, payload: JSONObject? = null): JSONObject =
        JSONObject(
            context.contentResolver
                .call(
                    UiProvider.uri(context.packageName),
                    action,
                    null,
                    Bundle().apply { putString("payload", payload?.toString() ?: "null") },
                )!!
                .getString("json")!!
        )

    @Test
    fun existingTomlSurvivesTheUiMigrationAndInvalidSave() {
        val config = File(context.filesDir, "config.toml")
        assumeFalse(config.exists())
        try {
            Mobile.configure(
                config.path,
                "existing-phone",
                "https://sync.example.com",
                "t".repeat(32),
                "camera",
                "s".repeat(32),
                "content://missing/tree/camera",
                false,
                true,
                false,
            )
            val original = config.readText()
            val snapshot = request("snapshot")
            assertTrue(snapshot.getBoolean("configured"))
            assertFalse(snapshot.getBoolean("ready"))
            assertEquals("existing-phone", snapshot.getJSONObject("settings").getString("deviceID"))
            assertTrue(request("save", snapshot.getJSONObject("settings")).has("error"))
            assertEquals(original, config.readText())
        } finally {
            config.delete()
        }
    }

    @Test
    fun launchesTheRealFyneNativeActivity() {
        val device = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
        context.startActivity(
            Intent(context, FyneActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        )
        assertTrue(
            "Fyne window did not open",
            device.wait(Until.hasObject(By.pkg(context.packageName)), 15000),
        )
        // Fyne exposes virtual accessibility nodes. Wait for its actual setup
        // content, rather than accepting Android's transient starting window.
        assertTrue(
            "Fyne setup did not render",
            device.wait(Until.hasObject(By.text("DoucheSync")), 15000),
        )
        device.pressHome()
    }
}
