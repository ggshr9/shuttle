import UIKit
import WebKit
import NetworkExtension

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

        // Inject native bridge
        let bridgeScript = WKUserScript(
            source: """
                window.isIOS = true;
                window.ShuttleVPN = {
                    isRunning: function() {
                        return new Promise(function(resolve) {
                            window.webkit.messageHandlers.shuttleNative.postMessage({action: 'isRunning'});
                            window._vpnStatusCallback = resolve;
                        });
                    },
                    start: function() {
                        window.webkit.messageHandlers.shuttleNative.postMessage({action: 'start'});
                    },
                    stop: function() {
                        window.webkit.messageHandlers.shuttleNative.postMessage({action: 'stop'});
                    },
                    getStatus: function() {
                        return new Promise(function(resolve) {
                            window.webkit.messageHandlers.shuttleNative.postMessage({action: 'status'});
                            window._statusCallback = resolve;
                        });
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
    }

    private func loadConfig() {
        guard let configURL = Bundle.main.url(forResource: "config", withExtension: "json"),
              let configData = try? String(contentsOf: configURL) else {
            print("Failed to load config.json from bundle")
            return
        }
        self.configData = configData

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
            // For VPN mode, show a simple native UI or embedded HTML
            webView.loadHTMLString(createVPNControlHTML(), baseURL: nil)
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
        guard let body = message.body as? [String: Any],
              let action = body["action"] as? String else { return }

        switch action {
        case "isRunning":
            let running = VPNManager.shared.isConnected
            webView.evaluateJavaScript("if(window._vpnStatusCallback) { window._vpnStatusCallback(\(running)); window._vpnStatusCallback = null; }", completionHandler: nil)

        case "start":
            VPNManager.shared.connect { error in
                if let error = error {
                    print("VPN connect failed: \(error)")
                }
            }

        case "stop":
            VPNManager.shared.disconnect()

        case "status":
            let status = MobileStatus()
            webView.evaluateJavaScript("if(window._statusCallback) { window._statusCallback('\(status.replacingOccurrences(of: "'", with: "\\'"))'); window._statusCallback = null; }", completionHandler: nil)

        default:
            break
        }
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
