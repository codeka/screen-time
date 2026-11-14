package com.codeka.screentime.ui

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.ContextCompat
import com.codeka.screentime.api.ApiClient
import com.codeka.screentime.data.DevicePrefs
import com.codeka.screentime.service.ScreenTimeService
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {

    private lateinit var prefs: DevicePrefs
    private lateinit var api: ApiClient
    private var hasOverlayPermissionState = mutableStateOf(false)
    private var isPairedState = mutableStateOf(false)

    private val notificationPermissionLauncher = registerForActivityResult(
        ActivityResultContracts.RequestPermission()
    ) { _ -> }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        prefs = DevicePrefs(this)
        api = ApiClient(prefs)
        hasOverlayPermissionState.value = Settings.canDrawOverlays(this)
        isPairedState.value = prefs.isPaired

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS)
                != PackageManager.PERMISSION_GRANTED
            ) {
                notificationPermissionLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
            }
        }

        setContent {
            val hasOverlayPermission by hasOverlayPermissionState
            var isPaired by isPairedState

            LaunchedEffect(Unit) {
                if (hasOverlayPermission && isPaired) {
                    startLockService()
                }
            }

            ScreenTimeTheme {
                Surface(
                    modifier = Modifier.fillMaxSize(),
                    color = Color(0xFF0F172A)
                ) {
                    if (!hasOverlayPermission) {
                        OverlayPermissionScreen(
                            onGrantClicked = {
                                val intent = Intent(
                                    Settings.ACTION_MANAGE_OVERLAY_PERMISSION,
                                    Uri.parse("package:$packageName")
                                )
                                startActivity(intent)
                            }
                        )
                    } else if (!isPaired) {
                        PairingScreen(
                            prefs = prefs,
                            api = api,
                            onPairSuccess = {
                                isPaired = true
                                startLockService()
                            }
                        )
                    } else {
                        AlreadyPairedScreen(
                            prefs = prefs,
                            onUnpair = {
                                prefs.clear()
                                isPaired = false
                                stopService(Intent(this, ScreenTimeService::class.java))
                            }
                        )
                    }
                }
            }
        }
    }

    override fun onResume() {
        super.onResume()
        val canDraw = Settings.canDrawOverlays(this)
        hasOverlayPermissionState.value = canDraw
        isPairedState.value = prefs.isPaired
        if (canDraw && prefs.isPaired) {
            startLockService()
        }
    }

    private fun startLockService() {
        val intent = Intent(this, ScreenTimeService::class.java)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            startForegroundService(intent)
        } else {
            startService(intent)
        }
    }
}

@Composable
fun OverlayPermissionScreen(onGrantClicked: () -> Unit) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center
    ) {
        Text(text = "🛡️", fontSize = 48.sp)
        Spacer(modifier = Modifier.height(16.dp))
        Text(
            text = "Permission Required",
            fontSize = 22.sp,
            fontWeight = FontWeight.Bold,
            color = Color.White
        )
        Spacer(modifier = Modifier.height(8.dp))
        Text(
            text = "Screen Time needs the 'Display over other apps' permission to show the lock screen overlay.",
            fontSize = 14.sp,
            color = Color(0xFF94A3B8),
            textAlign = TextAlign.Center
        )
        Spacer(modifier = Modifier.height(28.dp))
        Button(
            onClick = onGrantClicked,
            colors = ButtonDefaults.buttonColors(containerColor = Color(0xFF4F46E5)),
            shape = RoundedCornerShape(8.dp),
            modifier = Modifier.fillMaxWidth().height(48.dp)
        ) {
            Text("Grant Overlay Permission", fontSize = 15.sp, fontWeight = FontWeight.SemiBold)
        }
    }
}

