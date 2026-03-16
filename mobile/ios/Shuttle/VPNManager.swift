import NetworkExtension
import os.log

/// VPNManager handles the configuration and control of the Shuttle VPN tunnel.
class VPNManager {
    static let shared = VPNManager()

    private let log = OSLog(subsystem: "com.shuttle.app", category: "vpn")
    private var manager: NETunnelProviderManager?
    private var observers: [NSObjectProtocol] = []

    var isConnected: Bool {
        return manager?.connection.status == .connected
    }

    var status: NEVPNStatus {
        return manager?.connection.status ?? .invalid
    }

    private init() {
        setupObservers()
    }

    deinit {
        observers.forEach { NotificationCenter.default.removeObserver($0) }
    }

    // MARK: - Public Methods

    /// Load or create the VPN configuration
    func loadManager(completion: @escaping (Error?) -> Void) {
        NETunnelProviderManager.loadAllFromPreferences { [weak self] managers, error in
            guard let self = self else { return }

            if let error = error {
                os_log("Failed to load VPN managers: %{public}@", log: self.log, type: .error, error.localizedDescription)
                completion(error)
                return
            }

            // Use existing manager or create new one
            if let existing = managers?.first {
                self.manager = existing
                os_log("Loaded existing VPN configuration", log: self.log, type: .info)
            } else {
                self.manager = NETunnelProviderManager()
                os_log("Created new VPN configuration", log: self.log, type: .info)
            }

            completion(nil)
        }
    }

    /// Configure the VPN with the given config JSON
    func configure(config: String, completion: @escaping (Error?) -> Void) {
        guard let manager = manager else {
            let error = NSError(domain: "VPNManager", code: 1,
                              userInfo: [NSLocalizedDescriptionKey: "Manager not loaded"])
            completion(error)
            return
        }

        // Save config to App Group for extension access
        saveConfigToAppGroup(config)

        // Configure the tunnel provider
        let tunnelProtocol = NETunnelProviderProtocol()
        tunnelProtocol.providerBundleIdentifier = "com.shuttle.app.extension"
        tunnelProtocol.serverAddress = "Shuttle"
        tunnelProtocol.providerConfiguration = ["config": config]

        manager.protocolConfiguration = tunnelProtocol
        manager.localizedDescription = "Shuttle VPN"
        manager.isEnabled = true

        manager.saveToPreferences { error in
            if let error = error {
                os_log("Failed to save VPN configuration: %{public}@", log: self.log, type: .error, error.localizedDescription)
            } else {
                os_log("VPN configuration saved", log: self.log, type: .info)
            }
            completion(error)
        }
    }

    /// Start the VPN tunnel
    func connect(completion: @escaping (Error?) -> Void) {
        guard let manager = manager else {
            let error = NSError(domain: "VPNManager", code: 1,
                              userInfo: [NSLocalizedDescriptionKey: "Manager not configured"])
            completion(error)
            return
        }

        // Reload to get latest preferences
        manager.loadFromPreferences { error in
            if let error = error {
                completion(error)
                return
            }

            do {
                try manager.connection.startVPNTunnel()
                os_log("VPN tunnel starting", log: self.log, type: .info)
                completion(nil)
            } catch {
                os_log("Failed to start VPN tunnel: %{public}@", log: self.log, type: .error, error.localizedDescription)
                completion(error)
            }
        }
    }

    /// Stop the VPN tunnel
    func disconnect() {
        manager?.connection.stopVPNTunnel()
        os_log("VPN tunnel stopping", log: log, type: .info)
    }

    /// Send a message to the tunnel extension
    func sendMessage(_ message: String, completion: @escaping (String?) -> Void) {
        guard let session = manager?.connection as? NETunnelProviderSession,
              let data = message.data(using: .utf8) else {
            completion(nil)
            return
        }

        do {
            try session.sendProviderMessage(data) { response in
                if let response = response {
                    completion(String(data: response, encoding: .utf8))
                } else {
                    completion(nil)
                }
            }
        } catch {
            os_log("Failed to send message: %{public}@", log: log, type: .error, error.localizedDescription)
            completion(nil)
        }
    }

    // MARK: - Private Methods

    private func setupObservers() {
        let observer = NotificationCenter.default.addObserver(
            forName: .NEVPNStatusDidChange,
            object: nil,
            queue: .main
        ) { [weak self] notification in
            guard let self = self,
                  let connection = notification.object as? NEVPNConnection else { return }

            os_log("VPN status changed: %d", log: self.log, type: .info, connection.status.rawValue)

            // Post custom notification for UI updates
            NotificationCenter.default.post(name: .shuttleVPNStatusChanged, object: connection.status)
        }
        observers.append(observer)
    }

    private func saveConfigToAppGroup(_ config: String) {
        let appGroupID = "group.com.shuttle.app"
        guard let containerURL = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: appGroupID) else {
            os_log("Failed to get App Group container", log: log, type: .error)
            return
        }

        let configURL = containerURL.appendingPathComponent("config.json")
        do {
            try config.write(to: configURL, atomically: true, encoding: .utf8)
            os_log("Config saved to App Group", log: log, type: .info)
        } catch {
            os_log("Failed to save config: %{public}@", log: log, type: .error, error.localizedDescription)
        }
    }
}

// MARK: - Notifications

extension Notification.Name {
    static let shuttleVPNStatusChanged = Notification.Name("shuttleVPNStatusChanged")
}
