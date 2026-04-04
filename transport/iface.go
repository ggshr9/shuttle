package transport

import "github.com/shuttleX/shuttle/adapter"

// Type aliases bridging transport types to adapter types.
// All new code should import adapter directly.
// These aliases exist for backward compatibility during migration.
type (
	Stream          = adapter.Stream
	Connection      = adapter.Connection
	ClientTransport = adapter.ClientTransport
	ServerTransport = adapter.ServerTransport
)

// TransportConfig holds common transport configuration.
type TransportConfig struct {
	ServerAddr   string
	ServerName   string
	Password     string
	InsecureSkip bool
}
