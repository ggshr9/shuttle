import NetworkExtension
import os.log

/// PacketTunnelProvider implements the iOS Network Extension for VPN functionality.
///
/// This provider creates a TUN device and routes traffic through the Shuttle proxy engine.
/// The Go engine runs within this extension process via gomobile bindings.
///
/// Packet flow:
/// 1. iOS sends outbound packets via packetFlow → we write them to the Go engine pipe
/// 2. Go engine processes packets through proxy, returns responses
/// 3. We read response packets from the Go engine pipe → write back via packetFlow
///
/// Setup requirements:
/// 1. Add Network Extension capability to main app and extension
/// 2. Create App Group for shared data
/// 3. Add packet-tunnel-provider to extension's Info.plist
class PacketTunnelProvider: NEPacketTunnelProvider {

    private let log = OSLog(subsystem: "com.shuttle.app.extension", category: "tunnel")
    private var apiAddr: String?
    private var isPacketLoopRunning = false
    private var packetReadQueue = DispatchQueue(label: "com.shuttle.packet.read", qos: .userInteractive)
    private var packetWriteQueue = DispatchQueue(label: "com.shuttle.packet.write", qos: .userInteractive)

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

            // Start the Go engine on a background queue
            DispatchQueue.global(qos: .userInitiated).async {
                do {
                    try self.startEngine(config: config)
                    os_log("Engine started successfully", log: self.log, type: .info)

                    // Start bidirectional packet processing
                    self.startPacketRelay()

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

        isPacketLoopRunning = false

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
        case "logs":
            let logs = MobileGetRecentLogs(50)
            completionHandler?(logs.data(using: .utf8))
        default:
            // Try to handle as a config reload
            if message.hasPrefix("{") {
                var reloadErr: NSError?
                MobileReload(message, &reloadErr)
                if let err = reloadErr {
                    completionHandler?(err.localizedDescription.data(using: .utf8))
                } else {
                    completionHandler?("reloaded".data(using: .utf8))
                }
            } else {
                completionHandler?(nil)
            }
        }
    }

    override func sleep(completionHandler: @escaping () -> Void) {
        // System is going to sleep — pause keepalive to conserve battery
        os_log("Extension entering sleep", log: log, type: .info)
        completionHandler()
    }

    override func wake() {
        // System woke up — the network monitor in the Go engine will detect
        // any interface changes and trigger auto-reconnect if enabled
        os_log("Extension waking up", log: log, type: .info)
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
        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: "198.18.0.1")

        // IPv4 settings — route everything through the tunnel
        let ipv4Settings = NEIPv4Settings(addresses: ["198.18.0.1"], subnetMasks: ["255.255.0.0"])
        ipv4Settings.includedRoutes = [NEIPv4Route.default()]
        ipv4Settings.excludedRoutes = [
            // Exclude local/private networks to prevent routing loops
            NEIPv4Route(destinationAddress: "10.0.0.0", subnetMask: "255.0.0.0"),
            NEIPv4Route(destinationAddress: "172.16.0.0", subnetMask: "255.240.0.0"),
            NEIPv4Route(destinationAddress: "192.168.0.0", subnetMask: "255.255.0.0"),
            NEIPv4Route(destinationAddress: "127.0.0.0", subnetMask: "255.0.0.0"),
        ]
        settings.ipv4Settings = ipv4Settings

        // DNS settings — capture all DNS queries
        let dnsSettings = NEDNSSettings(servers: ["198.18.0.2"])
        dnsSettings.matchDomains = [""] // Match all domains
        settings.dnsSettings = dnsSettings

        // MTU
        settings.mtu = 1500

        return settings
    }

    private func startEngine(config: String) throws {
        var error: NSError?

        // Start the Go engine in proxy mode (SOCKS5/HTTP)
        // iOS packetFlow doesn't expose a raw fd, so we run the engine
        // and relay packets via the SOCKS5 proxy internally
        apiAddr = MobileStart(config, &error)

        if let error = error {
            throw error
        }

        // Enable auto-reconnect for mobile network handoffs
        MobileSetAutoReconnect(true)

        os_log("API server started at %{public}@", log: log, type: .info, apiAddr ?? "unknown")
    }

    /// Starts the bidirectional packet relay between iOS packetFlow and the Go engine.
    ///
    /// Outbound: Reads IP packets from packetFlow, forwards to Go engine via MobileWritePacket.
    /// Inbound: Reads processed packets from Go engine via MobileReadPacket, writes to packetFlow.
    private func startPacketRelay() {
        isPacketLoopRunning = true

        // Outbound: iOS → Go engine
        startOutboundRelay()

        // Inbound: Go engine → iOS
        startInboundRelay()
    }

    /// Reads packets from the iOS TUN and sends them to the Go engine for processing.
    private func startOutboundRelay() {
        packetFlow.readPackets { [weak self] packets, protocols in
            guard let self = self, self.isPacketLoopRunning else { return }

            for (index, packet) in packets.enumerated() {
                let proto = protocols[index].int32Value
                // Send packet to Go engine for proxying
                MobileWritePacket(packet, proto)
            }

            // Continue reading — packetFlow.readPackets is one-shot
            self.startOutboundRelay()
        }
    }

    /// Reads processed packets from the Go engine and writes them back to the iOS TUN.
    private func startInboundRelay() {
        packetWriteQueue.async { [weak self] in
            while let self = self, self.isPacketLoopRunning {
                // MobileReadPacket blocks until a packet is available or engine stops
                let result = MobileReadPacket()
                guard let data = result?.data, !data.isEmpty else {
                    // Engine stopped or no packet — exit loop
                    if self.isPacketLoopRunning {
                        // Brief yield before retry to avoid busy-spin
                        Thread.sleep(forTimeInterval: 0.001)
                        continue
                    }
                    break
                }

                // IPv4 = AF_INET (2), IPv6 = AF_INET6 (30 on Darwin)
                let proto: NSNumber = (result?.proto == 6) ? NSNumber(value: AF_INET6) : NSNumber(value: AF_INET)
                self.packetFlow.writePackets([data], withProtocols: [proto])
            }
        }
    }
}
