package com.shuttle.app

import android.content.Intent
import android.net.VpnService
import android.os.ParcelFileDescriptor
import android.util.Log

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
        const val EXTRA_CONFIG_JSON = "config_json"
        const val EXTRA_PER_APP_MODE = "per_app_mode"
        const val EXTRA_APP_LIST = "app_list"

        var isRunning = false
            private set
    }

    private var vpnInterface: ParcelFileDescriptor? = null
    private var apiAddr: String? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent == null) {
            stopSelf()
            return START_NOT_STICKY
        }

        val configJson = intent.getStringExtra(EXTRA_CONFIG_JSON) ?: run {
            Log.e(TAG, "No config provided")
            stopSelf()
            return START_NOT_STICKY
        }
        val perAppMode = intent.getStringExtra(EXTRA_PER_APP_MODE) ?: ""
        val appListStr = intent.getStringExtra(EXTRA_APP_LIST) ?: ""
        val appList = if (appListStr.isNotEmpty()) appListStr.split(",") else emptyList()

        Thread {
            try {
                startVpn(configJson, perAppMode, appList)
            } catch (e: Exception) {
                Log.e(TAG, "Failed to start VPN", e)
                stopSelf()
            }
        }.start()

        return START_STICKY
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
        isRunning = true
        Log.i(TAG, "Engine started, API at $apiAddr")
    }

    override fun onDestroy() {
        isRunning = false
        try {
            mobile.Mobile.stop()
        } catch (_: Exception) {}
        vpnInterface?.close()
        vpnInterface = null
        super.onDestroy()
    }

    override fun onRevoke() {
        onDestroy()
    }
}
