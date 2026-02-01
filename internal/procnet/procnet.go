package procnet

// ProcInfo describes a process with active network connections.
type ProcInfo struct {
	PID   uint32 `json:"pid"`
	Name  string `json:"name"`  // e.g. "chrome.exe"
	Conns int    `json:"conns"` // active TCP connection count
}
