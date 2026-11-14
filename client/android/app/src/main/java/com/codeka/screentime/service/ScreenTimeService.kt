package com.codeka.screentime.service

import android.app.Notification
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.pm.ServiceInfo
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.graphics.Color
import android.graphics.PixelFormat
import android.net.Uri
import android.os.Build
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.os.SystemClock
import android.provider.Settings
import android.view.Gravity
import android.view.View
import android.view.WindowManager
import android.widget.Button
import android.widget.LinearLayout
import android.widget.TextView
import androidx.core.app.NotificationCompat
import com.codeka.screentime.ScreenTimeApp
import com.codeka.screentime.api.ApiClient
import com.codeka.screentime.api.RealtimeClient
import com.codeka.screentime.data.DevicePrefs
import com.codeka.screentime.ui.MainActivity
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

class ScreenTimeService : Service() {

    private lateinit var prefs: DevicePrefs
    private lateinit var api: ApiClient
    private lateinit var realtime: RealtimeClient
    private lateinit var windowManager: WindowManager

    private val scope = CoroutineScope(Dispatchers.Main + Job())
    private var overlayView: View? = null
    private var isLocked = false
    private var remainingSeconds = 0L
    private var warned5Min = false
    private var warned1Min = false

    private val handler = Handler(Looper.getMainLooper())
    private var countdownRunnable: Runnable? = null
    private var heartbeatRunnable: Runnable? = null

