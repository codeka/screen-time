package com.codeka.screentime

import android.app.Application
import android.app.NotificationChannel
import android.app.NotificationManager
import android.os.Build

class ScreenTimeApp : Application() {

    override fun onCreate() {
        super.onCreate()
        createNotificationChannels()
    }

    private fun createNotificationChannels() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val serviceChannel = NotificationChannel(
                CHANNEL_SERVICE,
                "Screen Time Service",
                NotificationManager.IMPORTANCE_LOW
            ).apply {
                description = "Keeps screen time enforcement active"
            }

            val alertChannel = NotificationChannel(
                CHANNEL_ALERTS,
                "Screen Time Warnings",
                NotificationManager.IMPORTANCE_HIGH
            ).apply {
                description = "Notifies when screen time is expiring"
            }

            val manager = getSystemService(NotificationManager::class.java)
            manager.createNotificationChannel(serviceChannel)
            manager.createNotificationChannel(alertChannel)
        }
    }

    companion object {
        const val CHANNEL_SERVICE = "screentime_service_channel"
        const val CHANNEL_ALERTS = "screentime_alerts_channel"
    }
}

