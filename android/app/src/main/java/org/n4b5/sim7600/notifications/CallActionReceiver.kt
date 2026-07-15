package org.n4b5.sim7600.notifications

import android.app.NotificationManager
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import kotlinx.coroutines.runBlocking
import org.n4b5.sim7600.MainActivity
import org.n4b5.sim7600.data.SessionStore
import org.n4b5.sim7600.network.ApiClient
import kotlin.concurrent.thread

class CallActionReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val pending = goAsync()
        val callId = intent.getStringExtra(EXTRA_CALL_ID).orEmpty()
        val action = intent.getStringExtra(EXTRA_ACTION).orEmpty()
        thread(name = "sim7600-call-action") {
            try {
                val store = SessionStore(context)
                val config = store.load() ?: return@thread
                if (callId.isNotBlank() && action in setOf("answer", "reject", "hangup")) {
                    val error = runCatching {
                        runBlocking { ApiClient(config).callAction(callId, action) }
                    }.exceptionOrNull()
                    if (error == null) {
                        val id = EventStreamService.CALL_NOTIFICATION_BASE + (callId.hashCode() and 0x0fffffff) % 100_000
                        context.getSystemService(NotificationManager::class.java).cancel(id)
                    }
                    if (action == "answer") {
                        store.recordNotificationAnswer(callId, error?.message ?: error?.javaClass?.simpleName)
                        context.startActivity(
                            Intent(context, MainActivity::class.java)
                                .putExtra(MainActivity.EXTRA_ROUTE, "calls/$callId")
                                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP),
                        )
                    }
                }
            } finally {
                pending.finish()
            }
        }
    }

    companion object {
        const val EXTRA_CALL_ID = "call_id"
        const val EXTRA_ACTION = "call_action"
    }
}
