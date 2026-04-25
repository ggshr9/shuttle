import UIKit
import WebKit
import NetworkExtension
import AVFoundation
import os.log
import SharedBridge

/// Main view controller that hosts the Shuttle SPA via WKWebView.
///
/// Supports two modes:
/// 1. VPN mode: Uses Network Extension for system-wide proxying
/// 2. Proxy mode: Runs Go engine in-process for SOCKS5/HTTP proxy
///
/// Build steps:
/// 1. gomobile bind -target=ios -o Shuttle.xcframework ../../mobile
/// 2. Add Shuttle.xcframework to Xcode project
/// 3. Add Network Extension target
/// 4. Build with Xcode
class ViewController: UIViewController, WKNavigationDelegate, WKScriptMessageHandler {

    private var webView: WKWebView!
    private var apiAddr: String?
    private var useVPN = false
    private var configData: String?
    private var apiBridge: APIBridge!
    private var fallbackHandler: FallbackHandler!

    override func viewDidLoad() {
        super.viewDidLoad()

        setupWebView()
        loadConfig()

        // Listen for VPN status changes
        NotificationCenter.default.addObserver(
            self,
            selector: #selector(vpnStatusChanged),
            name: .shuttleVPNStatusChanged,
            object: nil
        )
    }

    private func setupWebView() {
        let contentController = WKUserContentController()
        contentController.add(self, name: "shuttleNative")

        // Register the APIBridge handler for envelope IPC. Used by JS-side
        // BridgeAdapter when iOS VPN mode is active. Co-exists with the
        // existing ShuttleVPN capability surface.
        apiBridge = APIBridge(manager: VPNManager.shared)
        contentController.add(apiBridge, name: "shuttleBridge")
        let bridgeBootstrap = WKUserScript(
            source: shuttleBridgeBootstrapJS,
            injectionTime: .atDocumentStart,
            forMainFrameOnly: true
        )
        contentController.addUserScript(bridgeBootstrap)

        // Fallback safety net: SPA posts here when the bridge probe fails.
        // Phase γ (Task 6.4) removes this once TestFlight is green.
        fallbackHandler = FallbackHandler(inlineHTML: createVPNControlHTML())
        contentController.add(fallbackHandler, name: "fallback")

        // Inject native bridge
        let bridgeScript = WKUserScript(
            source: """
                window.isIOS = true;
                window.ShuttleVPN = {
                    isRunning: function() {
                        return new Promise(function(resolve) {
                            window.webkit.messageHandlers.shuttleNative.postMessage(JSON.stringify({action:'isRunning'}));
                            window._vpnStatusCallback = resolve;
                        });
                    },
                    start: function() {
                        window.webkit.messageHandlers.shuttleNative.postMessage(JSON.stringify({action:'start'}));
                    },
                    stop: function() {
                        window.webkit.messageHandlers.shuttleNative.postMessage(JSON.stringify({action:'stop'}));
                    },
                    getStatus: function() {
                        return new Promise(function(resolve) {
                            window.webkit.messageHandlers.shuttleNative.postMessage(JSON.stringify({action:'status'}));
                            window._statusCallback = resolve;
                        });
                    },
                    invoke: function(msg) {
                        window.webkit.messageHandlers.shuttleNative.postMessage(msg);
                    }
                };
            """,
            injectionTime: .atDocumentStart,
            forMainFrameOnly: true
        )
        contentController.addUserScript(bridgeScript)

        let config = WKWebViewConfiguration()
        config.allowsInlineMediaPlayback = true
        config.userContentController = contentController

        webView = WKWebView(frame: view.bounds, configuration: config)
        webView.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        webView.navigationDelegate = self
        view.addSubview(webView)
        apiBridge.webView = webView
        fallbackHandler.webView = webView
    }

