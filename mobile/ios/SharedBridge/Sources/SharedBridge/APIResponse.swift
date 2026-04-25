import Foundation

/// APIResponse is the reply envelope returned by the Network Extension to the
/// main app's APIBridge. status == -1 indicates a transport-level failure
/// (e.g. extension cannot reach its own loopback listener) and the JS-side
/// BridgeAdapter will surface this as TransportError. body is base64-encoded.
public struct APIResponse: Codable, Equatable {
    public let status: Int
    public let headers: [String: String]
    public let body: String         // base64-encoded response body
    public let error: String?

    public init(status: Int, headers: [String: String], body: String, error: String? = nil) {
        self.status = status
        self.headers = headers
        self.body = body
        self.error = error
    }

    /// Convenience constructor for a transport-level error — Go engine
    /// unreachable, IPC timeout, etc.
    public static func transportError(_ msg: String) -> APIResponse {
        APIResponse(status: -1, headers: [:], body: "", error: msg)
    }

    /// Convenience constructor for the early-startup case where MobileStart
    /// has not yet returned the apiAddr.
    public static func engineNotReady() -> APIResponse {
        APIResponse(status: 503, headers: [:], body: "", error: "engine not ready")
    }
}
