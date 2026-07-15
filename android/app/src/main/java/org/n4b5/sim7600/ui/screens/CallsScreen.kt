package org.n4b5.sim7600.ui.screens

import android.Manifest
import android.content.pm.PackageManager
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import androidx.navigation.NavHostController
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import org.n4b5.sim7600.audio.AudioState
import org.n4b5.sim7600.audio.CallAudioSession
import org.n4b5.sim7600.model.VoiceCall
import org.n4b5.sim7600.model.formatPhone
import org.n4b5.sim7600.network.ApiClient
import org.n4b5.sim7600.ui.ErrorCard
import org.n4b5.sim7600.ui.KeyValue
import org.n4b5.sim7600.ui.Loading
import org.n4b5.sim7600.ui.SectionCard
import org.n4b5.sim7600.ui.formatTimestamp

@Composable
fun CallsScreen(client: ApiClient, nav: NavHostController) {
    var calls by remember { mutableStateOf<List<VoiceCall>>(emptyList()) }
    var error by remember { mutableStateOf("") }
    var dial by remember { mutableStateOf(false) }
    LaunchedEffect(client) {
        while (isActive) {
            runCatching { client.listCalls() }.onSuccess { calls = it }.onFailure { error = it.message.orEmpty() }
            delay(2_000)
        }
    }
    Box(Modifier.fillMaxSize()) {
        Column(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text("Calls", style = MaterialTheme.typography.headlineSmall)
            if (error.isNotBlank()) ErrorCard(error)
            LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                val open = calls.filter(VoiceCall::open)
                if (open.isNotEmpty()) {
                    item { Text("Now", style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.primary) }
                    items(open, key = { it.id }) { call -> CallRow(call) { nav.navigate("calls/${call.id}") } }
                }
                item { Text("Recent", style = MaterialTheme.typography.titleSmall) }
                items(calls.filterNot(VoiceCall::open), key = { it.id }) { call -> CallRow(call) { nav.navigate("calls/${call.id}") } }
            }
        }
        FloatingActionButton(onClick = { dial = true }, modifier = Modifier.align(Alignment.BottomEnd).padding(20.dp)) {
            androidx.compose.material3.Icon(Icons.Default.Add, "Dial")
        }
    }
    if (dial) DialDialog(client, nav, onClose = { dial = false }, onError = { error = it })
}

@Composable
private fun CallRow(call: VoiceCall, onClick: () -> Unit) {
    SectionCard(formatPhone(call.peer), Modifier.clickable(onClick = onClick)) {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
            Text("${if (call.direction == "in") "Incoming" else "Outgoing"} · ${call.state}")
            Text(formatTimestamp(call.startedAt), style = MaterialTheme.typography.labelSmall)
        }
    }
}

