package org.n4b5.sim7600

import android.app.Application
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.AndroidViewModel
import org.n4b5.sim7600.data.SessionStore
import org.n4b5.sim7600.model.ServerConfig
import org.n4b5.sim7600.network.ApiClient

class AppViewModel(application: Application) : AndroidViewModel(application) {
    val sessionStore = SessionStore(application)

    var config by mutableStateOf(sessionStore.load())
        private set
    var pendingRoute by mutableStateOf<String?>(null)
    var pendingAutoAudioCallId by mutableStateOf<String?>(null)
    var notificationCallActionError by mutableStateOf("")

    val client: ApiClient? get() = config?.let(::ApiClient)

    suspend fun connect(baseUrlInput: String, token: String): Result<Unit> = runCatching {
        val baseUrl = normalizeUrl(baseUrlInput)
        require(token.isNotBlank()) { "Bearer token is required" }
        val candidate = ServerConfig(baseUrl, token.trim())
        ApiClient(candidate).status(refresh = true)
        sessionStore.save(candidate)
        config = candidate
    }

    fun logout() {
        sessionStore.clear()
        config = null
        pendingAutoAudioCallId = null
        notificationCallActionError = ""
    }

    private fun normalizeUrl(value: String): String {
        val trimmed = value.trim().trimEnd('/')
        require(trimmed.isNotBlank()) { "Server URL is required" }
        return if (trimmed.startsWith("http://") || trimmed.startsWith("https://")) trimmed else "https://$trimmed"
    }
}
