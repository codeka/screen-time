package com.codeka.screentime.data

import android.content.Context
import android.content.SharedPreferences

class DevicePrefs(context: Context) {
    private val prefs: SharedPreferences =
        context.getSharedPreferences("screentime_prefs", Context.MODE_PRIVATE)

    var serverUrl: String
        get() = prefs.getString("server_url", "https://screentime.codeka.com") ?: "https://screentime.codeka.com"
        set(value) = prefs.edit().putString("server_url", value).apply()

    var deviceId: String
        get() = prefs.getString("device_id", "") ?: ""
        set(value) = prefs.edit().putString("device_id", value).apply()

    var authToken: String
        get() = prefs.getString("auth_token", "") ?: ""
        set(value) = prefs.edit().putString("auth_token", value).apply()

    var childId: String
        get() = prefs.getString("child_id", "") ?: ""
        set(value) = prefs.edit().putString("child_id", value).apply()

    var childDisplayName: String
        get() = prefs.getString("child_display_name", "") ?: ""
        set(value) = prefs.edit().putString("child_display_name", value).apply()

    var emergencyContact: String
        get() = prefs.getString("emergency_contact", "") ?: ""
        set(value) = prefs.edit().putString("emergency_contact", value).apply()

    val isPaired: Boolean
        get() = deviceId.isNotEmpty() && authToken.isNotEmpty()

    fun clear() {
        prefs.edit().clear().apply()
    }
}

