package engine

import "time"

// EventType identifies the kind of engine event.
type EventType int

const (
	EventConnected        EventType = iota // Engine connected to server
	EventDisconnected                      // Engine disconnected
	EventSpeedTick                         // Periodic speed update
	EventLog                               // Log message
	EventTransportChanged                  // Active transport switched
	EventError                             // Non-fatal error
)

// Event represents a real-time engine event.
type Event struct {
	Type      EventType `json:"type"`
	Timestamp time.Time `json:"timestamp"`

	// SpeedTick fields
	Upload   int64 `json:"upload,omitempty"`   // bytes/sec
	Download int64 `json:"download,omitempty"` // bytes/sec

	// Log fields
	Level   string `json:"level,omitempty"`
	Message string `json:"message,omitempty"`

	// TransportChanged fields
	Transport string `json:"transport,omitempty"`

	// Error fields
	Error string `json:"error,omitempty"`
}

// EngineState represents the engine's current state.
type EngineState int

const (
	StateStopped    EngineState = iota
	StateStarting
	StateRunning
	StateStopping
)

func (s EngineState) String() string {
	switch s {
	case StateStopped:
		return "stopped"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	default:
		return "unknown"
	}
}

// EngineStatus is a snapshot of engine state for the API.
type EngineStatus struct {
	State         string         `json:"state"`
	ActiveConns   int64          `json:"active_conns"`
	TotalConns    int64          `json:"total_conns"`
	BytesSent     int64          `json:"bytes_sent"`
	BytesReceived int64          `json:"bytes_received"`
	UploadSpeed   int64          `json:"upload_speed"`
	DownloadSpeed int64          `json:"download_speed"`
	Transport     string         `json:"transport"`
	Transports    []TransportInfo `json:"transports"`
}

// TransportInfo describes a transport's health.
type TransportInfo struct {
	Type      string `json:"type"`
	Available bool   `json:"available"`
	Latency   int64  `json:"latency_ms"`
}