    private func loadConfig() {
        guard let configURL = Bundle.main.url(forResource: "config", withExtension: "json"),
              let configData = try? String(contentsOf: configURL) else {
            print("Failed to load config.json from bundle")
            return
        }
        self.configData = configData

        // FORCE_VPN_MODE — XCUITest hook that bypasses config-driven mode
        // selection so testSPALoadsInVPNMode can exercise the VPN path on a
        // fresh simulator without crafting a tun-enabled config.
        if ProcessInfo.processInfo.environment["FORCE_VPN_MODE"] == "1" {
            useVPN = true
            setupVPN()
            return
        }

        // Check if VPN mode is enabled
        if let data = configData.data(using: .utf8),
           let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
           let proxy = json["proxy"] as? [String: Any],
           let tun = proxy["tun"] as? [String: Any],
           let tunEnabled = tun["enabled"] as? Bool, tunEnabled {
            useVPN = true
            setupVPN()
        } else {
            startProxyMode()
        }
    }

    private func setupVPN() {
        VPNManager.shared.loadManager { [weak self] error in
            if let error = error {
                print("Failed to load VPN manager: \(error)")
                // Fall back to proxy mode
                self?.startProxyMode()
                return
            }

            guard let configData = self?.configData else { return }

            VPNManager.shared.configure(config: configData) { error in
                if let error = error {
                    print("Failed to configure VPN: \(error)")
                }
                // Load WebView - VPN will be started via JavaScript bridge
                self?.loadWebView()
            }
        }
    }

    private func startProxyMode() {
        DispatchQueue.global(qos: .userInitiated).async { [weak self] in
            guard let configData = self?.configData else { return }

            var error: NSError?
            let addr = MobileStart(configData, &error)
            if let error = error {
                print("Engine start failed: \(error)")
                return
            }

            self?.apiAddr = addr

            DispatchQueue.main.async {
                self?.loadWebView()
            }
        }
    }

    private func loadWebView() {
        if let apiAddr = apiAddr, let url = URL(string: "http://\(apiAddr)") {
            webView.load(URLRequest(url: url))
        } else if useVPN {
            // VPN mode: load the bundled SPA. The build/scripts/build-ios.sh
            // copies gui/web/dist/* into Shuttle/www/. SPA boot probes
            // window.ShuttleBridge healthz; on failure it falls back via
            // FallbackHandler (Task 5.5).
            if let spaURL = Bundle.main.url(forResource: "index", withExtension: "html", subdirectory: "www") {
                webView.loadFileURL(spaURL, allowingReadAccessTo: spaURL.deletingLastPathComponent())
            } else {
                // Bundle missing the SPA — keep the inline HTML as a last resort.
                os_log("SPA bundle missing — falling back to inline HTML",
                       log: OSLog(subsystem: "com.shuttle.app", category: "VC"),
                       type: .error)
                webView.loadHTMLString(createVPNControlHTML(), baseURL: nil)
            }
        }
    }

    private func createVPNControlHTML() -> String {
        return """
        <!DOCTYPE html>
        <html>
        <head>
            <meta name="viewport" content="width=device-width, initial-scale=1">
            <style>
                body { font-family: -apple-system; background: #0f1117; color: #e1e4e8; text-align: center; padding: 40px 20px; }
                h1 { font-size: 24px; margin-bottom: 30px; }
                .status { font-size: 14px; color: #8b949e; margin-bottom: 20px; }
                .btn { background: #238636; color: #fff; border: none; padding: 15px 40px; border-radius: 8px; font-size: 16px; }
                .btn.stop { background: #f85149; }
            </style>
        </head>
        <body>
            <h1>Shuttle VPN</h1>
            <p class="status" id="status">Disconnected</p>
            <button class="btn" id="toggleBtn" onclick="toggleVPN()">Connect</button>
            <script>
                function updateUI(connected) {
                    document.getElementById('status').textContent = connected ? 'Connected' : 'Disconnected';
                    var btn = document.getElementById('toggleBtn');
                    btn.textContent = connected ? 'Disconnect' : 'Connect';
                    btn.className = connected ? 'btn stop' : 'btn';
                }
                function toggleVPN() {
                    ShuttleVPN.isRunning().then(function(running) {
                        if (running) {
                            ShuttleVPN.stop();
                        } else {
                            ShuttleVPN.start();
                        }
                    });
                }
                // Check initial status
                if (window.ShuttleVPN) {
                    ShuttleVPN.isRunning().then(updateUI);
                }
            </script>
        </body>
        </html>
        """
    }

