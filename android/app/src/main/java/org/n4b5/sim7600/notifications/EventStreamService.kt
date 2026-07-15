package org.n4b5.sim7600.notifications

import android.Manifest
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import okhttp3.Call
import org.json.JSONObject
import org.n4b5.sim7600.MainActivity
import org.n4b5.sim7600.R
import org.n4b5.sim7600.data.SessionStore
import org.n4b5.sim7600.model.formatPhone
import org.n4b5.sim7600.model.phoneKey
import org.n4b5.sim7600.network.ApiClient
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.concurrent.thread

class EventStreamService : Service() {
    private val running = AtomicBoolean(false)
    private var currentCall: Call? = null
    private lateinit var store: SessionStore

    override fun onCreate() {
        super.onCreate()
        store = SessionStore(this)
        createChannels()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP || !store.notificationsEnabled || store.load() == null) {
            stopSelf()
            return START_NOT_STICKY
        }
        startForeground(CONNECTION_NOTIFICATION, connectionNotification("Connected for real-time alerts"))
        if (running.compareAndSet(false, true)) thread(name = "sim7600-events", isDaemon = true, block = ::streamLoop)
        return START_STICKY
    }

    override fun onDestroy() {
        running.set(false)
        currentCall?.cancel()
        currentCall = null
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun streamLoop() {
        while (running.get()) {
            val config = store.load() ?: break
            try {
                val call = ApiClient(config).eventStreamCall(store.lastEventId)
                currentCall = call
                call.execute().use { response ->
                    if (!response.isSuccessful) throw IllegalStateException("Event stream HTTP ${response.code}")
                    val source = response.body?.source() ?: throw IllegalStateException("Empty event stream")
                    var event = ""
                    var id = 0L
                    var data = ""
                    while (running.get() && !source.exhausted()) {
                        val line = source.readUtf8Line() ?: break
                        when {
                            line.startsWith("event:") -> event = line.substringAfter(':').trim()
                            line.startsWith("id:") -> id = line.substringAfter(':').trim().toLongOrNull() ?: id
                            line.startsWith("data:") -> data += line.substringAfter(':').trim()
                            line.isEmpty() -> {
                                if (id > store.lastEventId) store.lastEventId = id
                                if (event == "sim7600" && data.isNotBlank()) handleEvent(JSONObject(data))
                                event = ""; data = ""
                            }
                        }
                    }
                }
            } catch (error: Throwable) {
                if (running.get()) {
                    val manager = getSystemService(NotificationManager::class.java)
                    manager.notify(CONNECTION_NOTIFICATION, connectionNotification("Reconnecting: ${error.message.orEmpty()}"))
                    Thread.sleep(2_000)
                }
            } finally {
                currentCall = null
            }
        }
        stopSelf()
    }

    private fun handleEvent(event: JSONObject) {
        val kind = event.optString("kind")
        val refId = event.optString("ref_id")
        val detail = runCatching { JSONObject(event.optString("detail")) }.getOrDefault(JSONObject())
        when (kind) {
            "call.ringing" -> notifyIncomingCall(refId, detail.optString("from", event.optString("raw")))
            "call.updated", "call.ended" -> cancelCallNotification(refId)
            "sms.arrived" -> notifySms(refId, detail.optString("from"), detail.optString("body"))
        }
    }

    private fun notifyIncomingCall(callId: String, from: String) {
        val route = if (callId.isBlank()) "calls" else "calls/$callId"
        val content = activityIntent(route, notificationId(callId, CALL_NOTIFICATION_BASE))
        val answer = callActionIntent(callId, "answer", notificationId(callId, CALL_NOTIFICATION_BASE) + 1)
        val reject = callActionIntent(callId, "reject", notificationId(callId, CALL_NOTIFICATION_BASE) + 2)
        val notification = NotificationCompat.Builder(this, CHANNEL_CALLS)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle("Incoming call")
            .setContentText(formatPhone(from))
            .setCategory(NotificationCompat.CATEGORY_CALL)
            .setPriority(NotificationCompat.PRIORITY_MAX)
            .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
            .setOngoing(true)
            .setAutoCancel(false)
            .setContentIntent(content)
            .setFullScreenIntent(content, true)
            .addAction(0, "Reject", reject)
            .addAction(0, "Answer", answer)
            .build()
        notify(notificationId(callId, CALL_NOTIFICATION_BASE), notification)
    }

    private fun notifySms(messageId: String, from: String, body: String) {
        val thread = phoneKey(from).ifBlank { from }
        val route = if (thread.isBlank()) "messages" else "messages/${android.net.Uri.encode(thread)}"
        val notification = NotificationCompat.Builder(this, CHANNEL_MESSAGES)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(if (from.isBlank()) "New message" else "Message from ${formatPhone(from)}")
            .setContentText(body.ifBlank { "You received a new text message" })
            .setStyle(NotificationCompat.BigTextStyle().bigText(body))
            .setCategory(NotificationCompat.CATEGORY_MESSAGE)
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
            .setAutoCancel(true)
            .setContentIntent(activityIntent(route, notificationId(messageId, MESSAGE_NOTIFICATION_BASE)))
            .build()
        notify(notificationId(messageId, MESSAGE_NOTIFICATION_BASE), notification)
    }

    private fun cancelCallNotification(callId: String) {
        if (callId.isNotBlank()) getSystemService(NotificationManager::class.java)
            .cancel(notificationId(callId, CALL_NOTIFICATION_BASE))
    }

    private fun activityIntent(route: String, requestCode: Int): PendingIntent {
        val intent = Intent(this, MainActivity::class.java)
            .putExtra(MainActivity.EXTRA_ROUTE, route)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP)
        return PendingIntent.getActivity(this, requestCode, intent, PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
    }

    private fun callActionIntent(callId: String, action: String, requestCode: Int): PendingIntent {
        val intent = Intent(this, CallActionReceiver::class.java)
            .putExtra(CallActionReceiver.EXTRA_CALL_ID, callId)
            .putExtra(CallActionReceiver.EXTRA_ACTION, action)
        return PendingIntent.getBroadcast(this, requestCode, intent, PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
    }

    private fun connectionNotification(status: String) = NotificationCompat.Builder(this, CHANNEL_CONNECTION)
        .setSmallIcon(R.drawable.ic_notification)
        .setContentTitle("SIM7600 notifications")
        .setContentText(status)
        .setOngoing(true)
        .setSilent(true)
        .setContentIntent(activityIntent("dashboard", CONNECTION_NOTIFICATION))
        .build()

    private fun createChannels() {
        val manager = getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(NotificationChannel(CHANNEL_CONNECTION, "Background connection", NotificationManager.IMPORTANCE_LOW))
        manager.createNotificationChannel(NotificationChannel(CHANNEL_CALLS, "Incoming calls", NotificationManager.IMPORTANCE_HIGH).apply {
            description = "Real-time SIM7600 incoming call alerts"
            lockscreenVisibility = android.app.Notification.VISIBILITY_PRIVATE
        })
        manager.createNotificationChannel(NotificationChannel(CHANNEL_MESSAGES, "Messages", NotificationManager.IMPORTANCE_HIGH).apply {
            lockscreenVisibility = android.app.Notification.VISIBILITY_PRIVATE
        })
    }

    private fun notify(id: Int, notification: android.app.Notification) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) return
        runCatching { NotificationManagerCompat.from(this).notify(id, notification) }
    }

    private fun notificationId(value: String, base: Int) = base + (value.hashCode() and 0x0fffffff) % 100_000

    companion object {
        const val CHANNEL_CONNECTION = "sim7600.connection"
        const val CHANNEL_CALLS = "sim7600.calls"
        const val CHANNEL_MESSAGES = "sim7600.messages"
        const val ACTION_STOP = "org.n4b5.sim7600.STOP_EVENTS"
        const val CALL_NOTIFICATION_BASE = 10_000
        private const val MESSAGE_NOTIFICATION_BASE = 200_000
        private const val CONNECTION_NOTIFICATION = 7

        fun start(context: Context) {
            val store = SessionStore(context)
            if (!store.notificationsEnabled || store.load() == null) return
            ContextCompat.startForegroundService(context, Intent(context, EventStreamService::class.java))
        }

        fun stop(context: Context) {
            context.stopService(Intent(context, EventStreamService::class.java))
        }
    }
}
