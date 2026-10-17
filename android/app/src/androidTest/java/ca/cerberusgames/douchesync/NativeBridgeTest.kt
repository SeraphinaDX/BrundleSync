// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import android.os.ParcelFileDescriptor
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import ca.cerberusgames.douchesync.mobile.Mobile
import java.io.File
import java.security.MessageDigest
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class NativeBridgeTest {
    @Test
    fun uploadsADescriptorDirectlyToAnAuthenticatedDesktopPeer() {
        val args = InstrumentationRegistry.getArguments()
        val discovery = args.getString("discovery")
        assumeTrue(
            "Run scripts/android-integration.py for the peer transfer test",
            discovery != null,
        )
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val root = File(context.cacheDir, "native-peer-test").apply { mkdirs() }
        try {
            val outgoing = File(root, "outgoing").apply { mkdirs() }
            val config = File(root, "config.toml")
            config.writeText(
                """
                [client]
                device_id = "phone-native-test"
                listen = "127.0.0.1:0"
                identity_dir = "${File(root,"identity").path}"
                discovery_url = "$discovery"
                discovery_token = "${args.getString("token")}"
                allow_http_discovery = true
                transfer_timeout = "30s"
                [[folders]]
                id = "camera"
                path = "${outgoing.path}"
                secret = "${args.getString("secret")}"
                [android]
                camera_uri = "content://camera/tree/native-test"
            """
                    .trimIndent()
            )
            val bytes = ByteArray(2 * 1024 * 1024 + 23) { (it % 251).toByte() }
            val photo = File(root, "native-photo.jpg").apply { writeBytes(bytes) }
            val session = Mobile.newSession(config.path)
            try {
                val receipt =
                    ParcelFileDescriptor.open(photo, ParcelFileDescriptor.MODE_READ_ONLY).use {
                        JSONObject(session.uploadDescriptor(it.detachFd().toLong(), photo.name, 0))
                    }
                val hash =
                    MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") {
                        (it.toInt() and 255).toString(16).padStart(2, '0')
                    }
                assertEquals(hash, receipt.getString("hash"))
                assertEquals(bytes.size.toLong(), receipt.getLong("size"))
                assertTrue(receipt.getBoolean("complete"))
                assertTrue("camera source was deleted", photo.exists())
            } finally {
                session.cancel()
            }
        } finally {
            root.deleteRecursively()
        }
    }

    @Test
    fun loadsGoCoreAndCancelsARealNativeSession() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val root = File(context.cacheDir, "native-test").apply { mkdirs() }
        try {
            assertEquals(
                context.packageManager.getPackageInfo(context.packageName, 0).versionName,
                Mobile.version(),
            )
            val config = File(root, "config.toml")
            Mobile.configure(
                config.path,
                Mobile.newDeviceID(),
                "https://sync.example.com",
                "t".repeat(32),
                "camera",
                "s".repeat(32),
                "content://camera/tree/root",
                false,
                true,
                false,
            )
            val session = Mobile.newSession(config.path)
            assertEquals(64, session.scope().length)
            assertEquals("content://camera/tree/root", session.cameraURI())
            session.cancel()
            val photo = File(root, "photo.jpg").apply { writeText("photo") }
            try {
                session.upload(photo.path, "photo.jpg", 0)
                fail("Cancelled Go session acknowledged a file")
            } catch (_: Exception) {}
        } finally {
            root.deleteRecursively()
        }
    }
}