    @objc private func vpnStatusChanged(_ notification: Notification) {
        guard let status = notification.object as? NEVPNStatus else { return }
        let connected = status == .connected

        // Update WebView
        webView.evaluateJavaScript("if(window.updateUI) updateUI(\(connected));", completionHandler: nil)
        webView.evaluateJavaScript("if(window._vpnStatusCallback) { window._vpnStatusCallback(\(connected)); window._vpnStatusCallback = null; }", completionHandler: nil)
    }

    // MARK: - WKScriptMessageHandler

    func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
        // Legacy Phase 1 path: body is already parsed by WebKit into a dict
        // via `postMessage({action: '...'})`.
        if let body = message.body as? [String: Any],
           let action = body["action"] as? String {
            handleLegacyAction(action)
            return
        }

        // New Phase 4 path: body is a JSON string with {id, action, payload}
        // posted via window.ShuttleVPN.invoke().
        guard let body = message.body as? String,
              let data = body.data(using: .utf8),
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any]
        else { return }

        // A JSON string with only {action: 'x'} (no id) is a legacy path that
        // now funnels through invoke() for consistency with Android.
        if json["id"] == nil {
            if let action = json["action"] as? String {
                handleLegacyAction(action)
            }
            return
        }

        guard let id = json["id"] as? Int,
              let action = json["action"] as? String
        else { return }

        let payload = json["payload"] as? [String: Any]

        switch action {
        case "requestPermission":
            requestPermission(id: id)
        case "scanQR":
            scanQR(id: id)
        case "share":
            share(id: id, payload: payload ?? [:])
        case "openExternal":
            if let url = payload?["url"] as? String {
                openExternal(id: id, url: url)
            } else {
                reject(id, "no url")
            }
        case "subscribeStatus":
            reject(id, "subscribeStatus not implemented yet")
        default:
            reject(id, "unknown action: \(action)")
        }
    }

    private func handleLegacyAction(_ action: String) {
        switch action {
        case "isRunning":
            let running = VPNManager.shared.isConnected
            webView.evaluateJavaScript(
                "if(window._vpnStatusCallback) { window._vpnStatusCallback(\(running)); window._vpnStatusCallback = null; }",
                completionHandler: nil
            )
        case "start":
            VPNManager.shared.connect { error in
                if let error = error { print("VPN connect failed: \(error)") }
            }
        case "stop":
            VPNManager.shared.disconnect()
        case "status":
            let status = MobileStatus()
            let escaped = status.replacingOccurrences(of: "'", with: "\\'")
            webView.evaluateJavaScript(
                "if(window._statusCallback) { window._statusCallback('\(escaped)'); window._statusCallback = null; }",
                completionHandler: nil
            )
        default:
            break
        }
    }

    // MARK: - Phase 4 action handlers

    private func requestPermission(id: Int) {
        // iOS shows the "Shuttle would like to add VPN Configurations" prompt
        // when NETunnelProviderManager.saveToPreferences is called on a manager
        // with a valid protocolConfiguration. A bare NEVPNManager.shared()
        // save fails with NEVPNErrorConfigurationInvalid because it has no
        // protocol, so we go through the tunnel-provider path used by the
        // rest of the app.
        NETunnelProviderManager.loadAllFromPreferences { [weak self] managers, loadErr in
            guard let self = self else { return }
            if let loadErr = loadErr {
                self.reject(id, loadErr.localizedDescription)
                return
            }

            let manager = managers?.first ?? NETunnelProviderManager()

            // Ensure a minimal protocol is attached. When the user has already
            // granted permission + configured the tunnel elsewhere, we keep
            // the existing protocolConfiguration intact.
            if manager.protocolConfiguration == nil {
                let proto = NETunnelProviderProtocol()
                proto.providerBundleIdentifier = "com.shuttle.app.extension"
                proto.serverAddress = "Shuttle"
                manager.protocolConfiguration = proto
                manager.localizedDescription = "Shuttle VPN"
            }
            manager.isEnabled = true

            manager.saveToPreferences { [weak self] saveErr in
                if let saveErr = saveErr {
                    // iOS NEVPNError.configurationReadWriteFailed / .permissionDenied
                    // both surface as generic errors here. Treat any failure as
                    // "denied" so the SPA toggles back off rather than hanging.
                    self?.resolve(id, "\"denied\"")
                    print("requestPermission save failed: \(saveErr)")
                } else {
                    self?.resolve(id, "\"granted\"")
                }
            }
        }
    }

    private func scanQR(id: Int) {
        // Request camera permission, then present the scanner on the main queue.
        AVCaptureDevice.requestAccess(for: .video) { [weak self] granted in
            DispatchQueue.main.async {
                guard granted else {
                    self?.reject(id, "camera permission denied")
                    return
                }
                let vc = QrScannerViewController { [weak self] code in
                    if let code = code, !code.isEmpty {
                        let escaped = code
                            .replacingOccurrences(of: "\\", with: "\\\\")
                            .replacingOccurrences(of: "\"", with: "\\\"")
                        self?.resolve(id, "\"\(escaped)\"")
                    } else {
                        self?.reject(id, "qr scan cancelled")
                    }
                }
                self?.present(vc, animated: true)
            }
        }
    }

    private func share(id: Int, payload: [String: Any]) {
        var items: [Any] = []
        if let url = payload["url"] as? String, let u = URL(string: url) { items.append(u) }
        else if let text = payload["text"] as? String { items.append(text) }
        else if let title = payload["title"] as? String { items.append(title) }
        guard !items.isEmpty else { reject(id, "empty share payload"); return }

        let vc = UIActivityViewController(activityItems: items, applicationActivities: nil)
        vc.completionWithItemsHandler = { [weak self] _, completed, _, error in
            if let error = error {
                self?.reject(id, error.localizedDescription)
            } else {
                self?.resolve(id, completed ? "\"ok\"" : "\"cancelled\"")
            }
        }
        present(vc, animated: true)
    }

    private func openExternal(id: Int, url: String) {
        guard let u = URL(string: url) else {
            reject(id, "invalid url")
            return
        }
        UIApplication.shared.open(u, options: [:]) { [weak self] success in
            if success { self?.resolve(id, "\"ok\"") }
            else       { self?.reject(id, "open failed") }
        }
    }

    /// Resolves a pending JS Promise. `jsonValue` must be valid JSON
    /// (quoted string, number, bool, null, object, array).
    private func resolve(_ id: Int, _ jsonValue: String) {
        webView.evaluateJavaScript(
            "window._shuttleResolve && window._shuttleResolve(\(id), \(jsonValue));",
            completionHandler: nil
        )
    }

    private func reject(_ id: Int, _ message: String) {
        let escaped = message
            .replacingOccurrences(of: "\\", with: "\\\\")
            .replacingOccurrences(of: "\"", with: "\\\"")
        webView.evaluateJavaScript(
            "window._shuttleReject && window._shuttleReject(\(id), \"\(escaped)\");",
            completionHandler: nil
        )
    }

    deinit {
        NotificationCenter.default.removeObserver(self)
        if !useVPN {
            var error: NSError?
            MobileStop(&error)
        }
    }
}

@main
class AppDelegate: UIResponder, UIApplicationDelegate {
    var window: UIWindow?

    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
    ) -> Bool {
        window = UIWindow(frame: UIScreen.main.bounds)
        window?.rootViewController = ViewController()
        window?.makeKeyAndVisible()
        return true
    }
}
