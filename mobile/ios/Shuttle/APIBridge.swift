import Foundation
import WebKit
import os.log
import SharedBridge

/// APIBridge wires JS-side `window.ShuttleBridge.send(envelope)` to native
/// `VPNManager.sendToExtension`. Each message from JS carries a unique id;
/// the response is sent back via `evaluateJavaScript("window.ShuttleBridge._complete(id, response)")`.
final class APIBridge: NSObject, WKScriptMessageHandler {
    weak var webView: WKWebView?
    private let manager: VPNManager
    private let log = OSLog(subsystem: "com.shuttle.app", category: "APIBridge")

    init(manager: VPNManager) {
        self.manager = manager
        super.init()
    }

    func userContentController(_ ucc: WKUserContentController, didReceive msg: WKScriptMessage) {
        guard let body = msg.body as? [String: Any],
              let id = body["id"] as? Int,
              let envelopeAny = body["envelope"] as? [String: Any] else {
            os_log("APIBridge: malformed message", log: log, type: .error)
            return
        }

        guard let envelopeData = try? JSONSerialization.data(withJSONObject: envelopeAny) else {
            failJS(id: id, message: "envelope encode failed")
            return
        }

        manager.sendToExtension(envelopeData, timeout: 30) { [weak self] response in
            self?.completeJS(id: id, response: response)
        }
    }

    private func completeJS(id: Int, response: Data?) {
        DispatchQueue.main.async { [weak self] in
            guard let self = self, let webView = self.webView else { return }
            if let data = response, let json = String(data: data, encoding: .utf8) {
                let js = "window.ShuttleBridge._complete(\(id), \(json))"
                webView.evaluateJavaScript(js, completionHandler: nil)
            } else {
                self.failJS(id: id, message: "IPC timeout or no response")
            }
        }
    }

    private func failJS(id: Int, message: String) {
        DispatchQueue.main.async { [weak self] in
            guard let self = self, let webView = self.webView else { return }
            // Single-quote escape for the message string. Newlines and quotes
            // are the realistic risks; envelope errors don't carry HTML.
            let safeMsg = message
                .replacingOccurrences(of: "\\", with: "\\\\")
                .replacingOccurrences(of: "'", with: "\\'")
                .replacingOccurrences(of: "\n", with: "\\n")
            let js = "window.ShuttleBridge._fail(\(id), '\(safeMsg)')"
            webView.evaluateJavaScript(js, completionHandler: nil)
        }
    }
}

/// JS injected at document start. Must match the BridgeTransport contract in
/// gui/web/src/lib/data/bridge-transport.ts: window.ShuttleBridge.send(env)
/// returns a Promise resolved by _complete or rejected by _fail with a
/// matching id.
let shuttleBridgeBootstrapJS: String = """
window.ShuttleBridge = (() => {
  const pending = new Map();
  let nextId = 0;
  return {
    send(envelope) {
      const id = ++nextId;
      return new Promise((resolve, reject) => {
        pending.set(id, { resolve, reject });
        webkit.messageHandlers.shuttleBridge.postMessage({ id, envelope });
      });
    },
    _complete(id, response) {
      const p = pending.get(id);
      if (!p) return;
      pending.delete(id);
      p.resolve(response);
    },
    _fail(id, message) {
      const p = pending.get(id);
      if (!p) return;
      pending.delete(id);
      p.reject(new Error(message));
    },
  };
})();
"""
