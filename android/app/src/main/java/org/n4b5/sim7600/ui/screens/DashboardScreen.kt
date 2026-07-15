package org.n4b5.sim7600.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.navigation.NavHostController
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import org.n4b5.sim7600.model.ModemEvent
import org.n4b5.sim7600.model.ModemStatus
import org.n4b5.sim7600.network.ApiClient
import org.n4b5.sim7600.ui.ErrorCard
import org.n4b5.sim7600.ui.KeyValue
import org.n4b5.sim7600.ui.Loading
import org.n4b5.sim7600.ui.SectionCard
import org.n4b5.sim7600.ui.formatTimestamp

@Composable
fun DashboardScreen(client: ApiClient, nav: NavHostController) {
    var status by remember { mutableStateOf<ModemStatus?>(null) }
    var events by remember { mutableStateOf<List<ModemEvent>>(emptyList()) }
    var error by remember { mutableStateOf("") }
    LaunchedEffect(client) {
        while (isActive) {
            runCatching {
                status = client.status()
                events = client.events(limit = 20)
            }.onFailure { error = it.message.orEmpty() }
            delay(5_000)
        }
    }
    Column(
        Modifier.fillMaxSize().padding(16.dp).verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text("SIM7600", style = MaterialTheme.typography.headlineSmall)
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(onClick = { nav.navigate("calls") }, modifier = Modifier.weight(1f)) { Text("Dial") }
            OutlinedButton(onClick = { nav.navigate("messages") }, modifier = Modifier.weight(1f)) { Text("Message") }
        }
        if (error.isNotBlank()) ErrorCard(error)
        val snapshot = status
        if (snapshot == null) Loading() else {
            SectionCard("Network") {
                KeyValue("Registration", if (snapshot.registered) "Registered" else "Not registered")
                KeyValue("Technology", snapshot.tech)
                KeyValue("Band", snapshot.band)
                KeyValue("Signal", "${snapshot.rsrpDbm} dBm / ${snapshot.rsrqDb} dB")
                KeyValue("CSQ", snapshot.csq.toString())
            }
            SectionCard("SIM") {
                KeyValue("State", snapshot.simState)
                KeyValue("Operator", snapshot.operator)
                KeyValue("IMSI", snapshot.imsi)
                KeyValue("ICCID", snapshot.iccid)
            }
            SectionCard("Modem") {
                KeyValue("Model", snapshot.model)
                KeyValue("IMEI", snapshot.imei)
                KeyValue("Firmware", snapshot.firmware)
                KeyValue("Battery", "${snapshot.batteryV} V")
                KeyValue("Uptime", "${snapshot.uptimeSeconds} s")
            }
        }
        SectionCard("Recent events") {
            if (events.isEmpty()) Text("No events yet")
            events.take(20).forEach { event -> KeyValue(event.kind, formatTimestamp(event.timestamp)) }
        }
    }
}
