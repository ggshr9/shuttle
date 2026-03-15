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
	EventConnection                        // Connection opened/closed
	EventNetworkChange                     // Network interface change detected
	EventConnectionError                   // Transport connection failed (circuit breaker)
)

var eventTypeNames = [...]string{
	EventConnected:        "connected",
	EventDisconnected:     "disconnected",
	EventSpeedTick:        "speed_tick",
	EventLog:              "log",
	EventTransportChanged: "transport_changed",
	EventError:            "error",
	EventConnection:       "connection",
	EventNetworkChange:    "network_change",
	EventConnectionError:  "connection_error",
}

func (t EventType) String() string {
	if int(t) < len(eventTypeNames) {
		return eventTypeNames[t]
	}
	return "unknown"
}

func (t EventType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.String() + `"`), nil
}

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

	// ConnectionError fields (EventConnectionError)
	BackoffMs int64 `json:"backoff_ms,omitempty"` // Backoff duration in ms before next retry

	// Connection fields (EventConnection)
	ConnID      string `json:"conn_id,omitempty"`      // Unique connection identifier
	ConnState   string `json:"conn_state,omitempty"`   // "opened" or "closed"
	Target      string `json:"target,omitempty"`       // Destination address
	Rule        string `json:"rule,omitempty"`         // Matched routing rule
	Protocol    string `json:"protocol,omitempty"`     // tcp/udp
	BytesIn     int64  `json:"bytes_in,omitempty"`     // Bytes received
	BytesOut    int64  `json:"bytes_out,omitempty"`    // Bytes sent
	DurationMs  int64  `json:"duration_ms,omitempty"`  // Connection duration in ms
	ProcessName string `json:"process_name,omitempty"` // Source process name (if available)
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
	State          string          `json:"state"`
	ActiveConns    int64           `json:"active_conns"`
	TotalConns     int64           `json:"total_conns"`
	BytesSent      int64           `json:"bytes_sent"`
	BytesReceived  int64           `json:"bytes_received"`
	UploadSpeed    int64           `json:"upload_speed"`
	DownloadSpeed  int64           `json:"download_speed"`
	Transport      string          `json:"transport"`
	Transports     []TransportInfo `json:"transports"`
	MultipathPaths []PathInfo      `json:"multipath_paths,omitempty"`
	Mesh           *MeshStatus     `json:"mesh,omitempty"`
	Streams        *StreamStats    `json:"streams,omitempty"`
	CircuitState   string          `json:"circuit_state,omitempty"`
}

// StreamStats summarises per-stream metrics for the API.
type StreamStats struct {
	TotalStreams    int64 `json:"total_streams"`
	ActiveStreams   int64 `json:"active_streams"`
	TotalBytesSent int64 `json:"total_bytes_sent"`
	TotalBytesRecv int64 `json:"total_bytes_recv"`
	AvgDurationMs  int64 `json:"avg_duration_ms"`
}

// MeshStatus describes the mesh VPN connection status.
type MeshStatus struct {
	Enabled   bool       `json:"enabled"`
	VirtualIP string     `json:"virtual_ip,omitempty"`
	CIDR      string     `json:"cidr,omitempty"`
	Peers     []MeshPeer `json:"peers,omitempty"`
}

// MeshPeer describes a mesh peer and its connection quality.
type MeshPeer struct {
	VirtualIP   string  `json:"virtual_ip"`
	State       string  `json:"state"` // "connected", "connecting", "disconnected"
	Method      string  `json:"method,omitempty"` // "direct", "relay", "p2p"
	AvgRTT      int64   `json:"avg_rtt_ms"`
	MinRTT      int64   `json:"min_rtt_ms"`
	MaxRTT      int64   `json:"max_rtt_ms"`
	Jitter      int64   `json:"jitter_ms"`
	PacketLoss  float64 `json:"packet_loss"` // 0.0 - 1.0
	Score       int     `json:"score"`       // 0-100, higher is better
}

// PathInfo describes a single multipath transport path status.
type PathInfo struct {
	Transport     string `json:"transport"`
	Latency       int64  `json:"latency_ms"`
	ActiveStreams int64  `json:"active_streams"`
	TotalStreams  int64  `json:"total_streams"`
	Available     bool   `json:"available"`
	Failures      int64  `json:"failures"`
	BytesSent     int64  `json:"bytes_sent"`
	BytesReceived int64  `json:"bytes_received"`
}

// TransportInfo describes a transport's health.
type TransportInfo struct {
	Type      string `json:"type"`
	Available bool   `json:"available"`
	Latency   int64  `json:"latency_ms"`
}
