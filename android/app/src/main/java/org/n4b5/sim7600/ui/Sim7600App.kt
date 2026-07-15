package org.n4b5.sim7600.ui

import android.net.Uri
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Call
import androidx.compose.material.icons.filled.Dashboard
import androidx.compose.material.icons.filled.ForwardToInbox
import androidx.compose.material.icons.filled.Message
import androidx.compose.material.icons.filled.MoreHoriz
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
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
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavHostController
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import kotlinx.coroutines.launch
import org.n4b5.sim7600.AppViewModel
import org.n4b5.sim7600.BuildConfig
import org.n4b5.sim7600.notifications.EventStreamService
import org.n4b5.sim7600.ui.screens.AdminScreen
import org.n4b5.sim7600.ui.screens.CallDetailScreen
import org.n4b5.sim7600.ui.screens.CallsScreen
import org.n4b5.sim7600.ui.screens.DashboardScreen
import org.n4b5.sim7600.ui.screens.EventsScreen
import org.n4b5.sim7600.ui.screens.ForwardingScreen
import org.n4b5.sim7600.ui.screens.MessageThreadScreen
import org.n4b5.sim7600.ui.screens.MessagesScreen

private data class Destination(val route: String, val label: String, val icon: androidx.compose.ui.graphics.vector.ImageVector)

private val bottomDestinations = listOf(
    Destination("dashboard", "Home", Icons.Default.Dashboard),
    Destination("messages", "Messages", Icons.Default.Message),
    Destination("calls", "Calls", Icons.Default.Call),
    Destination("forwarding", "Forward", Icons.Default.ForwardToInbox),
    Destination("more", "More", Icons.Default.MoreHoriz),
)

@Composable
fun Sim7600App(viewModel: AppViewModel) {
    val config = viewModel.config
    if (config == null) {
        LoginScreen(viewModel)
        return
    }
    val client = remember(config) { viewModel.client!! }
    val nav = rememberNavController()
    val backStack by nav.currentBackStackEntryAsState()
    val route = backStack?.destination?.route.orEmpty()

    LaunchedEffect(viewModel.pendingRoute) {
        viewModel.pendingRoute?.let {
            nav.navigate(it)
            viewModel.pendingRoute = null
        }
    }

    Scaffold(
        bottomBar = {
            NavigationBar {
                bottomDestinations.forEach { destination ->
                    NavigationBarItem(
                        selected = route == destination.route || route.startsWith(destination.route + "/"),
                        onClick = { nav.navigateTop(destination.route) },
                        icon = { Icon(destination.icon, null) },
                        label = { Text(destination.label) },
                    )
                }
            }
        },
    ) { padding ->
        NavHost(nav, startDestination = "dashboard", modifier = Modifier.padding(padding)) {
            composable("dashboard") { DashboardScreen(client, nav) }
            composable("messages") { MessagesScreen(client, nav) }
            composable("messages/{thread}") { entry ->
                MessageThreadScreen(client, Uri.decode(entry.arguments?.getString("thread").orEmpty()), nav)
            }
            composable("calls") { CallsScreen(client, nav) }
            composable("calls/{id}") { entry ->
                val callId = entry.arguments?.getString("id").orEmpty()
                CallDetailScreen(
                    client = client,
                    callId = callId,
                    nav = nav,
                    autoConnectEnabled = viewModel.sessionStore.autoConnectCallAudio,
                    autoConnectRequested = viewModel.pendingAutoAudioCallId == callId,
                    notificationActionError = viewModel.notificationCallActionError,
                    onAutoConnectHandled = {
                        if (viewModel.pendingAutoAudioCallId == callId) viewModel.pendingAutoAudioCallId = null
                    },
                    onNotificationErrorHandled = { viewModel.notificationCallActionError = "" },
                )
            }
            composable("forwarding") { ForwardingScreen(client) }
            composable("events") { EventsScreen(client) }
            composable("admin") { AdminScreen(client) }
            composable("more") { MoreScreen(nav, viewModel) }
            composable("settings") { SettingsScreen(viewModel, nav) }
        }
    }
}

