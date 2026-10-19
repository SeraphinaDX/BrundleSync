// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import org.junit.Assert.*
import org.junit.Test

class BackupPresentationTest {
    @Test
    fun workManagerOverridesStaleProgress() {
        assertEquals("Backing up", backupState(true, false, false, true, "uploading"))
        assertTrue(backupState(false, false, false, false, "uploading").contains("interrupted"))
    }

    @Test
    fun waitingAndAttentionAreVisible() {
        assertTrue(backupState(false, true, false, false, "retry").contains("retry"))
        assertEquals("Needs attention", backupState(false, false, true, true, "attention"))
        assertTrue(backupState(false, false, true, true, "complete").contains("next run"))
    }
}
