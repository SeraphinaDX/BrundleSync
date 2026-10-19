// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import android.content.Intent
import android.database.ContentObserver
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import org.golang.app.GoNativeActivity

/** The visible app is Fyne. No gomobile engine library is loaded in :ui. */
class FyneActivity : GoNativeActivity() {
    private external fun uiChanged()

    private val handler = Handler(Looper.getMainLooper())
    private var pending = false
    private var observing = false
    private val refresh = Runnable {
        pending = false
        if (observing) uiChanged()
    }
    private val observer =
        object : ContentObserver(handler) {
            override fun onChange(selfChange: Boolean) {
                if (!pending) {
                    pending = true
                    handler.postDelayed(refresh, 250)
                }
            }
        }

    override fun onResume() {
        super.onResume()
        observing = true
        contentResolver.registerContentObserver(UiProvider.uri(packageName), false, observer)
        uiChanged()
    }

    override fun onPause() {
        observing = false
        contentResolver.unregisterContentObserver(observer)
        handler.removeCallbacks(refresh)
        pending = false
        super.onPause()
    }

    // Called by RunNative on an attached Go thread. Byte arrays retain standard
    // UTF-8, including emoji in camera file names and non-ASCII credentials.
    fun bridgeRequest(action: String, payload: ByteArray): ByteArray {
        val result =
            contentResolver.call(
                UiProvider.uri(packageName),
                action,
                null,
                Bundle().apply { putString("payload", payload.toString(Charsets.UTF_8)) },
            )
        return (result?.getString("json") ?: "{\"error\":\"Android bridge unavailable\"}")
            .toByteArray(Charsets.UTF_8)
    }

    fun bridgeLaunch(action: String) {
        require(action in setOf("scan", "folder", "notifications"))
        runOnUiThread {
            startActivity(Intent(this, SetupActivity::class.java).putExtra("action", action))
        }
    }
}
