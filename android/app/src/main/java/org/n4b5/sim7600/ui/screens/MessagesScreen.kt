package org.n4b5.sim7600.ui.screens

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.net.Uri
import android.os.Build
import android.widget.Toast
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.navigation.NavHostController
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import org.n4b5.sim7600.model.SmsMessage
import org.n4b5.sim7600.model.SmsThread
import org.n4b5.sim7600.model.formatPhone
import org.n4b5.sim7600.model.groupMessages
import org.n4b5.sim7600.model.phoneKey
import org.n4b5.sim7600.network.ApiClient
import org.n4b5.sim7600.ui.ErrorCard
import org.n4b5.sim7600.ui.SectionCard
import org.n4b5.sim7600.ui.formatTimestamp

private suspend fun loadMessages(client: ApiClient): List<SmsMessage> = coroutineScope {
    val inbound = async { client.listSms("in") }
    val outbound = async { client.listSms("out") }
    inbound.await() + outbound.await()
}

private fun copyMessageText(context: Context, text: String) {
    val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
    clipboard.setPrimaryClip(ClipData.newPlainText("Message text", text))
    if (Build.VERSION.SDK_INT <= Build.VERSION_CODES.S_V2) {
        Toast.makeText(context, "Message text copied", Toast.LENGTH_SHORT).show()
    }
}

@Composable
fun MessagesScreen(client: ApiClient, nav: NavHostController) {
    var threads by remember { mutableStateOf<List<SmsThread>>(emptyList()) }
    var search by remember { mutableStateOf("") }
    var error by remember { mutableStateOf("") }
    var compose by remember { mutableStateOf(false) }
    LaunchedEffect(client) {
        while (isActive) {
            runCatching { groupMessages(loadMessages(client)) }.onSuccess { threads = it }.onFailure { error = it.message.orEmpty() }
            delay(5_000)
        }
    }
    Box(Modifier.fillMaxSize()) {
        Column(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text("Messages", style = MaterialTheme.typography.headlineSmall)
            OutlinedTextField(search, { search = it }, label = { Text("Search conversations") }, modifier = Modifier.fillMaxWidth())
            if (error.isNotBlank()) ErrorCard(error)
            val visible = threads.filter { thread ->
                search.isBlank() || thread.number.contains(search, true) || thread.messages.any { it.body.contains(search, true) }
            }
            LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(visible, key = { it.key }) { thread ->
                    SectionCard(formatPhone(thread.number), Modifier.clickable { nav.navigate("messages/${Uri.encode(thread.key)}") }) {
                        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                            Text((if (thread.latest.inbound) "" else "You: ") + thread.latest.body, maxLines = 2, modifier = Modifier.weight(1f))
                            Text(formatTimestamp(thread.latest.timestamp), style = MaterialTheme.typography.labelSmall, modifier = Modifier.padding(start = 8.dp))
                        }
                    }
                }
            }
        }
        FloatingActionButton(onClick = { compose = true }, modifier = Modifier.align(Alignment.BottomEnd).padding(20.dp)) {
            androidx.compose.material3.Icon(Icons.Default.Add, "New message")
        }
    }
    if (compose) NewMessageDialog(client, nav, onClose = { compose = false }, onError = { error = it })
}

@Composable
private fun NewMessageDialog(client: ApiClient, nav: NavHostController, onClose: () -> Unit, onError: (String) -> Unit) {
    var to by remember { mutableStateOf("") }
    var body by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf("") }
    val scope = rememberCoroutineScope()
    AlertDialog(
        onDismissRequest = onClose,
        title = { Text("New message") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(to, { to = it }, label = { Text("Phone number") })
                OutlinedTextField(body, { body = it }, label = { Text("Message") }, minLines = 3)
                if (error.isNotBlank()) ErrorCard(error)
            }
        },
        confirmButton = {
            Button(
                onClick = {
                    scope.launch {
                        busy = true
                        error = ""
                        runCatching { client.sendSms(to, body) }.onSuccess {
                            onClose(); nav.navigate("messages/${Uri.encode(phoneKey(to).ifBlank { to })}")
                        }.onFailure {
                            error = it.message ?: "SMS send failed"
                            onError(error)
                        }
                        busy = false
                    }
                },
                enabled = !busy && to.isNotBlank() && body.isNotBlank(),
            ) { Text(if (busy) "Sending…" else "Send") }
        },
        dismissButton = { TextButton(onClick = onClose) { Text("Cancel") } },
    )
}

