package com.codeka.screentime.api

import com.codeka.screentime.data.DevicePrefs
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.util.concurrent.TimeUnit

data class PairResult(
    val deviceId: String,
    val authToken: String,
    val childId: String,
    val childDisplayName: String,
    val emergencyContact: String
)

data class AccessStatusResult(
    val access: String, // "ALLOWED" | "LOCKED"
    val reason: String, // "GRANT" | "SCHEDULE" | "NONE"
    val validUntil: String?,
    val remainingSeconds: Long,
    val childDisplayName: String,
    val emergencyContact: String
)

class ApiClient(private val prefs: DevicePrefs) {
    private val client = OkHttpClient.Builder()
        .connectTimeout(5, TimeUnit.SECONDS)
        .readTimeout(5, TimeUnit.SECONDS)
        .build()

    private val jsonMediaType = "application/json; charset=utf-8".toMediaType()

    suspend fun pairDevice(pairingCode: String, deviceName: String): PairResult? =
        withContext(Dispatchers.IO) {
            try {
                val json = JSONObject().apply {
                    put("pairing_code", pairingCode.trim().uppercase())
                    put("device_name", deviceName)
                    put("platform", "ANDROID")
                }
                val body = json.toString().toRequestBody(jsonMediaType)
                val request = Request.Builder()
                    .url("${prefs.serverUrl}/api/devices/pair")
                    .post(body)
                    .build()

                client.newCall(request).execute().use { response ->
                    if (response.isSuccessful) {
                        val respBody = response.body?.string() ?: return@use null
                        val obj = JSONObject(respBody)
                        return@use PairResult(
                            deviceId = obj.getString("device_id"),
                            authToken = obj.getString("auth_token"),
                            childId = obj.getString("child_id"),
                            childDisplayName = obj.optString("child_display_name", ""),
                            emergencyContact = obj.optString("emergency_contact", "")
                        )
                    }
                }
            } catch (e: Exception) {
                e.printStackTrace()
            }
            null
        }

    suspend fun getAccessStatus(): AccessStatusResult? = withContext(Dispatchers.IO) {
        if (!prefs.isPaired) return@withContext null
        try {
            val request = Request.Builder()
                .url("${prefs.serverUrl}/api/devices/access-status")
                .header("Authorization", "Bearer ${prefs.authToken}")
                .get()
                .build()

            client.newCall(request).execute().use { response ->
                if (response.isSuccessful) {
                    val respBody = response.body?.string() ?: return@use null
                    val obj = JSONObject(respBody)
                    return@use AccessStatusResult(
                        access = obj.optString("access", "LOCKED"),
                        reason = obj.optString("reason", "NONE"),
                        validUntil = if (obj.has("valid_until")) obj.getString("valid_until") else null,
                        remainingSeconds = obj.optLong("remaining_seconds", 0L),
                        childDisplayName = obj.optString("child_display_name", ""),
                        emergencyContact = obj.optString("emergency_contact", "")
                    )
                }
            }
        } catch (e: Exception) {
            e.printStackTrace()
        }
        null
    }

    suspend fun requestScreenTime(): Boolean = withContext(Dispatchers.IO) {
        if (!prefs.isPaired) return@withContext false
        try {
            val body = "".toRequestBody(jsonMediaType)
            val request = Request.Builder()
                .url("${prefs.serverUrl}/api/requests")
                .header("Authorization", "Bearer ${prefs.authToken}")
                .post(body)
                .build()

            client.newCall(request).execute().use { response ->
                return@use response.isSuccessful
            }
        } catch (e: Exception) {
            e.printStackTrace()
            false
        }
    }

    suspend fun sendHeartbeat(uptimeSeconds: Long, status: String) = withContext(Dispatchers.IO) {
        if (!prefs.isPaired) return@withContext
        try {
            val json = JSONObject().apply {
                put("uptime_seconds", uptimeSeconds)
                put("reported_status", status)
            }
            val body = json.toString().toRequestBody(jsonMediaType)
            val request = Request.Builder()
                .url("${prefs.serverUrl}/api/devices/heartbeat")
                .header("Authorization", "Bearer ${prefs.authToken}")
                .post(body)
                .build()

            client.newCall(request).execute().close()
        } catch (e: Exception) {
            // Heartbeat failures logged silently
        }
    }
}

