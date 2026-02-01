package com.shuttle.app

import android.annotation.SuppressLint
import android.app.Activity
import android.content.Intent
import android.net.VpnService
import android.os.Bundle
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient

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

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        webView = WebView(this).apply {
            settings.javaScriptEnabled = true
            settings.domStorageEnabled = true
            settings.cacheMode = WebSettings.LOAD_NO_CACHE
            webViewClient = WebViewClient()
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

        // Load WebView after a short delay to let VPN start
        webView.postDelayed({
            // In VPN mode, the API server address comes from the service
            // For simplicity, poll or use a fixed local address
            webView.loadUrl("http://127.0.0.1:12345")
        }, 2000)
    }

    override fun onDestroy() {
        if (ShuttleVpnService.isRunning) {
            stopService(Intent(this, ShuttleVpnService::class.java))
        } else {
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
