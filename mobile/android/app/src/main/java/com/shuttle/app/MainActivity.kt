package com.shuttle.app

import android.annotation.SuppressLint
import android.app.Activity
import android.os.Bundle
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient

/**
 * Main activity that hosts the Shuttle SPA via WebView.
 *
 * The Go engine + API server runs in-process via gomobile bindings.
 * The WebView loads http://127.0.0.1:{port} to render the shared Svelte UI.
 *
 * Build steps:
 * 1. gomobile bind -target=android -o app/libs/shuttle.aar ../../mobile
 * 2. Add shuttle.aar to app/libs/
 * 3. Build with Gradle
 */
class MainActivity : Activity() {

    private lateinit var webView: WebView
    private var apiAddr: String? = null

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

        // Start engine in background thread
        Thread {
            try {
                // Load config from assets or shared preferences
                val configJson = assets.open("config.json").bufferedReader().readText()
                apiAddr = mobile.Mobile.start(configJson)
                runOnUiThread {
                    webView.loadUrl("http://$apiAddr")
                }
            } catch (e: Exception) {
                e.printStackTrace()
            }
        }.start()
    }

    override fun onDestroy() {
        try {
            mobile.Mobile.stop()
        } catch (_: Exception) {}
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
