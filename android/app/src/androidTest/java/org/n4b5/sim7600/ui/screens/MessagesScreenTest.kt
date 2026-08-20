package org.n4b5.sim7600.ui.screens

import androidx.compose.material3.MaterialTheme
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.n4b5.sim7600.model.SmsMessage

class MessagesScreenTest {
    @get:Rule
    val composeRule = createComposeRule()

    @Test
    fun copyTextCopiesOnlyTheMessageBody() {
        val message = SmsMessage(
            id = "sms-1",
            direction = "in",
            peer = "+15551234567",
            body = "Text to copy",
            state = "received",
            timestamp = "2026-08-19T12:00:00Z",
            encoding = "GSM-7",
            parts = 1,
            incomplete = false,
            errorCode = "",
            errorDetail = "",
        )
        var copiedText = ""

        composeRule.setContent {
            MaterialTheme {
                MessageDetailsDialog(
                    message = message,
                    onClose = {},
                    onCopy = { copiedText = it },
                    onDelete = {},
                )
            }
        }

        composeRule.onNodeWithText("Copy text").performClick()

        assertEquals(message.body, copiedText)
    }
}
