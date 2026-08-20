package org.n4b5.sim7600

import org.junit.Assert.assertEquals
import org.junit.Test
import org.n4b5.sim7600.model.SmsMessage
import org.n4b5.sim7600.model.formatPhone
import org.n4b5.sim7600.model.groupMessages
import org.n4b5.sim7600.model.phoneKey
import org.n4b5.sim7600.ui.screens.formatVoicemailDuration

class ModelsTest {
    @Test fun phoneNumbersNormalizeForConversationGrouping() {
        assertEquals("2025550123", phoneKey("+1 (202) 555-0123"))
        assertEquals("(202) 555-0123", formatPhone("+12025550123"))
    }

    @Test fun messagesAreGroupedAndSortedChronologically() {
        val newer = message("2", "out", "12025550123", "later", "2026-07-14T02:00:00Z")
        val older = message("1", "in", "+12025550123", "earlier", "2026-07-14T01:00:00Z")
        val thread = groupMessages(listOf(newer, older)).single()
        assertEquals("2025550123", thread.key)
        assertEquals(listOf("1", "2"), thread.messages.map { it.id })
        assertEquals("later", thread.latest.body)
    }

    @Test fun voicemailDurationUsesMediaTimeFormat() {
        assertEquals("0:00", formatVoicemailDuration(0))
        assertEquals("1:05", formatVoicemailDuration(65_000))
    }

    private fun message(id: String, direction: String, peer: String, body: String, timestamp: String) = SmsMessage(
        id, direction, peer, body, "submitted", timestamp, "gsm7", 1, false, "", "",
    )
}
