import NetworkExtension
import os.log

/// PacketTunnelProvider implements the iOS Network Extension for VPN functionality.
///
/// This provider creates a TUN device and routes traffic through the Shuttle proxy engine.
/// The Go engine runs within this extension process via gomobile bindings.
///
/// Setup requirements:
/// 1. Add Network Extension capability to main app and extension
/// 2. Create App Group for shared data
/// 3. Add packet-tunnel-provider to extension's Info.plist
class PacketTunnelProvider: NEPacketTunnelProvider {

    private let log = OSLog(subsystem: "com.shuttle.app.extension", category: "tunnel")
    private var tunnelFD: Int32 = -1
    private var apiAddr: String?

    override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        os_log("Starting tunnel", log: log, type: .info)

        // Load config from App Group shared container or options
        guard let config = loadConfig(options: options) else {
            let error = NSError(domain: "ShuttleTunnel", code: 1,
                              userInfo: [NSLocalizedDescriptionKey: "Failed to load configuration"])
            completionHandler(error)
            return
        }

        // Configure tunnel network settings
        let networkSettings = createNetworkSettings()

        setTunnelNetworkSettings(networkSettings) { [weak self] error in
            guard let self = self else { return }

            if let error = error {
                os_log("Failed to set network settings: %{public}@", log: self.log, type: .error, error.localizedDescription)
                completionHandler(error)
                return
            }

            // Get the TUN file descriptor
            // Note: On iOS, the packetFlow is used instead of raw fd
            // We'll use the packetFlow for reading/writing packets

            // Start the Go engine
            DispatchQueue.global(qos: .userInitiated).async {
                do {
                    try self.startEngine(config: config)
                    os_log("Engine started successfully", log: self.log, type: .info)

                    // Start packet processing loop
                    self.startPacketLoop()

                    DispatchQueue.main.async {
                        completionHandler(nil)
                    }
                } catch {
                    os_log("Failed to start engine: %{public}@", log: self.log, type: .error, error.localizedDescription)
                    DispatchQueue.main.async {
                        completionHandler(error)
                    }
                }
            }
        }
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        os_log("Stopping tunnel, reason: %d", log: log, type: .info, reason.rawValue)

        // Stop the Go engine
        var error: NSError?
        MobileStop(&error)
        if let error = error {
            os_log("Error stopping engine: %{public}@", log: log, type: .error, error.localizedDescription)
        }

        completionHandler()
    }

    override func handleAppMessage(_ messageData: Data, completionHandler: ((Data?) -> Void)?) {
        // Handle messages from the main app (e.g., status queries, config updates)
        guard let message = String(data: messageData, encoding: .utf8) else {
            completionHandler?(nil)
            return
        }

        switch message {
        case "status":
            let status = MobileStatus()
            completionHandler?(status.data(using: .utf8))
        case "stop":
            stopTunnel(with: .userInitiated) {
                completionHandler?("stopped".data(using: .utf8))
            }
        default:
            completionHandler?(nil)
        }
    }

    // MARK: - Private Methods

    private func loadConfig(options: [String: NSObject]?) -> String? {
        // Try to get config from options (passed when starting VPN)
        if let configData = options?["config"] as? String {
            return configData
        }

        // Try to load from App Group shared container
        let appGroupID = "group.com.shuttle.app"
        if let containerURL = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: appGroupID) {
            let configURL = containerURL.appendingPathComponent("config.json")
            if let configData = try? String(contentsOf: configURL, encoding: .utf8) {
                return configData
            }
        }

        // Try to load from extension bundle
        if let bundleURL = Bundle.main.url(forResource: "config", withExtension: "json"),
           let configData = try? String(contentsOf: bundleURL, encoding: .utf8) {
            return configData
        }

        return nil
    }

    private func createNetworkSettings() -> NEPacketTunnelNetworkSettings {
        // Use the same IP configuration as Android
        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: "198.18.0.1")

        // IPv4 settings
        let ipv4Settings = NEIPv4Settings(addresses: ["198.18.0.1"], subnetMasks: ["255.255.0.0"])
        ipv4Settings.includedRoutes = [NEIPv4Route.default()]
        ipv4Settings.excludedRoutes = [
            // Exclude local networks
            NEIPv4Route(destinationAddress: "10.0.0.0", subnetMask: "255.0.0.0"),
            NEIPv4Route(destinationAddress: "172.16.0.0", subnetMask: "255.240.0.0"),
            NEIPv4Route(destinationAddress: "192.168.0.0", subnetMask: "255.255.0.0"),
        ]
        settings.ipv4Settings = ipv4Settings

        // DNS settings
        let dnsSettings = NEDNSSettings(servers: ["198.18.0.2"])
        dnsSettings.matchDomains = [""] // Match all domains
        settings.dnsSettings = dnsSettings

        // MTU
        settings.mtu = 1500

        return settings
    }

    private func startEngine(config: String) throws {
        var error: NSError?

        // For iOS, we use the standard start without TUN fd
        // The packetFlow will be used for packet I/O
        apiAddr = MobileStart(config, &error)

        if let error = error {
            throw error
        }

        os_log("API server started at %{public}@", log: log, type: .info, apiAddr ?? "unknown")
    }

    private func startPacketLoop() {
        // Read packets from the TUN and send to engine
        // This is a simplified implementation - production code would need proper packet handling

        packetFlow.readPackets { [weak self] packets, protocols in
            guard let self = self else { return }

            // Process packets through the engine
            for (index, packet) in packets.enumerated() {
                // In a full implementation, these packets would be sent to the Go engine
                // via a channel or pipe, processed through the proxy, and responses written back
                _ = protocols[index]
            }

            // Continue reading
            self.startPacketLoop()
        }
    }

    private func writePackets(_ packets: [Data], protocols: [NSNumber]) {
        packetFlow.writePackets(packets, withProtocols: protocols)
    }
}
