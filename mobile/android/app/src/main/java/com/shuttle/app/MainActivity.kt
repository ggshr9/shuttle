package com.shuttle.app

import android.annotation.SuppressLint
import android.app.Activity
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.net.VpnService
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.webkit.JavascriptInterface
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Toast

/**
 * Main activity that hosts the Shuttle SPA via WebView.
 *
 * Supports two modes:
 * 1. VPN mode: starts ShuttleVpnService for system-wide per-app proxying via TUN
 * 2. Proxy mode (fallback): runs Go engine directly with SOCKS5/HTTP proxy
 *
 * Build steps:
 * 1. gomobile bind -target=android -o app/libs/shuttle.aar ../../mobile
 * 2. Add shuttle.aar to app/libs/
 * 3. Build with Gradle
 */
class MainActivity : Activity() {

    companion object {
        private const val VPN_REQUEST_CODE = 1001
    }

    private lateinit var webView: WebView
    private var apiAddr: String? = null
    private var configJson: String? = null
    private val handler = Handler(Looper.getMainLooper())
    private var statusCheckRunnable: Runnable? = null

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        webView = WebView(this).apply {
            settings.javaScriptEnabled = true
            settings.domStorageEnabled = true
            settings.cacheMode = WebSettings.LOAD_NO_CACHE
            webViewClient = object : WebViewClient() {
                override fun onPageFinished(view: WebView?, url: String?) {
                    super.onPageFinished(view, url)
                    // Inject native bridge for VPN control
                    injectNativeBridge()
                }
            }
            // Add JavaScript interface for native VPN control
            addJavascriptInterface(VpnBridge(), "ShuttleNative")
        }
        setContentView(webView)

        Thread {
            try {
                configJson = assets.open("config.json").bufferedReader().readText()
                val cfg = org.json.JSONObject(configJson!!)
                val tunCfg = cfg.optJSONObject("proxy")?.optJSONObject("tun")
                val tunEnabled = tunCfg?.optBoolean("enabled", false) ?: false

                if (tunEnabled) {
                    // Request VPN permission on UI thread
                    runOnUiThread { requestVpnPermission() }
                } else {
                    // Proxy-only mode: start engine directly
                    apiAddr = mobile.Mobile.start(configJson)
                    runOnUiThread {
                        webView.loadUrl("http://$apiAddr")
                    }
                }
            } catch (e: Exception) {
                e.printStackTrace()
            }
        }.start()
    }

    private fun requestVpnPermission() {
        val intent = VpnService.prepare(this)
        if (intent != null) {
            startActivityForResult(intent, VPN_REQUEST_CODE)
        } else {
            startVpnService()
        }
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == VPN_REQUEST_CODE && resultCode == RESULT_OK) {
            startVpnService()
        }
    }

    private fun startVpnService() {
        val json = configJson ?: return
        val cfg = org.json.JSONObject(json)
        val tunCfg = cfg.optJSONObject("proxy")?.optJSONObject("tun")
        val perAppMode = tunCfg?.optString("per_app_mode", "") ?: ""
        val appListArr = tunCfg?.optJSONArray("app_list")
        val appList = if (appListArr != null) {
            (0 until appListArr.length()).joinToString(",") { appListArr.getString(it) }
        } else ""

        val intent = Intent(this, ShuttleVpnService::class.java).apply {
            putExtra(ShuttleVpnService.EXTRA_CONFIG_JSON, json)
            putExtra(ShuttleVpnService.EXTRA_PER_APP_MODE, perAppMode)
            putExtra(ShuttleVpnService.EXTRA_APP_LIST, appList)
        }
        startService(intent)

        // Poll for API server availability
        pollForApiServer()
    }

    private fun pollForApiServer() {
        var attempts = 0
        val maxAttempts = 20
        val pollInterval = 500L

        statusCheckRunnable = object : Runnable {
            override fun run() {
                if (ShuttleVpnService.isRunning) {
                    // Try to connect to API
                    val status = try {
                        mobile.Mobile.status()
                    } catch (e: Exception) { null }

                    if (status != null && status.contains("running")) {
                        // API is ready, load WebView
                        // Parse status to get API address
                        webView.loadUrl("http://127.0.0.1:12345")
                        return
                    }
                }

                attempts++
                if (attempts < maxAttempts) {
                    handler.postDelayed(this, pollInterval)
                } else {
                    Toast.makeText(this@MainActivity, "Failed to connect to VPN service", Toast.LENGTH_SHORT).show()
                }
            }
        }
        handler.post(statusCheckRunnable!!)
    }

    private fun stopVpnService() {
        val intent = Intent(this, ShuttleVpnService::class.java).apply {
            action = ShuttleVpnService.ACTION_STOP
        }
        startService(intent)
    }

    private fun injectNativeBridge() {
        val js = """
            window.isAndroid = true;
            window.ShuttleVPN = {
                isRunning: function() { return ShuttleNative.isVpnRunning(); },
                start: function() { ShuttleNative.startVpn(); },
                stop: function() { ShuttleNative.stopVpn(); },
                getStatus: function() { return ShuttleNative.getStatus(); }
            };
        """.trimIndent()
        webView.evaluateJavascript(js, null)
    }

    inner class VpnBridge {
        @JavascriptInterface
        fun isVpnRunning(): Boolean {
            return ShuttleVpnService.isRunning
        }

        @JavascriptInterface
        fun startVpn() {
            runOnUiThread { requestVpnPermission() }
        }

        @JavascriptInterface
        fun stopVpn() {
            runOnUiThread { stopVpnService() }
        }

        @JavascriptInterface
        fun getStatus(): String {
            return try {
                mobile.Mobile.status()
            } catch (e: Exception) {
                """{"state":"error","error":"${e.message}"}"""
            }
        }
    }

    override fun onDestroy() {
        statusCheckRunnable?.let { handler.removeCallbacks(it) }
        // Don't stop VPN service on activity destroy - let it run in background
        // Only stop if in proxy-only mode
        if (!ShuttleVpnService.isRunning) {
            try {
                mobile.Mobile.stop()
            } catch (_: Exception) {}
        }
        super.onDestroy()
    }

    override fun onBackPressed() {
        if (webView.canGoBack()) {
            webView.goBack()
        } else {
            super.onBackPressed()
        }
    }
}
