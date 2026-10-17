// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import android.app.Application
import android.content.ContentProvider
import android.content.ContentValues
import android.database.Cursor
import android.database.MatrixCursor
import android.net.Uri
import android.provider.DocumentsContract
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.shadows.ShadowContentResolver

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class CameraTreeTest {
    class Photos : ContentProvider() {
        var queries = 0

        override fun onCreate() = true

        override fun query(
            uri: Uri,
            projection: Array<out String>?,
            selection: String?,
            selectionArgs: Array<out String>?,
            sortOrder: String?,
        ): Cursor {
            queries++
            val fields = projection ?: emptyArray()
            val cursor = MatrixCursor(fields)
            fun row(id: String, name: String, mime: String, size: Long = 0) {
                val data: Map<String,Any> =
                    mapOf(
                        DocumentsContract.Document.COLUMN_DOCUMENT_ID to id,
                        DocumentsContract.Document.COLUMN_DISPLAY_NAME to name,
                        DocumentsContract.Document.COLUMN_MIME_TYPE to mime,
                        DocumentsContract.Document.COLUMN_SIZE to size,
                        DocumentsContract.Document.COLUMN_LAST_MODIFIED to 1234L,
                    )
                cursor.addRow(fields.map { data[it] }.toTypedArray())
            }
            if (DocumentsContract.getDocumentId(uri) == "root") {
                row("photo", "IMG_001.jpg", "image/jpeg", 42)
                row("sub", "Videos", DocumentsContract.Document.MIME_TYPE_DIR)
                row("notes", "notes.txt", "text/plain", 10)
            } else {
                row("video", "clip.mp4", "video/mp4", 99)
                row("root", "cycle", DocumentsContract.Document.MIME_TYPE_DIR)
            }
            return cursor
        }

        override fun getType(uri: Uri): String? = null

        override fun insert(uri: Uri, values: ContentValues?): Uri? = null

        override fun delete(uri: Uri, selection: String?, selectionArgs: Array<out String>?): Int =
            0

        override fun update(
            uri: Uri,
            values: ContentValues?,
            selection: String?,
            selectionArgs: Array<out String>?,
        ): Int = 0
    }

    @Test
    fun selectsMediaRecursivelyWithOneMetadataQueryPerDirectory() {
        val provider = Photos()
        ShadowContentResolver.registerProviderInternal("camera.test", provider)
        val items =
            CameraTree.scan(
                    RuntimeEnvironment.getApplication().contentResolver,
                    Uri.parse("content://camera.test/tree/root"),
                ) {
                    false
                }
                .toList()
        assertEquals(listOf("IMG_001.jpg", "clip.mp4"), items.map { it.name })
        assertEquals(2, provider.queries)
        assertEquals(42, items[0].size)
        assertEquals("content", items[0].uri.scheme)
    }

    @Test
    fun cancelledScanDoesNotReadTheProvider() {
        val provider = Photos()
        ShadowContentResolver.registerProviderInternal("camera.test", provider)
        val items =
            CameraTree.scan(
                    RuntimeEnvironment.getApplication().contentResolver,
                    Uri.parse("content://camera.test/tree/root"),
                ) {
                    true
                }
                .toList()
        assertTrue(items.isEmpty())
        assertEquals(0, provider.queries)
    }
}
