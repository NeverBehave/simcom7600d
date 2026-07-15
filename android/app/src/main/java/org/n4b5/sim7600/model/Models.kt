package org.n4b5.sim7600.model

import org.json.JSONArray
import org.json.JSONObject

data class ServerConfig(val baseUrl: String, val token: String)

data class ModemStatus(
    val model: String,
    val imei: String,
    val firmware: String,
    val simState: String,
    val operator: String,
    val imsi: String,
    val iccid: String,
    val registered: Boolean,
    val tech: String,
    val band: String,
    val rsrpDbm: Int,
    val rsrqDb: Int,
    val csq: Int,
    val batteryV: Double,
    val uptimeSeconds: Long,
) {
    companion object {
        fun fromJson(root: JSONObject): ModemStatus {
            val modem = root.objectOrEmpty("modem")
            val sim = root.objectOrEmpty("sim")
            val network = root.objectOrEmpty("network")
            val battery = root.objectOrEmpty("battery")
            return ModemStatus(
                model = modem.string("model"), imei = modem.string("imei"), firmware = modem.string("firmware"),
                simState = sim.string("state"), operator = sim.string("operator"),
                imsi = sim.string("imsi"), iccid = sim.string("iccid"),
                registered = network.optBoolean("registered"), tech = network.string("tech"),
                band = network.string("band"), rsrpDbm = network.optInt("rsrp_dbm"),
                rsrqDb = network.optInt("rsrq_db"), csq = network.optInt("csq"),
                batteryV = battery.optDouble("voltage_v"), uptimeSeconds = root.optLong("uptime_s"),
            )
        }
    }
}

data class SmsMessage(
    val id: String,
    val direction: String,
    val peer: String,
    val body: String,
    val state: String,
    val timestamp: String,
    val encoding: String,
    val parts: Int,
    val incomplete: Boolean,
    val errorCode: String,
    val errorDetail: String,
) {
    val inbound: Boolean get() = direction == "in"

    companion object {
        fun fromJson(json: JSONObject) = SmsMessage(
            id = json.string("id"), direction = json.string("direction"),
            peer = if (json.string("direction") == "in") json.string("from") else json.string("to"),
            body = json.string("body"), state = json.string("state"),
            timestamp = if (json.string("direction") == "in") json.string("received_at") else json.string("ts"),
            encoding = json.string("encoding"),
            parts = when (val value = json.opt("parts")) {
                is JSONArray -> value.length()
                is Number -> value.toInt()
                else -> 0
            },
            incomplete = json.optBoolean("incomplete"), errorCode = json.string("error_code"),
            errorDetail = json.string("error_detail"),
        )
    }
}

data class SmsThread(val key: String, val number: String, val messages: List<SmsMessage>) {
    val latest: SmsMessage get() = messages.last()
}

fun groupMessages(messages: List<SmsMessage>): List<SmsThread> = messages
    .groupBy { phoneKey(it.peer).ifBlank { it.peer } }
    .map { (key, values) ->
        val sorted = values.sortedBy { it.timestamp }
        SmsThread(key, sorted.last().peer, sorted)
    }
    .sortedByDescending { it.latest.timestamp }

data class VoiceCall(
    val id: String,
    val direction: String,
    val peer: String,
    val state: String,
    val startedAt: String,
    val answeredAt: String,
    val endedAt: String,
    val endReason: String,
    val durationMs: Int,
) {
    val open: Boolean get() = state in setOf("ringing", "dialing", "alerting", "active", "held")

    companion object {
        fun fromJson(json: JSONObject) = VoiceCall(
            id = json.string("id"), direction = json.string("direction"),
            peer = if (json.string("direction") == "in") json.string("from") else json.string("to"),
            state = json.string("state"), startedAt = json.string("started_at"),
            answeredAt = json.string("answered_at"), endedAt = json.string("ended_at"),
            endReason = json.string("end_reason"), durationMs = json.optInt("duration_ms"),
        )
    }
}

data class ForwardingRule(
    val reason: String,
    val enabled: Boolean,
    val number: String,
    val timeoutSeconds: Int,
    val available: Boolean,
    val error: String,
) {
    companion object {
        fun fromJson(json: JSONObject) = ForwardingRule(
            reason = json.string("reason"), enabled = json.optBoolean("enabled"),
            number = json.string("number"), timeoutSeconds = json.optInt("timeout_seconds"),
            available = json.optBoolean("available"), error = json.string("error"),
        )
    }
}

data class ModemEvent(
    val id: Long,
    val timestamp: String,
    val kind: String,
    val refKind: String,
    val refId: String,
    val raw: String,
    val detail: String,
) {
    companion object {
        fun fromJson(json: JSONObject) = ModemEvent(
            id = json.optLong("id"), timestamp = json.string("ts"), kind = json.string("kind"),
            refKind = json.string("ref_kind"), refId = json.string("ref_id"),
            raw = json.string("raw"), detail = json.string("detail"),
        )
    }
}

data class AdminCapabilities(val atPassthrough: Boolean, val modemReset: Boolean)

fun phoneKey(value: String): String {
    val digits = value.filter(Char::isDigit)
    return if (digits.length == 11 && digits.startsWith("1")) digits.drop(1) else digits
}

fun formatPhone(value: String): String {
    if (value.isBlank()) return "Unknown number"
    val key = phoneKey(value)
    return if (key.length == 10) "(${key.take(3)}) ${key.substring(3, 6)}-${key.takeLast(4)}" else value
}

internal fun JSONObject.string(name: String): String = if (isNull(name)) "" else optString(name, "")
internal fun JSONObject.objectOrEmpty(name: String): JSONObject = optJSONObject(name) ?: JSONObject()