@Composable
fun MessageThreadScreen(client: ApiClient, threadKey: String, nav: NavHostController) {
    var messages by remember { mutableStateOf<List<SmsMessage>>(emptyList()) }
    var body by remember { mutableStateOf("") }
    var error by remember { mutableStateOf("") }
    var selected by remember { mutableStateOf<SmsMessage?>(null) }
    var busy by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    suspend fun refresh() {
        messages = loadMessages(client).filter { phoneKey(it.peer).ifBlank { it.peer } == threadKey }.sortedBy { it.timestamp }
    }
    LaunchedEffect(client, threadKey) {
        while (isActive) {
            runCatching { refresh() }.onFailure { error = it.message.orEmpty() }
            delay(3_000)
        }
    }
    val number = messages.lastOrNull()?.peer ?: threadKey
    Column(Modifier.fillMaxSize()) {
        Row(Modifier.fillMaxWidth().padding(12.dp), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
            TextButton(onClick = { nav.popBackStack() }) { Text("Back") }
            Text(formatPhone(number), style = MaterialTheme.typography.titleMedium)
            TextButton(onClick = {
                scope.launch {
                    runCatching { client.dial(number) }.onSuccess { nav.navigate("calls/${it.id}") }.onFailure { error = it.message.orEmpty() }
                }
            }) { Text("Call") }
        }
        if (error.isNotBlank()) ErrorCard(error, Modifier.padding(horizontal = 12.dp))
        LazyColumn(Modifier.weight(1f).fillMaxWidth().padding(horizontal = 12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            items(messages, key = { it.id }) { message ->
                Row(Modifier.fillMaxWidth(), horizontalArrangement = if (message.inbound) Arrangement.Start else Arrangement.End) {
                    androidx.compose.material3.Surface(
                        color = if (message.inbound) MaterialTheme.colorScheme.surfaceVariant else MaterialTheme.colorScheme.primary,
                        contentColor = if (message.inbound) MaterialTheme.colorScheme.onSurfaceVariant else Color.White,
                        shape = MaterialTheme.shapes.large,
                        modifier = Modifier.fillMaxWidth(0.82f).clickable { selected = message },
                    ) {
                        Column(Modifier.padding(12.dp)) {
                            Text(message.body)
                            Text(
                                listOf(formatTimestamp(message.timestamp), message.state).filter(String::isNotBlank).joinToString(" · "),
                                style = MaterialTheme.typography.labelSmall,
                            )
                            if (message.state == "failed" && message.errorDetail.isNotBlank()) {
                                Text(message.errorDetail, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.labelSmall)
                            }
                        }
                    }
                }
            }
        }
        Row(Modifier.fillMaxWidth().padding(12.dp), horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.Bottom) {
            OutlinedTextField(body, { body = it }, label = { Text("Text message") }, modifier = Modifier.weight(1f), maxLines = 4)
            Button(
                onClick = {
                    scope.launch {
                        busy = true
                        runCatching { client.sendSms(number, body) }.onSuccess { body = ""; refresh() }.onFailure { error = it.message.orEmpty() }
                        busy = false
                    }
                },
                enabled = !busy && body.isNotBlank(),
            ) { Text("Send") }
        }
    }
    selected?.let { message ->
        MessageDetailsDialog(
            message = message,
            onClose = { selected = null },
            onCopy = {
                copyMessageText(context, it)
                selected = null
            },
            onDelete = {
                scope.launch {
                    runCatching { client.deleteSms(message.id); refresh() }.onFailure { error = it.message.orEmpty() }
                    selected = null
                }
            },
        )
    }
}

@Composable
internal fun MessageDetailsDialog(
    message: SmsMessage,
    onClose: () -> Unit,
    onCopy: (String) -> Unit,
    onDelete: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onClose,
        title = { Text("Message details") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text("ID: ${message.id}")
                Text("Direction: ${message.direction}")
                Text("Time: ${message.timestamp}")
                Text("Encoding: ${message.encoding}; parts: ${message.parts}")
                if (message.state.isNotBlank()) Text("State: ${message.state}")
                if (message.errorDetail.isNotBlank()) Text("Error: ${message.errorDetail}")
            }
        },
        confirmButton = {
            Row {
                TextButton(onClick = { onCopy(message.body) }) {
                    Icon(Icons.Default.ContentCopy, contentDescription = null)
                    Text("Copy text", modifier = Modifier.padding(start = 8.dp))
                }
                TextButton(onClick = onClose) { Text("Close") }
            }
        },
        dismissButton = { TextButton(onClick = onDelete) { Text("Delete") } },
    )
}
