// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import android.app.Application
import android.net.Uri
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class BackupIndexTest {
    @Test
    fun receiptsAreScopedToSourceDestinationAndFileVersion() {
        BackupIndex(RuntimeEnvironment.getApplication()).use { index ->
            val item = CameraItem(Uri.parse("content://camera/photo"), "photo.jpg", 42, 1234)
            assertFalse(index.contains("destination-a", item, "run-1", false))
            index.replace("destination-a", item, "verified-receipt", "run-1")
            assertTrue(index.contains("destination-a", item, "run-2", false))
            assertFalse(index.contains("destination-b", item, "run-2", false))
            assertFalse(index.contains("destination-a", item.copy(size = 43), "run-2", false))
            assertFalse(index.contains("destination-a", item.copy(modified = 9999), "run-2", false))
            assertFalse(index.contains("destination-a", item, "run-2", true))
            index.replace("destination-a", item, "verified-again", "run-2")
            assertTrue(index.contains("destination-a", item, "run-2", true))
        }
    }

    @Test
    fun unknownMetadataCanOnlyResumeTheSameInterruptedJob() {
        BackupIndex(RuntimeEnvironment.getApplication()).use { index ->
            val item = CameraItem(Uri.parse("content://camera/unknown"), "photo.jpg", -1, 0)
            index.replace("target", item, "receipt", "run-1")
            assertTrue(index.contains("target", item, "run-1", false))
            assertFalse(index.contains("target", item, "run-2", false))
        }
    }
}
