// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import android.content.ContentResolver
import android.net.Uri
import android.provider.DocumentsContract

data class CameraItem(val uri: Uri, val name: String, val size: Long, val modified: Long)

/**
 * Query all metadata columns together: one provider round-trip per directory, rather than several
 * lookups per photo. Content URIs stay in the Android layer.
 */
object CameraTree {
    private val columns =
        arrayOf(
            DocumentsContract.Document.COLUMN_DOCUMENT_ID,
            DocumentsContract.Document.COLUMN_DISPLAY_NAME,
            DocumentsContract.Document.COLUMN_MIME_TYPE,
            DocumentsContract.Document.COLUMN_SIZE,
            DocumentsContract.Document.COLUMN_LAST_MODIFIED,
        )

    fun scan(resolver: ContentResolver, tree: Uri, stopped: () -> Boolean): Sequence<CameraItem> =
        sequence {
            val directories = ArrayDeque<Pair<String, Int>>()
            val seen = HashSet<String>()
            directories.add(DocumentsContract.getTreeDocumentId(tree) to 0)
            var total = 0
            while (directories.isNotEmpty()) {
                if (stopped()) return@sequence
                val (id, depth) = directories.removeFirst()
                if (!seen.add(id)) continue
                require(depth <= 64) { "Camera folder is nested too deeply" }
                val children = DocumentsContract.buildChildDocumentsUriUsingTree(tree, id)
                val items = ArrayList<CameraItem>()
                val cursor =
                    resolver.query(children, columns, null, null, null)
                        ?: error("Cannot list camera folder")
                cursor.use {
                    while (it.moveToNext()) {
                        if (stopped()) return@sequence
                        require(++total <= 100_000) { "Camera folder exceeds 100,000 entries" }
                        val childID = it.getString(0)
                        val name = it.getString(1) ?: "photo"
                        val mime = it.getString(2) ?: ""
                        if (mime == DocumentsContract.Document.MIME_TYPE_DIR)
                            directories.add(childID to depth + 1)
                        else if (mime.startsWith("image/") || mime.startsWith("video/")) {
                            items.add(
                                CameraItem(
                                    DocumentsContract.buildDocumentUriUsingTree(tree, childID),
                                    name,
                                    if (it.isNull(3)) -1 else it.getLong(3),
                                    if (it.isNull(4)) 0 else it.getLong(4),
                                )
                            )
                        }
                    }
                }
                // The provider cursor is closed before any potentially long transfer.
                yieldAll(items)
            }
        }

    fun metadata(resolver: ContentResolver, item: CameraItem): Pair<Long, Long> {
        resolver
            .query(
                item.uri,
                arrayOf(
                    DocumentsContract.Document.COLUMN_SIZE,
                    DocumentsContract.Document.COLUMN_LAST_MODIFIED,
                ),
                null,
                null,
                null,
            )
            ?.use {
                if (it.moveToFirst())
                    return (if (it.isNull(0)) -1 else it.getLong(0)) to
                        (if (it.isNull(1)) 0 else it.getLong(1))
            }
        error("Camera file disappeared")
    }
}