@Composable
fun PairingScreen(
    prefs: DevicePrefs,
    api: ApiClient,
    onPairSuccess: () -> Unit
) {
    val coroutineScope = rememberCoroutineScope()
    var code by remember { mutableStateOf("") }
    var devName by remember { mutableStateOf(Build.MODEL ?: "Android Phone") }
    var serverUrl by remember { mutableStateOf(prefs.serverUrl) }
    var loading by remember { mutableStateOf(false) }
    var errorMsg by remember { mutableStateOf("") }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center
    ) {
        Text(text = "⏱️", fontSize = 44.sp)
        Spacer(modifier = Modifier.height(8.dp))
        Text(
            text = "Pair This Phone",
            fontSize = 24.sp,
            fontWeight = FontWeight.Bold,
            color = Color.White
        )
        Spacer(modifier = Modifier.height(6.dp))
        Text(
            text = "Enter the 6-character code from the parent portal.",
            fontSize = 13.sp,
            color = Color(0xFF94A3B8),
            textAlign = TextAlign.Center
        )

        if (errorMsg.isNotEmpty()) {
            Spacer(modifier = Modifier.height(12.dp))
            Surface(
                color = Color(0xFF7F1D1D),
                shape = RoundedCornerShape(6.dp),
                modifier = Modifier.fillMaxWidth()
            ) {
                Text(
                    text = errorMsg,
                    color = Color(0xFFFECACA),
                    fontSize = 13.sp,
                    modifier = Modifier.padding(10.dp),
                    textAlign = TextAlign.Center
                )
            }
        }

        Spacer(modifier = Modifier.height(20.dp))

        OutlinedTextField(
            value = code,
            onValueChange = { if (it.length <= 6) code = it.uppercase() },
            label = { Text("Pairing Code") },
            placeholder = { Text("e.g. KT79X2") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
            colors = OutlinedTextFieldDefaults.colors(
                focusedBorderColor = Color(0xFF6366F1),
                unfocusedBorderColor = Color(0xFF475569),
                focusedTextColor = Color.White,
                unfocusedTextColor = Color.White
            )
        )

        Spacer(modifier = Modifier.height(12.dp))

        OutlinedTextField(
            value = devName,
            onValueChange = { devName = it },
            label = { Text("Device Name") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
            colors = OutlinedTextFieldDefaults.colors(
                focusedBorderColor = Color(0xFF6366F1),
                unfocusedBorderColor = Color(0xFF475569),
                focusedTextColor = Color.White,
                unfocusedTextColor = Color.White
            )
        )

        Spacer(modifier = Modifier.height(12.dp))

        OutlinedTextField(
            value = serverUrl,
            onValueChange = { serverUrl = it },
            label = { Text("Server URL") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
            colors = OutlinedTextFieldDefaults.colors(
                focusedBorderColor = Color(0xFF6366F1),
                unfocusedBorderColor = Color(0xFF475569),
                focusedTextColor = Color.White,
                unfocusedTextColor = Color.White
            )
        )

        Spacer(modifier = Modifier.height(24.dp))

        Button(
            onClick = {
                if (code.length < 4) {
                    errorMsg = "Please enter valid code"
                    return@Button
                }
                loading = true
                errorMsg = ""
                prefs.serverUrl = serverUrl.trimEnd('/')

                coroutineScope.launch {
                    val result = api.pairDevice(code, devName)
                    loading = false
                    if (result != null) {
                        prefs.deviceId = result.deviceId
                        prefs.authToken = result.authToken
                        prefs.childId = result.childId
                        prefs.childDisplayName = result.childDisplayName
                        prefs.emergencyContact = result.emergencyContact
                        onPairSuccess()
                    } else {
                        errorMsg = "Invalid or expired pairing code."
                    }
                }
            },
            enabled = !loading,
            colors = ButtonDefaults.buttonColors(containerColor = Color(0xFF4F46E5)),
            shape = RoundedCornerShape(8.dp),
            modifier = Modifier.fillMaxWidth().height(48.dp)
        ) {
            Text(if (loading) "Pairing..." else "Pair This Phone", fontSize = 15.sp, fontWeight = FontWeight.SemiBold)
        }
    }
}

@Composable
fun AlreadyPairedScreen(prefs: DevicePrefs, onUnpair: () -> Unit) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center
    ) {
        Text(text = "✅", fontSize = 48.sp)
        Spacer(modifier = Modifier.height(16.dp))
        Text(
            text = "Device Managed",
            fontSize = 22.sp,
            fontWeight = FontWeight.Bold,
            color = Color.White
        )
        Spacer(modifier = Modifier.height(8.dp))
        Text(
            text = "Linked to ${prefs.childDisplayName}. Screen time is monitored by parent dashboard.",
            fontSize = 14.sp,
            color = Color(0xFF94A3B8),
            textAlign = TextAlign.Center
        )
        Spacer(modifier = Modifier.height(32.dp))
        OutlinedButton(
            onClick = onUnpair,
            colors = ButtonDefaults.outlinedButtonColors(contentColor = Color(0xFFFCA5A5)),
            modifier = Modifier.fillMaxWidth().height(44.dp)
        ) {
            Text("Unpair Device")
        }
    }
}

@Composable
fun ScreenTimeTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = darkColorScheme(
            primary = Color(0xFF4F46E5),
            background = Color(0xFF0F172A),
            surface = Color(0xFF1E293B)
        ),
        content = content
    )
}

