import Foundation

/// APIRequest is the on-the-wire envelope sent from the main iOS app to the
/// Network Extension via NETunnelProviderSession.sendProviderMessage. The
/// extension decodes it, makes an HTTP call to its in-extension Go engine
/// listener at 127.0.0.1:apiAddr, and returns an APIResponse.
public struct APIRequest: Codable, Equatable {
    public let method: String
    public let path: String
    public let headers: [String: String]
    public let body: String?    // base64-encoded request body, nil for GET/DELETE typically

    public init(method: String, path: String, headers: [String: String], body: String? = nil) {
        self.method = method
        self.path = path
        self.headers = headers
        self.body = body
    }
}