    private val screenReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            when (intent?.action) {
                Intent.ACTION_USER_PRESENT, Intent.ACTION_SCREEN_ON -> {
                    checkAccessAndApply()
                }
            }
        }
    }

    override fun onCreate() {
        super.onCreate()
        prefs = DevicePrefs(this)
        api = ApiClient(prefs)
        windowManager = getSystemService(Context.WINDOW_SERVICE) as WindowManager

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startForeground(1001, buildForegroundNotification(), ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else {
            startForeground(1001, buildForegroundNotification())
        }

        val filter = IntentFilter().apply {
            addAction(Intent.ACTION_USER_PRESENT)
            addAction(Intent.ACTION_SCREEN_ON)
        }
        registerReceiver(screenReceiver, filter)

        setupRealtime()
        startHeartbeat()
        startCountdown()

        // Initial pre-popup check
        checkAccessAndApply()
    }

    private fun setupRealtime() {
        realtime = RealtimeClient(prefs) { type, data ->
            handler.post {
                when (type) {
                    "ACCESS_UPDATE" -> {
                        if (data.optString("access") == "LOCKED") {
                            lock()
                        } else {
                            val remaining = data.optLong("remaining_seconds", 1800L)
                            unlock(remaining)
                        }
                    }

                    "SCHEDULE_UPDATED" -> {
                        checkAccessAndApply()
                    }

                    "REQUEST_DENIED" -> {
                        lock()
                    }
                }
            }
        }
        realtime.start()
    }

    private fun checkAccessAndApply() {
        scope.launch {
            val status = api.getAccessStatus()
            if (status != null && status.access == "ALLOWED") {
                unlock(status.remainingSeconds)
            } else {
                lock()
            }
        }
    }

    private fun lock() {
        if (isLocked && overlayView != null) return
        isLocked = true

        if (!Settings.canDrawOverlays(this)) return

        if (overlayView == null) {
            val layoutParams = WindowManager.LayoutParams(
                WindowManager.LayoutParams.MATCH_PARENT,
                WindowManager.LayoutParams.MATCH_PARENT,
                WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY,
                WindowManager.LayoutParams.FLAG_NOT_TOUCH_MODAL or
                        WindowManager.LayoutParams.FLAG_LAYOUT_IN_SCREEN,
                PixelFormat.TRANSLUCENT
            ).apply {
                gravity = Gravity.CENTER
            }

            val view = createOverlayView()
            overlayView = view
            try {
                windowManager.addView(view, layoutParams)
            } catch (e: Exception) {
                e.printStackTrace()
            }
        }
    }

    private fun unlock(seconds: Long) {
        isLocked = false
        remainingSeconds = if (seconds > 0) seconds else 1800L
        warned5Min = false
        warned1Min = false

        if (overlayView != null) {
            try {
                windowManager.removeView(overlayView)
            } catch (e: Exception) {
                e.printStackTrace()
            }
            overlayView = null
        }
    }

    private fun createOverlayView(): View {
        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            gravity = Gravity.CENTER
            setBackgroundColor(Color.parseColor("#0F172A"))
            setPadding(48, 48, 48, 48)
        }

        val card = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            gravity = Gravity.CENTER
            setBackgroundColor(Color.parseColor("#1E293B"))
            setPadding(48, 48, 48, 48)
        }

        val clockText = TextView(this).apply {
            textSize = 40f
            setTextColor(Color.WHITE)
            gravity = Gravity.CENTER
            text = SimpleDateFormat("h:mm a", Locale.getDefault()).format(Date())
        }

        val greetingText = TextView(this).apply {
            textSize = 20f
            setTextColor(Color.WHITE)
            gravity = Gravity.CENTER
            text = "Hi, ${prefs.childDisplayName.ifEmpty { "Lucas" }}!"
            setPadding(0, 16, 0, 8)
        }

        val statusText = TextView(this).apply {
            textSize = 14f
            setTextColor(Color.parseColor("#FBBF24"))
            gravity = Gravity.CENTER
            text = "🔒 SCREEN TIME LOCKED"
            setPadding(0, 0, 0, 24)
        }

        val infoText = TextView(this).apply {
            textSize = 13f
            setTextColor(Color.parseColor("#94A3B8"))
            gravity = Gravity.CENTER
            text = "Your device is paused. Ask your parents for time or use emergency dialer."
            setPadding(0, 0, 0, 32)
        }

        val requestBtn = Button(this).apply {
            text = "✋ Request Screen Time"
            setBackgroundColor(Color.parseColor("#4F46E5"))
            setTextColor(Color.WHITE)
            setPadding(24, 24, 24, 24)
            setOnClickListener {
                isEnabled = false
                text = "Sending Request..."
                scope.launch {
                    val success = api.requestScreenTime()
                    if (success) {
                        text = "Waiting for Parent..."
                        infoText.text = "Request sent to parent's phone! When approved, this screen will unlock."
                    } else {
                        isEnabled = true
                        text = "Try Again"
                    }
                }
            }
        }

        val emergencyBtn = Button(this).apply {
            text = "📞 Emergency Call"
            setBackgroundColor(Color.parseColor("#334155"))
            setTextColor(Color.parseColor("#E2E8F0"))
            setPadding(16, 16, 16, 16)
            setOnClickListener {
                val intent = if (prefs.emergencyContact.isNotEmpty()) {
                    Intent(Intent.ACTION_DIAL, Uri.parse("tel:${prefs.emergencyContact}"))
                } else {
                    Intent(Intent.ACTION_DIAL)
                }.apply {
                    flags = Intent.FLAG_ACTIVITY_NEW_TASK
                }
                startActivity(intent)
            }
        }

        card.addView(clockText)
        card.addView(greetingText)
        card.addView(statusText)
        card.addView(infoText)
        card.addView(requestBtn)

        val spacer = View(this).apply {
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, 24
            )
        }
        card.addView(spacer)
        card.addView(emergencyBtn)

        root.addView(card)
        return root
    }

    private fun startCountdown() {
        countdownRunnable = object : Runnable {
            override fun run() {
                if (!isLocked && remainingSeconds > 0) {
                    remainingSeconds--

                    if (remainingSeconds in 291..300 && !warned5Min) {
                        warned5Min = true
                        sendWarningNotification(
                            "Screen Time: 5 Minutes Remaining",
                            "Your screen time expires in 5 minutes."
                        )
                    }

                    if (remainingSeconds in 51..60 && !warned1Min) {
                        warned1Min = true
                        sendWarningNotification("Screen Time: 1 Minute Remaining", "The screen will lock shortly.")
                    }

                    if (remainingSeconds <= 0) {
                        lock()
                    }
                }
                handler.postDelayed(this, 1000)
            }
        }
        handler.post(countdownRunnable!!)
    }

    private fun startHeartbeat() {
        heartbeatRunnable = object : Runnable {
            override fun run() {
                scope.launch {
                    val uptimeSec = SystemClock.elapsedRealtime() / 1000
                    api.sendHeartbeat(uptimeSec, if (isLocked) "LOCKED" else "ACTIVE")
                }
                handler.postDelayed(this, 60000)
            }
        }
        handler.post(heartbeatRunnable!!)
    }

    private fun sendWarningNotification(title: String, message: String) {
        val notification = NotificationCompat.Builder(this, ScreenTimeApp.CHANNEL_ALERTS)
            .setContentTitle(title)
            .setContentText(message)
            .setSmallIcon(android.R.drawable.ic_lock_idle_lock)
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .setAutoCancel(true)
            .build()

        val manager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        manager.notify(2002, notification)
    }

    private fun buildForegroundNotification(): Notification {
        val pendingIntent = PendingIntent.getActivity(
            this, 0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE
        )

        return NotificationCompat.Builder(this, ScreenTimeApp.CHANNEL_SERVICE)
            .setContentTitle("Screen Time Active")
            .setContentText("Device is monitored and protected")
            .setSmallIcon(android.R.drawable.ic_lock_idle_lock)
            .setOngoing(true)
            .setContentIntent(pendingIntent)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .build()
    }

    override fun onDestroy() {
        unregisterReceiver(screenReceiver)
        realtime.stop()
        countdownRunnable?.let { handler.removeCallbacks(it) }
        heartbeatRunnable?.let { handler.removeCallbacks(it) }
        scope.cancel()
        if (overlayView != null) {
            try {
                windowManager.removeView(overlayView)
            } catch (e: Exception) {
            }
        }
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null
}

