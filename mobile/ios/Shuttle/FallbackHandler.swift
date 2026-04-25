import Foundation
import WebKit
import os.log

/// FallbackHandler is the Phase β safety net: when the JS-side bridge probe
/// (boot.ts) determines that envelope IPC is broken (timeout, 5xx healthz,
/// missing window.ShuttleBridge under bridge=1 force flag), the SPA posts a
/// fallback signal via webkit.messageHandlers.fallback. This handler reloads
/// the WKWebView with an inline HTML page so the user gets at least a working
/// Connect button.
///
/// Removed in Phase γ (Task 6.4) once TestFlight confirms the bridge path.
final class FallbackHandler: NSObject, WKScriptMessageHandler {
    weak var webView: WKWebView?
    private let inlineHTML: String
    private let log = OSLog(subsystem: "com.shuttle.app", category: "Fallback")

    init(inlineHTML: String) {
        self.inlineHTML = inlineHTML
        super.init()
    }

    func userContentController(_ ucc: WKUserContentController, didReceive msg: WKScriptMessage) {
        let reason = (msg.body as? [String: Any])?["reason"] as? String ?? "unknown"
        os_log("Bridge fallback triggered: %{public}@", log: log, type: .error, reason)
        DispatchQueue.main.async { [weak self] in
            guard let self = self, let webView = self.webView else { return }
            webView.loadHTMLString(self.inlineHTML, baseURL: nil)
        }
    }
}
