package org.n4b5.sim7600.ui.screens

import android.media.AudioAttributes
import android.media.MediaPlayer
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
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
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.MarkEmailRead
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.Phone
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Voicemail
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Slider
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.navigation.NavHostController
import java.io.File
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import org.n4b5.sim7600.model.Voicemail
import org.n4b5.sim7600.model.formatPhone
import org.n4b5.sim7600.network.ApiClient
import org.n4b5.sim7600.ui.ErrorCard
import org.n4b5.sim7600.ui.Loading
import org.n4b5.sim7600.ui.SectionCard
import org.n4b5.sim7600.ui.formatTimestamp

@Composable
fun VoicemailScreen(client: ApiClient, nav: NavHostController) {
    var items by remember { mutableStateOf<List<Voicemail>>(emptyList()) }
    var loading by remember { mutableStateOf(true) }
    var error by remember { mutableStateOf("") }
    var syncing by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    LaunchedEffect(client) {
        while (isActive) {
            runCatching { client.listVoicemails() }
                .onSuccess { items = it; error = "" }
                .onFailure { error = it.message.orEmpty() }
            loading = false
            delay(15_000)
        }
    }
    Column(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
            Column {
                Text("Voicemail", style = MaterialTheme.typography.headlineSmall)
                Text(
                    items.count { !it.read }.let { if (it == 0) "All caught up" else "$it unheard" },
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            IconButton(onClick = {
                scope.launch {
                    syncing = true
                    runCatching { client.syncVoicemails(); client.listVoicemails() }
                        .onSuccess { items = it; error = "" }
                        .onFailure { error = it.message.orEmpty() }
                    syncing = false
                }
            }, enabled = !syncing) {
                Icon(Icons.Default.Refresh, contentDescription = "Sync voicemail")
            }
        }
        if (error.isNotBlank()) ErrorCard(error)
        if (loading) Loading()
        else if (items.isEmpty()) Text("No voicemail", color = MaterialTheme.colorScheme.onSurfaceVariant)
        else LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            items(items, key = { it.id }) { item ->
                SectionCard(formatPhone(item.from), Modifier.clickable { nav.navigate("voicemail/${item.id}") }) {
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                        Column(Modifier.weight(1f)) {
                            Text(formatTimestamp(item.receivedAt), style = MaterialTheme.typography.bodySmall)
                            Text(formatVoicemailDuration(item.durationMs), style = MaterialTheme.typography.labelSmall)
                        }
                        if (!item.read) Text("New", color = MaterialTheme.colorScheme.primary, style = MaterialTheme.typography.labelMedium)
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun VoicemailDetailScreen(client: ApiClient, voicemailId: String, nav: NavHostController) {
    var item by remember { mutableStateOf<Voicemail?>(null) }
    var error by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    suspend fun refresh() { item = client.getVoicemail(voicemailId) }
    LaunchedEffect(client, voicemailId) {
        runCatching { refresh() }.onFailure { error = it.message.orEmpty() }
    }
    val current = item
    if (current == null) {
        if (error.isNotBlank()) ErrorCard(error, Modifier.padding(16.dp)) else Loading()
        return
    }
    LazyColumn(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        item {
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                TextButton(onClick = { nav.popBackStack() }) { Text("Back") }
                if (!current.read) Text("New", color = MaterialTheme.colorScheme.primary, style = MaterialTheme.typography.labelMedium)
            }
        }
        item {
            Text(formatPhone(current.from), style = MaterialTheme.typography.headlineSmall)
            Text("${formatTimestamp(current.receivedAt)} · ${formatVoicemailDuration(current.durationMs)}", color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        item {
            VoicemailPlayer(client, current) {
                if (!current.read) scope.launch { runCatching { client.setVoicemailRead(current.id, true) }.onSuccess { item = it } }
            }
        }
        if (current.transcript.isNotBlank()) item {
            SectionCard("Transcript") { Text(current.transcript) }
        }
        item {
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = {
                    scope.launch {
                        busy = true
                        runCatching { client.dial(current.from) }.onSuccess { nav.navigate("calls/${it.id}") }.onFailure { error = it.message.orEmpty() }
                        busy = false
                    }
                }, enabled = !busy && current.from.isNotBlank()) { Icon(Icons.Default.Phone, null); Text("Call back", Modifier.padding(start = 8.dp)) }
                OutlinedButton(onClick = {
                    scope.launch {
                        busy = true
                        runCatching { client.setVoicemailRead(current.id, !current.read) }.onSuccess { item = it }.onFailure { error = it.message.orEmpty() }
                        busy = false
                    }
                }, enabled = !busy) {
                    Icon(Icons.Default.MarkEmailRead, null)
                    Text(if (current.read) "Mark unheard" else "Mark heard", Modifier.padding(start = 8.dp))
                }
                OutlinedButton(onClick = {
                    scope.launch {
                        busy = true
                        runCatching { client.deleteVoicemail(current.id) }.onSuccess { nav.popBackStack() }.onFailure { error = it.message.orEmpty() }
                        busy = false
                    }
                }, enabled = !busy) { Icon(Icons.Default.Delete, null); Text("Delete", Modifier.padding(start = 8.dp)) }
            }
        }
        if (error.isNotBlank()) item { ErrorCard(error) }
    }
}

@Composable
private fun VoicemailPlayer(client: ApiClient, voicemail: Voicemail, onPlay: () -> Unit) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var player by remember(voicemail.id) { mutableStateOf<MediaPlayer?>(null) }
    var file by remember(voicemail.id) { mutableStateOf<File?>(null) }
    var loading by remember(voicemail.id) { mutableStateOf(false) }
    var playing by remember(voicemail.id) { mutableStateOf(false) }
    var position by remember(voicemail.id) { mutableIntStateOf(0) }
    var duration by remember(voicemail.id) { mutableIntStateOf(voicemail.durationMs.coerceAtLeast(1)) }
    var error by remember(voicemail.id) { mutableStateOf("") }

    fun toggle() {
        val ready = player
        if (ready != null) {
            if (ready.isPlaying) { ready.pause(); playing = false }
            else { ready.start(); playing = true; onPlay() }
            return
        }
        scope.launch {
            loading = true
            error = ""
            runCatching {
                val audio = File.createTempFile("voicemail-${voicemail.id}-", ".audio", context.cacheDir)
                client.downloadVoicemailAudio(voicemail.id, audio)
                val next = MediaPlayer().apply {
                    setAudioAttributes(AudioAttributes.Builder().setContentType(AudioAttributes.CONTENT_TYPE_SPEECH).setUsage(AudioAttributes.USAGE_MEDIA).build())
                    setDataSource(audio.absolutePath)
                    prepare()
                    setOnCompletionListener { playing = false; position = duration }
                    start()
                }
                file = audio
                player = next
                duration = next.duration.coerceAtLeast(1)
                playing = true
                onPlay()
            }.onFailure { error = it.message ?: "Could not play voicemail" }
            loading = false
        }
    }

    LaunchedEffect(playing) {
        while (playing) {
            position = runCatching { player?.currentPosition ?: 0 }.getOrDefault(0)
            delay(250)
        }
    }
    DisposableEffect(voicemail.id) {
        onDispose { player?.release(); file?.delete() }
    }

    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            IconButton(onClick = ::toggle, enabled = !loading) {
                Icon(if (playing) Icons.Default.Pause else Icons.Default.PlayArrow, if (playing) "Pause" else "Play")
            }
            Slider(
                value = position.coerceIn(0, duration).toFloat(),
                onValueChange = { next -> position = next.toInt(); player?.seekTo(position) },
                valueRange = 0f..duration.toFloat(),
                enabled = player != null,
                modifier = Modifier.weight(1f),
            )
            Text("${formatVoicemailDuration(position)} / ${formatVoicemailDuration(duration)}", style = MaterialTheme.typography.labelSmall)
        }
        if (loading) Text("Preparing audio…", style = MaterialTheme.typography.bodySmall)
        if (error.isNotBlank()) Text(error, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
    }
}

internal fun formatVoicemailDuration(durationMs: Int): String {
    val total = (durationMs.coerceAtLeast(0) + 500) / 1000
    return "${total / 60}:${(total % 60).toString().padStart(2, '0')}"
}
