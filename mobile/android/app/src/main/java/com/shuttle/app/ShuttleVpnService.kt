package com.shuttle.app

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log
import androidx.core.app.NotificationCompat

/**
 * ShuttleVpnService creates a TUN device via Android VpnService API,
 * applies per-app filtering, and passes the TUN fd to the Go engine.
 *
 * Config is passed via intent extras:
 * - "config_json": full engine config JSON string
 * - "per_app_mode": "allow" or "deny" (optional)
 * - "app_list": comma-separated package names (optional)
 */
class ShuttleVpnService : VpnService() {

    companion object {
        private const val TAG = "ShuttleVpn"
        private const val NOTIFICATION_ID = 1
        private const val CHANNEL_ID = "shuttle_vpn"

        const val EXTRA_CONFIG_JSON = "config_json"
        const val EXTRA_PER_APP_MODE = "per_app_mode"
        const val EXTRA_APP_LIST = "app_list"
        const val ACTION_STOP = "com.shuttle.app.STOP_VPN"

        var isRunning = false
            private set

        var apiAddress: String? = null
            private set

        var lastError: String? = null
            private set

        var bytesReceived: Long = 0
            private set

        var bytesSent: Long = 0
            private set
    }

    private var vpnInterface: ParcelFileDescriptor? = null
    private var apiAddr: String? = null

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            stopSelf()
            return START_NOT_STICKY
        }

        if (intent == null) {
            stopSelf()
            return START_NOT_STICKY
        }

        val configJson = intent.getStringExtra(EXTRA_CONFIG_JSON) ?: run {
            Log.e(TAG, "No config provided")
            lastError = "No configuration provided"
            stopSelf()
            return START_NOT_STICKY
        }
        val perAppMode = intent.getStringExtra(EXTRA_PER_APP_MODE) ?: ""
        val appListStr = intent.getStringExtra(EXTRA_APP_LIST) ?: ""
        val appList = if (appListStr.isNotEmpty()) appListStr.split(",") else emptyList()

        // Start foreground service with notification
        startForeground(NOTIFICATION_ID, createNotification("Connecting..."))

        Thread {
            try {
                startVpn(configJson, perAppMode, appList)
                updateNotification("Connected")
            } catch (e: Exception) {
                Log.e(TAG, "Failed to start VPN", e)
                lastError = e.message
                updateNotification("Connection failed")
                stopSelf()
            }
        }.start()

        return START_STICKY
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                "Shuttle VPN",
                NotificationManager.IMPORTANCE_LOW
            ).apply {
                description = "VPN connection status"
                setShowBadge(false)
            }
            val manager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
            manager.createNotificationChannel(channel)
        }
    }

    private fun createNotification(status: String): Notification {
        val stopIntent = Intent(this, ShuttleVpnService::class.java).apply {
            action = ACTION_STOP
        }
        val stopPendingIntent = PendingIntent.getService(
            this, 0, stopIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val openIntent = Intent(this, MainActivity::class.java)
        val openPendingIntent = PendingIntent.getActivity(
            this, 0, openIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("Shuttle VPN")
            .setContentText(status)
            .setSmallIcon(android.R.drawable.ic_lock_lock)
            .setOngoing(true)
            .setContentIntent(openPendingIntent)
            .addAction(android.R.drawable.ic_menu_close_clear_cancel, "Stop", stopPendingIntent)
            .build()
    }

    private fun updateNotification(status: String) {
        val manager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        manager.notify(NOTIFICATION_ID, createNotification(status))
    }

    private fun startVpn(configJson: String, perAppMode: String, appList: List<String>) {
        val builder = Builder()
            .setSession("Shuttle")
            .setMtu(1500)
            .addAddress("198.18.0.1", 16)
            .addRoute("0.0.0.0", 0)
            .addDnsServer("198.18.0.2")

        // Per-app filtering
        when (perAppMode) {
            "allow" -> {
                for (pkg in appList) {
                    try {
                        builder.addAllowedApplication(pkg.trim())
                    } catch (e: Exception) {
                        Log.w(TAG, "Cannot add allowed app: $pkg", e)
                    }
                }
            }
            "deny" -> {
                for (pkg in appList) {
                    try {
                        builder.addDisallowedApplication(pkg.trim())
                    } catch (e: Exception) {
                        Log.w(TAG, "Cannot add disallowed app: $pkg", e)
                    }
                }
            }
        }

        // Always exclude ourselves to prevent routing loops
        try {
            builder.addDisallowedApplication(packageName)
        } catch (_: Exception) {}

        vpnInterface = builder.establish() ?: run {
            Log.e(TAG, "Failed to establish VPN interface")
            return
        }

        val fd = vpnInterface!!.fd
        Log.i(TAG, "VPN established, fd=$fd")

        apiAddr = mobile.Mobile.startWithTUN(configJson, fd.toLong())
        apiAddress = apiAddr
        isRunning = true
        Log.i(TAG, "Engine started, API at $apiAddr")
    }

    override fun onDestroy() {
        isRunning = false
        apiAddress = null
        try {
            // Get final stats
            val status = mobile.Mobile.status()
            Log.i(TAG, "Final status: $status")
            mobile.Mobile.stop()
        } catch (e: Exception) {
            Log.w(TAG, "Error stopping engine", e)
        }
        vpnInterface?.close()
        vpnInterface = null
        stopForeground(STOP_FOREGROUND_REMOVE)
        super.onDestroy()
    }

    override fun onRevoke() {
        onDestroy()
    }
}