private fun NavHostController.navigateTop(route: String) {
    navigate(route) {
        popUpTo(graph.findStartDestination().id) { saveState = true }
        launchSingleTop = true
        restoreState = true
    }
}

@Composable
private fun LoginScreen(viewModel: AppViewModel) {
    var url by remember { mutableStateOf("") }
    var token by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf("") }
    val scope = rememberCoroutineScope()
    Column(
        Modifier.fillMaxSize().padding(24.dp).verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.Center,
    ) {
        Text("Connect to SIM7600", style = androidx.compose.material3.MaterialTheme.typography.headlineMedium)
        Text("Use the HTTPS Cloudflare address when the phone is away from your home network.", modifier = Modifier.padding(vertical = 12.dp))
        OutlinedTextField(url, { url = it }, label = { Text("Server URL") }, placeholder = { Text("https://phone.example.com") }, modifier = Modifier.fillMaxWidth())
        OutlinedTextField(
            token, { token = it }, label = { Text("Bearer token") },
            visualTransformation = PasswordVisualTransformation(), modifier = Modifier.fillMaxWidth().padding(top = 12.dp),
        )
        if (error.isNotBlank()) ErrorCard(error, Modifier.padding(top = 12.dp))
        Button(
            onClick = {
                scope.launch {
                    busy = true
                    error = ""
                    viewModel.connect(url, token).onFailure { error = it.message ?: "Connection failed" }
                    busy = false
                }
            },
            enabled = !busy && url.isNotBlank() && token.isNotBlank(),
            modifier = Modifier.fillMaxWidth().padding(top = 16.dp),
        ) { Text(if (busy) "Connecting…" else "Connect") }
    }
}

@Composable
private fun MoreScreen(nav: NavHostController, viewModel: AppViewModel) {
    Column(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text("More", style = androidx.compose.material3.MaterialTheme.typography.headlineSmall)
        Button(onClick = { nav.navigate("events") }, modifier = Modifier.fillMaxWidth()) { Text("Event log") }
        Button(onClick = { nav.navigate("admin") }, modifier = Modifier.fillMaxWidth()) { Text("Administration") }
        OutlinedButton(onClick = { nav.navigate("settings") }, modifier = Modifier.fillMaxWidth()) { Text("Connection & notifications") }
        SectionCard("About") {
            KeyValue("Version", BuildConfig.VERSION_NAME)
            KeyValue("Git revision", BuildConfig.GIT_REVISION)
        }
        TextButton(onClick = viewModel::logout, modifier = Modifier.fillMaxWidth()) { Text("Sign out") }
    }
}

@Composable
private fun SettingsScreen(viewModel: AppViewModel, nav: NavHostController) {
    val context = androidx.compose.ui.platform.LocalContext.current
    var enabled by remember { mutableStateOf(viewModel.sessionStore.notificationsEnabled) }
    var autoConnectAudio by remember { mutableStateOf(viewModel.sessionStore.autoConnectCallAudio) }
    Column(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
            Text("Settings", style = androidx.compose.material3.MaterialTheme.typography.headlineSmall)
            TextButton(onClick = { nav.popBackStack() }) { Text("Done") }
        }
        SectionCard("Server") { KeyValue("Address", viewModel.config?.baseUrl.orEmpty()) }
        SectionCard("Real-time notifications") {
            Text(if (enabled) "Connected in the background for incoming calls and SMS." else "Background alerts are disabled.")
            Button(onClick = {
                enabled = !enabled
                viewModel.sessionStore.notificationsEnabled = enabled
                if (enabled) EventStreamService.start(context) else EventStreamService.stop(context)
            }) { Text(if (enabled) "Disable notifications" else "Enable notifications") }
        }
        SectionCard("Call audio") {
            Text("Automatically connect the phone microphone and speaker after you answer a call.")
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                Text("Connect audio on answer")
                androidx.compose.material3.Switch(
                    checked = autoConnectAudio,
                    onCheckedChange = {
                        autoConnectAudio = it
                        viewModel.sessionStore.autoConnectCallAudio = it
                    },
                )
            }
        }
        OutlinedButton(onClick = viewModel::logout, modifier = Modifier.fillMaxWidth()) { Text("Forget server and sign out") }
    }
}
