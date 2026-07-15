package org.n4b5.sim7600.network

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.Call
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.json.JSONArray
import org.json.JSONObject
import org.n4b5.sim7600.model.AdminCapabilities
import org.n4b5.sim7600.model.ForwardingRule
import org.n4b5.sim7600.model.ModemEvent
import org.n4b5.sim7600.model.ModemStatus
import org.n4b5.sim7600.model.ServerConfig
import org.n4b5.sim7600.model.SmsMessage
import org.n4b5.sim7600.model.VoiceCall
import java.io.IOException
import java.net.URLEncoder
import java.nio.charset.StandardCharsets
import java.util.UUID
import java.util.concurrent.TimeUnit

class ApiException(
    val status: Int,
    val method: String,
    val route: String,
    val host: String,
    val cfRay: String? = null,
    val serverDetail: String? = null,
    message: String,
) : IOException(message)

class ApiClient(val config: ServerConfig) {
    private val jsonType = "application/json; charset=utf-8".toMediaType()
    private val client = OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(100, TimeUnit.SECONDS)
        .build()
    // Sending can legitimately wait behind another modem command and each SMS
    // segment can take up to 30 seconds. Do not let the app abandon the origin
    // at exactly 100 seconds; that produced Cloudflare's incomplete-response
    // page even though the web client continued waiting.
    private val smsClient = client.newBuilder().readTimeout(5, TimeUnit.MINUTES).build()
    private val streamClient = client.newBuilder().readTimeout(0, TimeUnit.MILLISECONDS).build()

    suspend fun status(refresh: Boolean = false): ModemStatus =
        ModemStatus.fromJson(requestJson("GET", "/v1/status?refresh=${if (refresh) 1 else 0}"))

    suspend fun listSms(direction: String, limit: Int = 500): List<SmsMessage> =
        requestJson("GET", "/v1/sms?direction=${encode(direction)}&limit=$limit")
            .array("items").objects().map(SmsMessage::fromJson)

    suspend fun getSms(id: String): SmsMessage = SmsMessage.fromJson(requestJson("GET", "/v1/sms/${encode(id)}"))

    suspend fun sendSms(to: String, body: String): SmsMessage = SmsMessage.fromJson(
        requestJson(
            "POST", "/v1/sms",
            // Match the proven web request. Forcing the status-report bit makes
            // this SIM7600/T-Mobile path intermittently reject otherwise valid
            // messages with the firmware-specific `+CMS ERROR: 0` response.
            JSONObject().put("to", to).put("body", body),
            idempotencyKey = UUID.randomUUID().toString(),
        ),
    )

    suspend fun deleteSms(id: String) { requestJson("DELETE", "/v1/sms/${encode(id)}") }

    suspend fun listCalls(limit: Int = 200): List<VoiceCall> =
        requestJson("GET", "/v1/calls?limit=$limit").array("items").objects().map(VoiceCall::fromJson)

    suspend fun getCall(id: String): VoiceCall = VoiceCall.fromJson(requestJson("GET", "/v1/calls/${encode(id)}"))

    suspend fun dial(to: String): VoiceCall = VoiceCall.fromJson(
        requestJson(
            "POST", "/v1/calls", JSONObject().put("to", to),
            idempotencyKey = UUID.randomUUID().toString(),
        ),
    )

    suspend fun callAction(id: String, action: String): VoiceCall = VoiceCall.fromJson(
        requestJson("POST", "/v1/calls/${encode(id)}/${encode(action)}", JSONObject()),
    )

    suspend fun sendDtmf(id: String, digits: String, durationMs: Int = 200) {
        requestJson(
            "POST", "/v1/calls/${encode(id)}/dtmf",
            JSONObject().put("digits", digits).put("duration_ms", durationMs),
        )
    }

    suspend fun forwarding(): List<ForwardingRule> =
        requestJson("GET", "/v1/call-forwarding").array("items").objects().map(ForwardingRule::fromJson)

    suspend fun updateForwarding(
        reason: String,
        enabled: Boolean,
        number: String,
        timeoutSeconds: Int,
    ): ForwardingRule {
        val body = JSONObject().put("enabled", enabled)
        if (enabled) body.put("number", number)
        if (reason == "no_reply") body.put("timeout_seconds", timeoutSeconds)
        return ForwardingRule.fromJson(requestJson("PUT", "/v1/call-forwarding/${encode(reason)}", body))
    }

    suspend fun events(since: Long = 0, kind: String = "", limit: Int = 200): List<ModemEvent> {
        val query = buildString {
            append("?since=$since&limit=$limit")
            if (kind.isNotBlank()) append("&kind=${encode(kind)}")
        }
        return requestJson("GET", "/v1/events$query").array("items").objects().map(ModemEvent::fromJson)
    }

    suspend fun adminCapabilities(): AdminCapabilities {
        val json = requestJson("GET", "/v1/admin/capabilities")
        return AdminCapabilities(json.optBoolean("at_passthrough"), json.optBoolean("modem_reset"))
    }

    suspend fun adminQueue(): String = requestJson("GET", "/v1/admin/queue").toString(2)
    suspend fun reconcile() { requestJson("POST", "/v1/admin/reconcile", JSONObject()) }
    suspend fun vacuum() { requestJson("POST", "/v1/admin/vacuum", JSONObject()) }
    suspend fun resetModem() { requestJson("POST", "/v1/admin/at-reset", JSONObject()) }
    suspend fun sendAt(command: String, timeoutMs: Int = 5000): JSONObject =
        requestJson("POST", "/v1/admin/at", JSONObject().put("cmd", command).put("timeout_ms", timeoutMs))

