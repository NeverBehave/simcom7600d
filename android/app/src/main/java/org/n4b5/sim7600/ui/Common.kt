package org.n4b5.sim7600.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

@Composable
fun Loading(modifier: Modifier = Modifier) {
    Row(modifier.fillMaxWidth().padding(32.dp), horizontalArrangement = Arrangement.Center) {
        CircularProgressIndicator()
    }
}

@Composable
fun ErrorCard(message: String, modifier: Modifier = Modifier) {
    Card(modifier.fillMaxWidth(), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.errorContainer)) {
        Text(message, modifier = Modifier.padding(16.dp), color = MaterialTheme.colorScheme.onErrorContainer)
    }
}

@Composable
fun SectionCard(title: String, modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    Card(modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Text(title, style = MaterialTheme.typography.titleMedium)
            content()
        }
    }
}

@Composable
fun KeyValue(label: String, value: String) {
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.Top) {
        Text(label, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Text(value.ifBlank { "—" }, modifier = Modifier.padding(start = 16.dp), style = MaterialTheme.typography.bodyMedium)
    }
}

fun friendlyReason(reason: String): String = reason.replace('_', ' ').replaceFirstChar { it.uppercase() }

fun formatTimestamp(value: String): String = runCatching {
    DateTimeFormatter.ofPattern("MMM d, h:mm a").withZone(ZoneId.systemDefault()).format(Instant.parse(value))
}.getOrDefault(value)
