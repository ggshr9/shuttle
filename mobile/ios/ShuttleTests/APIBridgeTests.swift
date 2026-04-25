import XCTest
import Foundation
@testable import Shuttle

/// Unit tests for APIBridge envelope-forwarding behaviour.
///
/// WKScriptMessage is final with a private constructor, so tests bypass
/// `userContentController(_:didReceive:)` and call the internal
/// `handle(id:envelopeAny:)` seam directly. A stub `EnvelopeSender` closure
/// replaces the real VPNManager, keeping these tests host-safe.
final class APIBridgeTests: XCTestCase {

    // MARK: - Helpers

    /// Minimal valid envelope that JSONSerialization can round-trip.
    private func validEnvelope(method: String = "GET", path: String = "/api/healthz") -> [String: Any] {
        return [
            "method": method,
            "path": path,
            "headers": [String: String](),
        ]
    }

    // MARK: - Tests

    /// The bridge must serialise the envelope dict into Data and pass it to the
    /// send closure without modification.
    func testHandle_ForwardsEnvelopeBytes() throws {
        var capturedData: Data?

        let bridge = APIBridge(send: { data, _, completion in
            capturedData = data
            // Synthesise a stub APIResponse so completeJS fires.
            // webView is nil so evaluateJavaScript is a no-op, which is fine.
            let resp = APIResponse(status: 200, headers: [:], body: "Zm9v", error: nil)
            let respData = try? JSONEncoder().encode(resp)
            completion(respData)
        })

        bridge.handle(id: 42, envelopeAny: validEnvelope())

        XCTAssertNotNil(capturedData, "Bridge did not forward envelope to send closure")

        let decoded = try JSONDecoder().decode(APIRequest.self, from: capturedData!)
        XCTAssertEqual(decoded.method, "GET")
        XCTAssertEqual(decoded.path, "/api/healthz")
    }

    /// When the send closure calls completion(nil) — simulating an IPC timeout —
    /// the bridge must still invoke the closure (not silently drop it) and
    /// gracefully handle the nil response (failJS path). Without a webView the
    /// failJS call is a no-op, so we verify the closure itself was invoked.
    func testHandle_NilResponseInvokesSendClosure() {
        let exp = expectation(description: "send closure invoked")

        let bridge = APIBridge(send: { _, _, completion in
            exp.fulfill()
            completion(nil) // simulate IPC timeout
        })

        bridge.handle(id: 1, envelopeAny: validEnvelope(path: "/x"))

        wait(for: [exp], timeout: 1.0)
    }

    /// When the envelope dict contains a value that JSONSerialization cannot
    /// serialise (e.g. Date), the bridge must call failJS and must NOT invoke
    /// the send closure.
    func testHandle_InvalidEnvelopeJSON_DoesNotCallSend() {
        var sendCalled = false

        let bridge = APIBridge(send: { _, _, _ in
            sendCalled = true
        })

        // Date is not directly serialisable by JSONSerialization (it lacks the
        // .convertToISO8601 / .fragmentsAllowed that JSONEncoder uses).
        var badEnvelope = validEnvelope()
        badEnvelope["weird"] = Date()

        bridge.handle(id: 1, envelopeAny: badEnvelope)

        XCTAssertFalse(sendCalled, "send should not be called when envelope cannot be serialised")
    }

    /// The timeout value forwarded to the send closure must match the expected
    /// 30-second constant hard-coded in APIBridge.
    func testHandle_ForwardsCorrectTimeout() {
        var capturedTimeout: TimeInterval = 0

        let bridge = APIBridge(send: { _, timeout, completion in
            capturedTimeout = timeout
            completion(nil)
        })

        bridge.handle(id: 2, envelopeAny: validEnvelope())

        XCTAssertEqual(capturedTimeout, 30, "Bridge should forward a 30-second timeout")
    }

    /// The convenience init(manager:) must be usable without crashing (smoke
    /// test). The real VPNManager singleton is constructed; we do not start or
    /// stop the VPN — just confirm the bridge is non-nil.
    func testConvenienceInit_DoesNotCrash() {
        // VPNManager.shared is a singleton — safe to reference without network.
        let bridge = APIBridge(manager: VPNManager.shared)
        XCTAssertNotNil(bridge)
    }
}
