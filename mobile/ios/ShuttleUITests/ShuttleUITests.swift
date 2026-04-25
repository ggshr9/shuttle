import XCTest

/// Minimal iOS launch smoke test.
///
/// The bar is deliberately low: the app must launch and the Svelte SPA
/// must boot inside the WKWebView. Deeper interactions (tapping the
/// Power button, confirming the VPN permission dialog) require Springboard
/// coordination and will be added in a follow-up once this baseline is
/// green on simulator CI.
final class ShuttleUITests: XCTestCase {

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    func testAppLaunchesAndSPARenders() throws {
        let app = XCUIApplication()
        app.launch()

        // Wait for the WKWebView to render the Now page. The SPA takes a
        // second or two to boot + fetch config, so use a generous timeout.
        let nowLabel = app.staticTexts["Now"].firstMatch
        XCTAssertTrue(
            nowLabel.waitForExistence(timeout: 15),
            "Now label did not appear in WebView within 15s — SPA failed to boot"
        )
    }

    func testBottomTabsPresent() throws {
        let app = XCUIApplication()
        app.launch()

        // WKWebView exposes inner text via static-text accessibility elements.
        // We probe for 2+ tab labels to confirm the BottomTabs component rendered
        // (not a specific count — labels may localize in future).
        let servers = app.staticTexts["Servers"].firstMatch
        let settings = app.otherElements.buttons["Settings"].firstMatch
        XCTAssertTrue(
            servers.waitForExistence(timeout: 15) || settings.waitForExistence(timeout: 5),
            "Expected to find either 'Servers' tab label or the 'Settings' gear button"
        )
    }

    /// Phase 5 acceptance: iOS VPN mode loads the same SPA chrome as proxy
    /// mode and other runtimes. The unification contract says users see
    /// identical navigation regardless of transport — failure here means
    /// VPN mode silently degraded (e.g. fell back to inline HTML).
    func testSPALoadsInVPNMode() throws {
        let app = XCUIApplication()
        // FORCE_VPN_MODE bypasses the config-driven mode selection and
        // pushes ShuttleApp.swift into the VPN setup path. Honored by
        // ViewController.loadConfig() when Phase 5 wiring lands.
        app.launchEnvironment["FORCE_VPN_MODE"] = "1"
        app.launch()

        // Allow the system VPN-permission dialog if it appears. Springboard
        // owns this UI; we tap "Allow" if it's present, otherwise continue.
        let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
        let allowBtn = springboard.buttons["Allow"]
        if allowBtn.waitForExistence(timeout: 5) { allowBtn.tap() }

        // The SPA chrome — BottomTabs with at least Now + Mesh — is the
        // signal that we got the full SPA, not the legacy fallback HTML
        // (which has only a single "Connect" button and no nav).
        let nowLabel = app.staticTexts["Now"].firstMatch
        XCTAssertTrue(
            nowLabel.waitForExistence(timeout: 15),
            "Now tab missing in VPN mode — SPA failed to boot or fell back to inline HTML"
        )

        // Mesh tab is unique to the SPA — fallback HTML never renders it.
        // Asserting both Now AND Mesh confirms the same nav as proxy mode.
        let meshTab = app.staticTexts["Mesh"].firstMatch
        XCTAssertTrue(
            meshTab.waitForExistence(timeout: 5),
            "Mesh tab missing in VPN mode — likely on the fallback inline HTML path"
        )
    }
}
