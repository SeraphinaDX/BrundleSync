// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import android.app.Application

class DoucheSyncApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        // WorkManager may start the process without an Activity. Supply the
        // application context to gomobile for both entry points.
        go.Seq.setContext(this)
    }
}
