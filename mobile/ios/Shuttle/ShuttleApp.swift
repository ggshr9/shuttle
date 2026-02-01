import UIKit
import WebKit

/// Main view controller that hosts the Shuttle SPA via WKWebView.
///
/// The Go engine + API server runs in-process via gomobile bindings.
/// The WKWebView loads http://127.0.0.1:{port} to render the shared Svelte UI.
///
/// Build steps:
/// 1. gomobile bind -target=ios -o Shuttle.xcframework ../../mobile
/// 2. Add Shuttle.xcframework to Xcode project
/// 3. Build with Xcode
class ViewController: UIViewController, WKNavigationDelegate {

    private var webView: WKWebView!
    private var apiAddr: String?

    override func viewDidLoad() {
        super.viewDidLoad()

        let config = WKWebViewConfiguration()
        config.allowsInlineMediaPlayback = true

        webView = WKWebView(frame: view.bounds, configuration: config)
        webView.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        webView.navigationDelegate = self
        view.addSubview(webView)

        // Start engine on background thread
        DispatchQueue.global(qos: .userInitiated).async { [weak self] in
            do {
                // Load config from bundle
                guard let configURL = Bundle.main.url(forResource: "config", withExtension: "json"),
                      let configData = try? String(contentsOf: configURL) else {
                    print("Failed to load config.json from bundle")
                    return
                }

                var error: NSError?
                let addr = MobileStart(configData, &error)
                if let error = error {
                    print("Engine start failed: \(error)")
                    return
                }

                self?.apiAddr = addr

                DispatchQueue.main.async {
                    if let addr = addr, let url = URL(string: "http://\(addr)") {
                        self?.webView.load(URLRequest(url: url))
                    }
                }
            }
        }
    }

    deinit {
        var error: NSError?
        MobileStop(&error)
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
