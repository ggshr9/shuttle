package com.shuttle.app

import android.app.Activity
import android.content.Intent
import android.os.Bundle
import com.journeyapps.barcodescanner.CaptureActivity
import com.google.zxing.integration.android.IntentIntegrator

/**
 * Thin wrapper around ZXing's CaptureActivity that returns the scanned QR
 * string via the "qr" intent extra. Started from MainActivity.VpnBridge
 * when the SPA calls ShuttleVPN.invoke({action: 'scanQR'}).
 */
class QrScanActivity : Activity() {

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val integrator = IntentIntegrator(this).apply {
            setDesiredBarcodeFormats(IntentIntegrator.QR_CODE)
            setOrientationLocked(true)
            setBeepEnabled(false)
        }
        integrator.initiateScan()
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        val result = IntentIntegrator.parseActivityResult(requestCode, resultCode, data)
        val code = result?.contents ?: ""
        val out = Intent().apply { putExtra("qr", code) }
        setResult(if (code.isNotEmpty()) RESULT_OK else RESULT_CANCELED, out)
        finish()
    }
}
