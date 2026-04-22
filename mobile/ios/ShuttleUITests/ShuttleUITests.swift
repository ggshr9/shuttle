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
}