    fun eventStreamCall(since: Long): Call = streamClient.newCall(
        requestBuilder("/v1/events/stream?since=$since").get().build(),
    )

    fun openAudioWebSocket(callId: String, listener: WebSocketListener): WebSocket {
        val httpUrl = absoluteUrl("/v1/calls/${encode(callId)}/audio")
        val wsUrl = when {
            httpUrl.startsWith("https://") -> "wss://${httpUrl.removePrefix("https://")}"
            httpUrl.startsWith("http://") -> "ws://${httpUrl.removePrefix("http://")}"
            else -> httpUrl
        }
        val request = Request.Builder()
            .url(wsUrl)
            .header("Sec-WebSocket-Protocol", "sim7600.audio.v1, sim7600.token.${config.token}")
            .build()
        return streamClient.newWebSocket(request, listener)
    }

    private suspend fun requestJson(
        method: String,
        path: String,
        body: JSONObject? = null,
        idempotencyKey: String? = null,
    ): JSONObject = withContext(Dispatchers.IO) {
        val requestBody = body?.toString()?.toRequestBody(jsonType)
        val builder = requestBuilder(path)
        when (method) {
            "GET" -> builder.get()
            "POST" -> builder.post(requestBody ?: ByteArray(0).toRequestBody(null))
            "PUT" -> builder.put(requestBody ?: ByteArray(0).toRequestBody(null))
            "DELETE" -> builder.delete(requestBody)
            else -> error("unsupported method $method")
        }
        if (idempotencyKey != null) builder.header("Idempotency-Key", idempotencyKey)
        execute(builder.build()).use(::decodeJson)
    }

    private fun requestBuilder(path: String): Request.Builder = Request.Builder()
        .url(absoluteUrl(path))
        .header("Authorization", "Bearer ${config.token}")
        .header("Accept", "application/json")

    private fun absoluteUrl(path: String) = config.baseUrl.trimEnd('/') + "/" + path.trimStart('/')
    private fun execute(request: Request): Response {
        val requestClient = if (request.method == "POST" && request.url.encodedPath == "/v1/sms") smsClient else client
        return try {
            requestClient.newCall(request).execute()
        } catch (error: IOException) {
            val route = request.url.encodedPath
            val operation = "${request.method} $route"
            val smsGuidance = if (request.method == "POST" && route == "/v1/sms") {
                " The send may have reached the modem; refresh the conversation before sending it again."
            } else ""
            throw ApiException(
                status = 0,
                method = request.method,
                route = route,
                host = request.url.host,
                message = "Network error contacting ${request.url.host} for $operation: " +
                    "${error.message ?: error.javaClass.simpleName}.$smsGuidance",
            )
        }
    }

    private fun decodeJson(response: Response): JSONObject {
        val text = response.body?.string().orEmpty()
        if (!response.isSuccessful) {
            val serverDetail = runCatching {
                val root = JSONObject(text)
                root.optString("detail").ifBlank {
                    root.optJSONObject("error")?.optString("message").orEmpty()
                }
            }.getOrDefault("").takeIf { it.isNotBlank() }
            val request = response.request
            val route = request.url.encodedPath
            val operation = "${request.method} $route"
            val cfRay = response.header("CF-Ray")
            val diagnostic = buildList {
                add("HTTP ${response.code}")
                add(operation)
                add(request.url.host)
                if (!cfRay.isNullOrBlank()) add("CF-Ray $cfRay")
            }.joinToString(" · ")
            val isCloudflareGatewayPage = response.code in 500..599 &&
                !cfRay.isNullOrBlank() && serverDetail == null
            val mayHaveSent = request.method == "POST" && route == "/v1/sms"
            val message = when {
                isCloudflareGatewayPage -> buildString {
                    append("Cloudflare could not get a complete response from the SIM7600 server. $diagnostic.")
                    if (mayHaveSent) append(" The request may have reached the modem; refresh the conversation before sending it again.")
                }
                response.code == 401 -> "Authentication failed. Check the server token. $diagnostic."
                serverDetail != null -> "SIM7600 server error: $serverDetail ($diagnostic)."
                else -> "SIM7600 request failed. $diagnostic."
            }
            throw ApiException(
                status = response.code,
                method = request.method,
                route = route,
                host = request.url.host,
                cfRay = cfRay,
                serverDetail = serverDetail,
                message = message,
            )
        }
        return if (text.isBlank()) JSONObject() else runCatching { JSONObject(text) }.getOrElse { error ->
            val request = response.request
            throw ApiException(
                status = response.code,
                method = request.method,
                route = request.url.encodedPath,
                host = request.url.host,
                cfRay = response.header("CF-Ray"),
                message = "The SIM7600 server returned an incomplete JSON response for ${request.method} " +
                    "${request.url.encodedPath}: ${error.message ?: "invalid JSON"}.",
            )
        }
    }

    private fun encode(value: String): String = URLEncoder.encode(value, StandardCharsets.UTF_8.name())
}

private fun JSONObject.array(name: String): JSONArray = optJSONArray(name) ?: JSONArray()
private fun JSONArray.objects(): List<JSONObject> = buildList {
    for (index in 0 until length()) optJSONObject(index)?.let(::add)
}
