package org.n4b5.sim7600.notifications

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import org.n4b5.sim7600.data.SessionStore

class BootReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action == Intent.ACTION_BOOT_COMPLETED) {
            val store = SessionStore(context)
            if (store.notificationsEnabled && store.load() != null) EventStreamService.start(context)
        }
    }
}
