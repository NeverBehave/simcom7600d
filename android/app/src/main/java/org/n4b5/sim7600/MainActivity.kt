package org.n4b5.sim7600

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.graphics.Color
import androidx.core.content.ContextCompat
import org.n4b5.sim7600.notifications.EventStreamService
import org.n4b5.sim7600.ui.Sim7600App

class MainActivity : ComponentActivity() {
    private val viewModel: AppViewModel by viewModels()
    private val notificationPermission = registerForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        if (granted && viewModel.config != null) EventStreamService.start(this)
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        handleIntent(intent)
        requestNotificationPermission()
        setContent {
            MaterialTheme(
                colorScheme = lightColorScheme(
                    primary = Color(0xFF2563EB),
                    secondary = Color(0xFF0F766E),
                    error = Color(0xFFB91C1C),
                ),
            ) {
                LaunchedEffect(viewModel.config) {
                    if (viewModel.config != null && canNotify()) EventStreamService.start(this@MainActivity)
                    else if (viewModel.config == null) EventStreamService.stop(this@MainActivity)
                }
                Sim7600App(viewModel)
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        handleIntent(intent)
    }

    private fun handleIntent(intent: Intent?) {
        intent?.getStringExtra(EXTRA_ROUTE)?.let { viewModel.pendingRoute = it }
        viewModel.sessionStore.consumePendingAutoAudioCallId().takeIf { it.isNotBlank() }
            ?.let { viewModel.pendingAutoAudioCallId = it }
        viewModel.notificationCallActionError = viewModel.sessionStore.consumePendingCallActionError()
    }

    private fun requestNotificationPermission() {
        if (Build.VERSION.SDK_INT >= 33 && !canNotify()) notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
    }

    private fun canNotify(): Boolean = Build.VERSION.SDK_INT < 33 ||
        ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED

    companion object {
        const val EXTRA_ROUTE = "route"
    }
}
