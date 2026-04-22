package com.shuttle.app

import android.annotation.SuppressLint
import android.app.Activity
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.net.Uri
import android.net.VpnService
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.webkit.JavascriptInterface
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Toast
import org.json.JSONObject

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
        private const val QR_REQUEST_CODE = 1002
        private const val MAX_POLL_ATTEMPTS = 40
        private const val POLL_INTERVAL_MS = 500L
    }

    private lateinit var webView: WebView
    private var apiAddr: String? = null
    private var configJson: String? = null
    private val handler = Handler(Looper.getMainLooper())
    private var statusCheckRunnable: Runnable? = null

    // Tracks pending bridge requests awaiting async results (permission dialog,
    // QR scan). Resolved via _shuttleResolve(id, value) after onActivityResult.
    private var pendingPermissionId: Int? = null
    private var pendingScanId: Int? = null

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
        if (requestCode == VPN_REQUEST_CODE) {
            val pending = pendingPermissionId
            pendingPermissionId = null
            if (resultCode == RESULT_OK) {
                pending?.let { resolveBridge(it, "\"granted\"") }
                // Legacy path — startVpnService was fired directly when permission
                // came from the config-driven (tun.enabled) bootstrap. With the
                // bridge path, SPA decides when to start; don't auto-kick.
                if (pending == null) {
                    startVpnService()
                }
            } else {
                pending?.let { resolveBridge(it, "\"denied\"") }
            }
            return
        }
        if (requestCode == QR_REQUEST_CODE) {
            val pending = pendingScanId
            pendingScanId = null
            val code = data?.getStringExtra("qr") ?: ""
            if (pending != null) {
                if (resultCode == RESULT_OK && code.isNotEmpty()) {
                    // Escape the QR code content for safe JSON injection.
                    val escaped = code.replace("\\", "\\\\").replace("\"", "\\\"")
                    resolveBridge(pending, "\"$escaped\"")
                } else {
                    rejectBridge(pending, "qr scan cancelled")
                }
            }
            return
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

        statusCheckRunnable = object : Runnable {
            override fun run() {
                if (ShuttleVpnService.isRunning) {
                    // Get the actual API address from the VPN service
                    val addr = ShuttleVpnService.apiAddress
                    if (addr != null) {
                        apiAddr = addr
                        webView.loadUrl("http://$addr")
                        return
                    }

                    // Fallback: try to detect from engine status
                    val status = try {
                        mobile.Mobile.status()
                    } catch (e: Exception) { null }

                    if (status != null && status.contains("running")) {
                        // Engine is running but we don't have the address — try status parsing
                        try {
                            val json = org.json.JSONObject(status)
                            val detectedAddr = json.optString("api_addr", "")
                            if (detectedAddr.isNotEmpty()) {
                                apiAddr = detectedAddr
                                webView.loadUrl("http://$detectedAddr")
                                return
                            }
                        } catch (_: Exception) {}
                    }
                }

                attempts++
                if (attempts < MAX_POLL_ATTEMPTS) {
                    handler.postDelayed(this, POLL_INTERVAL_MS)
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
                getStatus: function() { return ShuttleNative.getStatus(); },
                invoke: function(msg) { ShuttleNative.invoke(msg); }
            };
        """.trimIndent()
        webView.evaluateJavascript(js, null)
    }

    /** Resolves a pending bridge call in JS. `jsonValue` must be JSON (quoted string, number, etc). */
    private fun resolveBridge(id: Int, jsonValue: String) {
        runOnUiThread {
            webView.evaluateJavascript("window._shuttleResolve && window._shuttleResolve($id, $jsonValue);", null)
        }
    }

    /** Rejects a pending bridge call with a string error. */
    private fun rejectBridge(id: Int, message: String) {
        runOnUiThread {
            val escaped = message.replace("\\", "\\\\").replace("\"", "\\\"")
            webView.evaluateJavascript("window._shuttleReject && window._shuttleReject($id, \"$escaped\");", null)
        }
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

        @JavascriptInterface
        fun invoke(msg: String) {
            // Parse the Promise-wrapped message from lib/platform/shuttle-bridge.ts:
            //   { id: number, action: string, payload?: unknown }
            val json = try { JSONObject(msg) } catch (e: Exception) {
                android.util.Log.w("ShuttleBridge", "invoke: malformed json", e)
                return
            }
            val id = json.optInt("id", -1)
            val action = json.optString("action")
            if (id < 0 || action.isEmpty()) return

            when (action) {
                "requestPermission" -> handleRequestPermission(id)
                "scanQR"            -> handleScanQR(id)
                "share"             -> handleShare(id, json.optJSONObject("payload"))
                "openExternal"      -> handleOpenExternal(id, json.optJSONObject("payload"))
                "subscribeStatus"   -> rejectBridge(id, "subscribeStatus not implemented yet")
                else -> rejectBridge(id, "unknown action: $action")
            }
        }

        private fun handleRequestPermission(id: Int) {
            runOnUiThread {
                val intent = VpnService.prepare(this@MainActivity)
                if (intent == null) {
                    // Permission already granted for this app.
                    resolveBridge(id, "\"granted\"")
                } else {
                    pendingPermissionId = id
                    startActivityForResult(intent, VPN_REQUEST_CODE)
                }
            }
        }

        private fun handleScanQR(id: Int) {
            runOnUiThread {
                val intent = Intent(this@MainActivity, QrScanActivity::class.java)
                pendingScanId = id
                startActivityForResult(intent, QR_REQUEST_CODE)
            }
        }

        private fun handleShare(id: Int, payload: JSONObject?) {
            runOnUiThread {
                val title = payload?.optString("title") ?: ""
                val text  = payload?.optString("text") ?: payload?.optString("url") ?: ""
                val intent = Intent(Intent.ACTION_SEND).apply {
                    type = "text/plain"
                    if (title.isNotEmpty()) putExtra(Intent.EXTRA_SUBJECT, title)
                    if (text.isNotEmpty())  putExtra(Intent.EXTRA_TEXT, text)
                }
                try {
                    startActivity(Intent.createChooser(intent, null))
                    resolveBridge(id, "\"ok\"")
                } catch (e: Exception) {
                    rejectBridge(id, e.message ?: "share failed")
                }
            }
        }

        private fun handleOpenExternal(id: Int, payload: JSONObject?) {
            runOnUiThread {
                val url = payload?.optString("url") ?: ""
                if (url.isEmpty()) { rejectBridge(id, "no url"); return@runOnUiThread }
                try {
                    startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
                    resolveBridge(id, "\"ok\"")
                } catch (e: Exception) {
                    rejectBridge(id, e.message ?: "open failed")
                }
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