@Composable
private fun DialDialog(client: ApiClient, nav: NavHostController, onClose: () -> Unit, onError: (String) -> Unit) {
    var number by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    AlertDialog(
        onDismissRequest = onClose,
        title = { Text("New call") },
        text = { OutlinedTextField(number, { number = it }, label = { Text("Phone number") }) },
        confirmButton = {
            Button(
                onClick = {
                    scope.launch {
                        busy = true
                        runCatching { client.dial(number) }.onSuccess { onClose(); nav.navigate("calls/${it.id}") }
                            .onFailure { onError(it.message.orEmpty()) }
                        busy = false
                    }
                },
                enabled = !busy && number.isNotBlank(),
            ) { Text(if (busy) "Calling…" else "Call") }
        },
        dismissButton = { TextButton(onClick = onClose) { Text("Cancel") } },
    )
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun CallDetailScreen(
    client: ApiClient,
    callId: String,
    nav: NavHostController,
    autoConnectEnabled: Boolean = false,
    autoConnectRequested: Boolean = false,
    notificationActionError: String = "",
    onAutoConnectHandled: () -> Unit = {},
    onNotificationErrorHandled: () -> Unit = {},
) {
    var call by remember { mutableStateOf<VoiceCall?>(null) }
    var openCalls by remember { mutableStateOf<List<VoiceCall>>(emptyList()) }
    var error by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    var connectAudioAfterAnswer by remember(callId) { mutableStateOf(autoConnectRequested) }
    val scope = rememberCoroutineScope()
    LaunchedEffect(autoConnectRequested) {
        if (autoConnectRequested) connectAudioAfterAnswer = true
    }
    LaunchedEffect(notificationActionError) {
        if (notificationActionError.isNotBlank()) {
            error = notificationActionError
            onNotificationErrorHandled()
        }
    }
    suspend fun refresh() {
        call = client.getCall(callId)
        openCalls = client.listCalls(50).filter(VoiceCall::open)
    }
    LaunchedEffect(client, callId) {
        while (isActive) {
            runCatching { refresh() }.onFailure { error = it.message.orEmpty() }
            delay(1_000)
        }
    }
    fun action(name: String) {
        scope.launch {
            busy = true; error = ""
            runCatching { client.callAction(callId, name) }.onSuccess {
                call = it
                if (name == "answer" && autoConnectEnabled) connectAudioAfterAnswer = true
            }.onFailure { error = it.message.orEmpty() }
            busy = false
        }
    }
    val current = call
    if (current == null) { Loading(); return }
    val canMerge = (current.state == "active" && openCalls.any { it.id != callId && it.state == "held" }) ||
        (current.state == "held" && openCalls.any { it.id != callId && it.state == "active" })
    LazyColumn(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        item {
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                TextButton(onClick = { nav.popBackStack() }) { Text("Back") }
                Text(current.state.uppercase(), color = MaterialTheme.colorScheme.primary)
            }
        }
        item {
            SectionCard(formatPhone(current.peer)) {
                Text(if (current.direction == "in") "Incoming call" else "Outgoing call")
                Text(formatTimestamp(current.startedAt))
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (current.state == "ringing" && current.direction == "in") {
                        Button(onClick = { action("answer") }, enabled = !busy) { Text("Answer") }
                        OutlinedButton(onClick = { action("reject") }, enabled = !busy) { Text("Reject") }
                    }
                    if (current.state == "active") Button(onClick = { action("hold") }, enabled = !busy) { Text("Hold") }
                    if (current.state == "held") Button(onClick = { action("resume") }, enabled = !busy) { Text("Resume") }
                    if (canMerge) OutlinedButton(onClick = { action("merge") }, enabled = !busy) { Text("Merge calls") }
                    if (current.open && current.state != "ringing") OutlinedButton(onClick = { action("hangup") }, enabled = !busy) { Text("Hang up") }
                    if (!current.open) Button(onClick = {
                        scope.launch { runCatching { client.dial(current.peer) }.onSuccess { nav.navigate("calls/${it.id}") }.onFailure { error = it.message.orEmpty() } }
                    }) { Text("Call back") }
                }
            }
        }
        if (error.isNotBlank()) item { ErrorCard(error) }
        if (current.state in setOf("active", "held", "dialing", "alerting")) item {
            CallAudioControls(
                client = client,
                callId = current.id,
                autoConnect = connectAudioAfterAnswer,
                onAutoConnectHandled = {
                    connectAudioAfterAnswer = false
                    onAutoConnectHandled()
                },
            )
        }
        if (current.state == "active") item { DtmfPad(client, current.id, onError = { error = it }) }
        item {
            SectionCard("Call details") {
                KeyValue("Call ID", current.id)
                KeyValue("State", current.state)
                KeyValue("Answered", formatTimestamp(current.answeredAt))
                KeyValue("Ended", formatTimestamp(current.endedAt))
                KeyValue("End reason", current.endReason)
                if (current.durationMs > 0) KeyValue("Connected", "${current.durationMs / 1000} seconds")
            }
        }
    }
}

@Composable
private fun DtmfPad(client: ApiClient, callId: String, onError: (String) -> Unit) {
    val scope = rememberCoroutineScope()
    SectionCard("Keypad") {
        listOf("123", "456", "789", "*0#").forEach { row ->
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceEvenly) {
                row.forEach { digit ->
                    OutlinedButton(onClick = { scope.launch { runCatching { client.sendDtmf(callId, digit.toString()) }.onFailure { onError(it.message.orEmpty()) } } }) {
                        Text(digit.toString(), style = MaterialTheme.typography.titleLarge)
                    }
                }
            }
        }
    }
}

@Composable
private fun CallAudioControls(
    client: ApiClient,
    callId: String,
    autoConnect: Boolean = false,
    onAutoConnectHandled: () -> Unit = {},
) {
    val context = LocalContext.current
    var state by remember { mutableStateOf(AudioState.STOPPED) }
    var message by remember { mutableStateOf("Use the phone microphone and speaker for this call.") }
    var session by remember { mutableStateOf<CallAudioSession?>(null) }
    var micMuted by remember { mutableStateOf(false) }
    var speakerMuted by remember { mutableStateOf(false) }
    fun connect() {
        session?.stop()
        session = CallAudioSession(context, client, callId) { next, text -> state = next; message = text }.also { it.start() }
    }
    val permission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { if (it) connect() else message = "Microphone permission is required for two-way audio." }
    LaunchedEffect(autoConnect, callId) {
        if (!autoConnect || state != AudioState.STOPPED) return@LaunchedEffect
        if (ContextCompat.checkSelfPermission(context, Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED) connect()
        else permission.launch(Manifest.permission.RECORD_AUDIO)
        onAutoConnectHandled()
    }
    DisposableEffect(callId) { onDispose { session?.stop() } }
    SectionCard("Call audio") {
        Text(message)
        if (state == AudioState.LIVE) {
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                Text("Mute microphone")
                Switch(micMuted, { micMuted = it; session?.microphoneMuted = it })
            }
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                Text("Mute speaker")
                Switch(speakerMuted, { speakerMuted = it; session?.speakerMuted = it })
            }
            OutlinedButton(onClick = { session?.stop(); session = null }) { Text("Disconnect audio") }
            OutlinedButton(onClick = { session?.sendTestTone() }) { Text("Send test tone") }
        } else {
            Button(onClick = {
                if (ContextCompat.checkSelfPermission(context, Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED) connect()
                else permission.launch(Manifest.permission.RECORD_AUDIO)
            }, enabled = state != AudioState.CONNECTING) { Text("Connect audio") }
        }
    }
}
