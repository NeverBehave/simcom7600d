package org.n4b5.sim7600.data

import android.content.Context
import org.n4b5.sim7600.model.ServerConfig

class SessionStore(context: Context) {
    private val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)

    fun load(): ServerConfig? {
        val baseUrl = prefs.getString(KEY_URL, "").orEmpty()
        val token = prefs.getString(KEY_TOKEN, "").orEmpty()
        return if (baseUrl.isBlank() || token.isBlank()) null else ServerConfig(baseUrl, token)
    }

    fun save(config: ServerConfig) {
        prefs.edit().putString(KEY_URL, config.baseUrl).putString(KEY_TOKEN, config.token).apply()
    }

    fun clear() {
        prefs.edit()
            .remove(KEY_URL)
            .remove(KEY_TOKEN)
            .remove(KEY_EVENT_ID)
            .remove(KEY_PENDING_AUTO_AUDIO_CALL)
            .remove(KEY_PENDING_CALL_ACTION_ERROR)
            .apply()
    }

    var notificationsEnabled: Boolean
        get() = prefs.getBoolean(KEY_NOTIFICATIONS, true)
        set(value) { prefs.edit().putBoolean(KEY_NOTIFICATIONS, value).apply() }

    var autoConnectCallAudio: Boolean
        get() = prefs.getBoolean(KEY_AUTO_CONNECT_AUDIO, false)
        set(value) { prefs.edit().putBoolean(KEY_AUTO_CONNECT_AUDIO, value).apply() }

    var lastEventId: Long
        get() = prefs.getLong(KEY_EVENT_ID, 0L)
        set(value) { prefs.edit().putLong(KEY_EVENT_ID, value).apply() }

    fun recordNotificationAnswer(callId: String, error: String? = null) {
        val edit = prefs.edit()
        if (error == null && autoConnectCallAudio) edit.putString(KEY_PENDING_AUTO_AUDIO_CALL, callId)
        else edit.remove(KEY_PENDING_AUTO_AUDIO_CALL)
        if (error.isNullOrBlank()) edit.remove(KEY_PENDING_CALL_ACTION_ERROR)
        else edit.putString(KEY_PENDING_CALL_ACTION_ERROR, error)
        edit.commit()
    }

    fun consumePendingAutoAudioCallId(): String = consumeString(KEY_PENDING_AUTO_AUDIO_CALL)

    fun consumePendingCallActionError(): String = consumeString(KEY_PENDING_CALL_ACTION_ERROR)

    private fun consumeString(key: String): String {
        val value = prefs.getString(key, "").orEmpty()
        if (value.isNotBlank()) prefs.edit().remove(key).commit()
        return value
    }

    companion object {
        private const val PREFS = "sim7600.session"
        private const val KEY_URL = "base_url"
        private const val KEY_TOKEN = "token"
        private const val KEY_NOTIFICATIONS = "notifications"
        private const val KEY_AUTO_CONNECT_AUDIO = "auto_connect_call_audio"
        private const val KEY_EVENT_ID = "last_event_id"
        private const val KEY_PENDING_AUTO_AUDIO_CALL = "pending_auto_audio_call"
        private const val KEY_PENDING_CALL_ACTION_ERROR = "pending_call_action_error"
    }
}
