package org.n4b5.sim7600.ui.screens

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import org.n4b5.sim7600.model.ModemEvent
import org.n4b5.sim7600.network.ApiClient
import org.n4b5.sim7600.ui.ErrorCard
import org.n4b5.sim7600.ui.SectionCard
import org.n4b5.sim7600.ui.formatTimestamp

@Composable
fun EventsScreen(client: ApiClient) {
    val filters = listOf("" to "All", "sms." to "SMS", "call." to "Calls", "modem." to "Modem", "net." to "Network")
    var filter by remember { mutableStateOf("") }
    var events by remember { mutableStateOf<List<ModemEvent>>(emptyList()) }
    var error by remember { mutableStateOf("") }
    var selected by remember { mutableStateOf<ModemEvent?>(null) }
    LaunchedEffect(client) {
        while (isActive) {
            runCatching { events = client.events(limit = 500) }.onFailure { error = it.message.orEmpty() }
            delay(2_500)
        }
    }
    Column(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text("Event log", style = MaterialTheme.typography.headlineSmall)
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            filters.forEach { (value, label) -> FilterChip(selected = filter == value, onClick = { filter = value }, label = { Text(label) }) }
        }
        if (error.isNotBlank()) ErrorCard(error)
        selected?.let { event ->
            SectionCard(event.kind) {
                Text(formatTimestamp(event.timestamp))
                if (event.raw.isNotBlank()) Text(event.raw)
                if (event.detail.isNotBlank()) Text(event.detail)
                Text("Tap again to close", style = MaterialTheme.typography.labelSmall)
            }
        }
        LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            items(events.filter { filter.isBlank() || it.kind.startsWith(filter) }, key = { it.id }) { event ->
                SectionCard(event.kind, Modifier.clickable { selected = if (selected?.id == event.id) null else event }) {
                    Text(formatTimestamp(event.timestamp), color = MaterialTheme.colorScheme.onSurfaceVariant)
                    if (event.refId.isNotBlank()) Text("${event.refKind}: ${event.refId}")
                }
            }
        }
    }
}
