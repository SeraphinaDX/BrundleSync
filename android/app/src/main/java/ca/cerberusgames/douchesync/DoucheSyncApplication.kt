// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import android.app.Application

class DoucheSyncApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        // WorkManager may start the process without an Activity. Supply the
        // application context to gomobile for both entry points.
        val name =
            if (android.os.Build.VERSION.SDK_INT >= 28) getProcessName()
            else {
                (getSystemService(ACTIVITY_SERVICE) as android.app.ActivityManager)
                    .runningAppProcesses
                    ?.firstOrNull { it.pid == android.os.Process.myPid() }
                    ?.processName
            }
        if (name == packageName) go.Seq.setContext(this)
    }
}
