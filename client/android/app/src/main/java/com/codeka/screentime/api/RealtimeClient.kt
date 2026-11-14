package com.codeka.screentime.api

import com.codeka.screentime.data.DevicePrefs
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.json.JSONObject
import java.net.URI
import java.net.URLEncoder

class RealtimeClient(
    private val prefs: DevicePrefs,
    private val onEvent: (type: String, data: JSONObject) -> Unit
) {
    private val client = OkHttpClient()
    private var webSocket: WebSocket? = null
    private val scope = CoroutineScope(Dispatchers.IO + Job())
    private var isRunning = false

    private var isConnected = false

    fun start() {
        if (!prefs.isPaired || isRunning) return
        isRunning = true
        connect()
    }

    fun stop() {
        isRunning = false
        isConnected = false
        webSocket?.close(1000, "Normal closure")
        webSocket = null
    }

    private fun scheduleReconnect() {
        if (!isRunning) return
        scope.launch {
            delay(5000)
            if (isActive && isRunning && !isConnected) {
                connect()
            }
        }
    }

    private fun connect() {
        if (!isRunning || isConnected) return
        try {
            val baseUri = URI(prefs.serverUrl)
            val scheme = if (baseUri.scheme == "https") "wss" else "ws"
            val host = baseUri.authority
            val token = URLEncoder.encode(prefs.authToken, "UTF-8")
            val url = "$scheme://$host/api/ws/device?token=$token"

            val request = Request.Builder().url(url).build()
            isConnected = true
            webSocket = client.newWebSocket(request, object : WebSocketListener() {
                override fun onMessage(webSocket: WebSocket, text: String) {
                    try {
                        val obj = JSONObject(text)
                        val type = obj.optString("type", "")
                        onEvent(type, obj)
                    } catch (e: Exception) {
                        e.printStackTrace()
                    }
                }

                override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                    isConnected = false
                    scheduleReconnect()
                }

                override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                    isConnected = false
                    scheduleReconnect()
                }
            })
        } catch (e: Exception) {
            isConnected = false
            e.printStackTrace()
            scheduleReconnect()
        }
    }
}

