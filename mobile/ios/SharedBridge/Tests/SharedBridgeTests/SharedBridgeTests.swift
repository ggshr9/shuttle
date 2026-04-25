import XCTest
@testable import SharedBridge

final class SharedBridgeTests: XCTestCase {

    func testAPIRequest_RoundTrip() throws {
        let req = APIRequest(
            method: "GET",
            path: "/api/x",
            headers: ["A": "B", "Content-Type": "application/json"],
            body: "ZGF0YQ=="
        )
        let data = try JSONEncoder().encode(req)
        let decoded = try JSONDecoder().decode(APIRequest.self, from: data)
        XCTAssertEqual(decoded, req)
    }

    func testAPIRequest_NilBody_OmittedInJSON() throws {
        // The Codable default emits "body":null when nil. Make sure round-trip
        // preserves nil rather than empty string.
        let req = APIRequest(method: "GET", path: "/x", headers: [:], body: nil)
        let data = try JSONEncoder().encode(req)
        let decoded = try JSONDecoder().decode(APIRequest.self, from: data)
        XCTAssertNil(decoded.body)
    }

    func testAPIResponse_RoundTrip() throws {
        let res = APIResponse(status: 200, headers: ["x": "y"], body: "Zm9v", error: nil)
        let data = try JSONEncoder().encode(res)
        let decoded = try JSONDecoder().decode(APIResponse.self, from: data)
        XCTAssertEqual(decoded, res)
    }

    func testAPIResponse_TransportError_HasStatusMinusOne() {
        let res = APIResponse.transportError("IPC timeout")
        XCTAssertEqual(res.status, -1)
        XCTAssertEqual(res.error, "IPC timeout")
        XCTAssertEqual(res.body, "")
    }

    func testAPIResponse_EngineNotReady_HasStatus503() {
        let res = APIResponse.engineNotReady()
        XCTAssertEqual(res.status, 503)
        XCTAssertEqual(res.error, "engine not ready")
    }
}
