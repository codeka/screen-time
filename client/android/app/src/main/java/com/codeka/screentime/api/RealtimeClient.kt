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

    fun start() {
        if (!prefs.isPaired || isRunning) return
        isRunning = true
        scope.launch {
            while (isActive && isRunning) {
                connect()
                delay(5000)
            }
        }
    }

    fun stop() {
        isRunning = false
        webSocket?.close(1000, "Normal closure")
        webSocket = null
    }

    private fun connect() {
        try {
            val baseUri = URI(prefs.serverUrl)
            val scheme = if (baseUri.scheme == "https") "wss" else "ws"
            val host = baseUri.authority
            val token = URLEncoder.encode(prefs.authToken, "UTF-8")
            val url = "$scheme://$host/api/ws/device?token=$token"

            val request = Request.Builder().url(url).build()
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

                override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                    // Reconnect on next loop
                }
            })
        } catch (e: Exception) {
            e.printStackTrace()
        }
    }
}

