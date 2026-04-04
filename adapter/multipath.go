package adapter

// MultipathStats holds per-path statistics for multipath transports.
type MultipathStats struct {
	Interface string  `json:"interface"`
	LocalAddr string  `json:"local_addr"`
	RTT       int64   `json:"rtt_ms"`
	LossRate  float64 `json:"loss_rate"`
	BytesSent int64   `json:"bytes_sent"`
	BytesRecv int64   `json:"bytes_recv"`
	Available bool    `json:"available"`
}

// MultipathStatsProvider is an optional interface for transports that support
// multipath and can report per-path statistics.
type MultipathStatsProvider interface {
	MultipathStats() []MultipathStats
}
