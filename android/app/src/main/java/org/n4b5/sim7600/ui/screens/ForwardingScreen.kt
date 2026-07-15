package org.n4b5.sim7600.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import org.n4b5.sim7600.model.ForwardingRule
import org.n4b5.sim7600.network.ApiClient
import org.n4b5.sim7600.ui.ErrorCard
import org.n4b5.sim7600.ui.SectionCard
import org.n4b5.sim7600.ui.friendlyReason

@Composable
fun ForwardingScreen(client: ApiClient) {
    var rules by remember { mutableStateOf<List<ForwardingRule>>(emptyList()) }
    var error by remember { mutableStateOf("") }
    var loaded by remember(client) { mutableStateOf(false) }
    var loading by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    fun refresh() {
        scope.launch {
            loaded = true
            loading = true
            runCatching { client.forwarding() }.onSuccess { rules = it; error = "" }.onFailure { error = it.message.orEmpty() }
            loading = false
        }
    }
    val unconditional = rules.firstOrNull { it.reason == "unconditional" }?.enabled == true
    Column(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
            Column { Text("Call forwarding", style = MaterialTheme.typography.headlineSmall); Text("Carrier-managed voice rules") }
            Button(onClick = ::refresh, enabled = !loading) { Text(if (loaded) "Refresh" else "Load status") }
        }
        if (!loaded) {
            SectionCard("Load carrier status when needed") {
                Text("Reading forwarding rules temporarily occupies the modem command channel. Status is not queried until you ask for it.")
                Button(onClick = ::refresh) { Text("Load forwarding status") }
            }
        }
        if (error.isNotBlank()) ErrorCard(error)
        if (unconditional) Text("Always forwarding is enabled. Other forwarding rules are locked until it is disabled.", color = MaterialTheme.colorScheme.primary)
        if (loaded) LazyColumn(verticalArrangement = Arrangement.spacedBy(10.dp)) {
            items(rules, key = { it.reason }) { rule ->
                ForwardingRuleCard(
                    client = client,
                    rule = rule,
                    disabled = loading || (unconditional && rule.reason != "unconditional"),
                    onUpdated = ::refresh,
                    onError = { error = it },
                )
            }
        }
    }
}

@Composable
private fun ForwardingRuleCard(
    client: ApiClient,
    rule: ForwardingRule,
    disabled: Boolean,
    onUpdated: () -> Unit,
    onError: (String) -> Unit,
) {
    var enabled by remember(rule) { mutableStateOf(rule.enabled) }
    var number by remember(rule) { mutableStateOf(rule.number) }
    var timeout by remember(rule) { mutableStateOf((rule.timeoutSeconds.takeIf { it > 0 } ?: 20).toString()) }
    var saving by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    SectionCard(friendlyReason(rule.reason)) {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
            Text(if (rule.available) "Network status confirmed" else rule.error.ifBlank { "Status unavailable" })
            Switch(checked = enabled, onCheckedChange = { enabled = it }, enabled = !disabled && !saving)
        }
        OutlinedTextField(number, { number = it }, label = { Text("Forward to") }, enabled = enabled && !disabled, modifier = Modifier.fillMaxWidth())
        if (rule.reason == "no_reply") {
            OutlinedTextField(timeout, { timeout = it.filter(Char::isDigit) }, label = { Text("Ring seconds (5–30)") }, enabled = enabled && !disabled, modifier = Modifier.fillMaxWidth())
        }
        Button(
            onClick = {
                scope.launch {
                    saving = true
                    runCatching { client.updateForwarding(rule.reason, enabled, number, timeout.toIntOrNull() ?: 20) }
                        .onSuccess { onUpdated() }.onFailure { onError(it.message.orEmpty()) }
                    saving = false
                }
            },
            enabled = !disabled && !saving && (!enabled || number.isNotBlank()),
        ) { Text(if (saving) "Saving…" else "Apply") }
    }
}
