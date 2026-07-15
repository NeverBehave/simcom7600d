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
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
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
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import org.n4b5.sim7600.model.AdminCapabilities
import org.n4b5.sim7600.network.ApiClient
import org.n4b5.sim7600.ui.ErrorCard
import org.n4b5.sim7600.ui.SectionCard

@Composable
fun AdminScreen(client: ApiClient) {
    var capabilities by remember { mutableStateOf(AdminCapabilities(false, false)) }
    var queue by remember { mutableStateOf("—") }
    var command by remember { mutableStateOf("AT") }
    var output by remember { mutableStateOf("") }
    var error by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    var confirmReset by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    LaunchedEffect(client) {
        runCatching { capabilities = client.adminCapabilities() }
        while (isActive) {
            runCatching { queue = client.adminQueue() }
            delay(4_000)
        }
    }
    fun run(label: String, operation: suspend () -> Unit) {
        scope.launch {
            busy = true; error = ""
            runCatching { operation() }.onSuccess { output = "$label complete" }.onFailure { error = it.message.orEmpty() }
            busy = false
        }
    }
    Column(
        Modifier.fillMaxSize().padding(16.dp).verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text("Administration", style = MaterialTheme.typography.headlineSmall)
        if (error.isNotBlank()) ErrorCard(error)
        SectionCard("Maintenance") {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = { run("Reconcile") { client.reconcile() } }, enabled = !busy) { Text("Reconcile") }
                OutlinedButton(onClick = { run("VACUUM") { client.vacuum() } }, enabled = !busy) { Text("VACUUM") }
            }
            Button(onClick = { confirmReset = true }, enabled = !busy && capabilities.modemReset) { Text("Reset modem") }
            if (!capabilities.modemReset) Text("Modem reset is disabled on this server.")
        }
        SectionCard("AT console") {
            OutlinedTextField(command, { command = it }, label = { Text("AT command") }, modifier = Modifier.fillMaxWidth(), enabled = capabilities.atPassthrough)
            Button(
                onClick = {
                    scope.launch {
                        busy = true; error = ""
                        runCatching { client.sendAt(command).toString(2) }.onSuccess { output = it }.onFailure { error = it.message.orEmpty() }
                        busy = false
                    }
                },
                enabled = !busy && capabilities.atPassthrough && command.isNotBlank(),
            ) { Text("Send") }
            if (!capabilities.atPassthrough) Text("AT passthrough is disabled on this server.")
            if (output.isNotBlank()) Text(output, style = MaterialTheme.typography.bodySmall)
        }
        SectionCard("Executor queue") { Text(queue, style = MaterialTheme.typography.bodySmall) }
    }
    if (confirmReset) {
        AlertDialog(
            onDismissRequest = { confirmReset = false },
            title = { Text("Reset modem?") },
            text = { Text("This interrupts active calls and SMS currently in flight.") },
            confirmButton = {
                Button(onClick = { confirmReset = false; run("Modem reset") { client.resetModem() } }) { Text("Reset") }
            },
            dismissButton = { TextButton(onClick = { confirmReset = false }) { Text("Cancel") } },
        )
    }
}
