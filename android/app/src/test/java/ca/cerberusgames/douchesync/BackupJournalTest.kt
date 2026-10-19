// SPDX-License-Identifier: GPL-3.0-or-later
package ca.cerberusgames.douchesync

import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = android.app.Application::class)
class BackupJournalTest {
    @Test
    fun boundedHistoryAndScopeIsolation() {
        val journal =
            BackupJournal.get(RuntimeEnvironment.getApplication<android.app.Application>())
        journal.configured("first")
        for (i in 1..105) journal.event("first", "Event #$i.")
        assertFalse(journal.history().contains("Event #1."))
        assertTrue(journal.history().contains("Event #105."))
        journal.report("first", "complete", "Done", 5, 0, 0)
        assertTrue(journal.prefs.getLong("completed", 0) > 0)
        journal.configured("second")
        journal.report("first", "complete", "Old worker", 99, 0, 0)
        assertEquals(0L, journal.prefs.getLong("completed", 0))
        assertEquals(0, journal.prefs.getInt("saved", 0))
        journal.clearHistory()
        assertEquals("No backup activity yet.", journal.history())
    }
}
