package engine

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"strings"
	"time"

	"github.com/shuttle-proxy/shuttle/update"
)

// DiagnosticsBundle collects system state for debugging.
type DiagnosticsBundle struct {
	Timestamp   time.Time              `json:"timestamp"`
	Version     string                 `json:"version"`
	System      SystemInfo             `json:"system"`
	Config      map[string]interface{} `json:"config"`
	Status      *EngineStatus          `json:"status"`
	Connections []ConnectionDiag       `json:"connections"`
	DNS         DNSDiag                `json:"dns"`
	Router      RouterDiag             `json:"router"`
}

// SystemInfo holds runtime and OS information.
type SystemInfo struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	NumCPU   int    `json:"num_cpu"`
	GoVer    string `json:"go_version"`
	NumGR    int    `json:"goroutines"`
	MemAlloc uint64 `json:"mem_alloc_bytes"`
	MemSys   uint64 `json:"mem_sys_bytes"`
	Uptime   string `json:"uptime"`
}

// ConnectionDiag describes a single connection for diagnostics.
type ConnectionDiag struct {
	ID        string `json:"id"`
	Transport string `json:"transport"`
	State     string `json:"state"`
	Streams   int    `json:"streams"`
	BytesIn   int64  `json:"bytes_in"`
	BytesOut  int64  `json:"bytes_out"`
	Duration  string `json:"duration"`
}

// DNSDiag holds DNS resolver diagnostics.
type DNSDiag struct {
	CacheSize   int      `json:"cache_size"`
	Servers     []string `json:"servers"`
	PrefetchTop []string `json:"prefetch_top,omitempty"`
}

// RouterDiag holds routing engine diagnostics.
type RouterDiag struct {
	RuleCount  int              `json:"rule_count"`
	GeoIPReady bool             `json:"geoip_ready"`
	Stats      map[string]int64 `json:"stats"`
}

// CollectDiagnostics gathers all diagnostic information from the engine.
func (e *Engine) CollectDiagnostics() *DiagnosticsBundle {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	status := e.Status()

	// Build connection list from stream tracker
	var conns []ConnectionDiag
	e.mu.RLock()
	st := e.streamTracker
	cfg := e.cfg
	rt := e.currentRouter
	gm := e.geoManager
	e.mu.RUnlock()

	if st != nil {
		for _, ts := range st.ByTransport() {
			conns = append(conns, ConnectionDiag{
				Transport: ts.Transport,
				State:     "active",
				Streams:   int(ts.ActiveStreams),
				BytesIn:   ts.BytesRecv,
				BytesOut:  ts.BytesSent,
			})
		}
	}

	// Build DNS diagnostics
	dnsDiag := DNSDiag{}
	if cfg != nil {
		var servers []string
		if cfg.Routing.DNS.Domestic != "" {
			servers = append(servers, cfg.Routing.DNS.Domestic)
		}
		if cfg.Routing.DNS.Remote.Server != "" {
			servers = append(servers, cfg.Routing.DNS.Remote.Server)
		}
		dnsDiag.Servers = servers
	}

	// Build router diagnostics
	routerDiag := RouterDiag{
		Stats: make(map[string]int64),
	}
	if cfg != nil {
		routerDiag.RuleCount = len(cfg.Routing.Rules)
	}
	if rt != nil {
		routerDiag.GeoIPReady = true
	}
	if gm != nil {
		routerDiag.Stats["geo_manager_active"] = 1
	}

	// Build redacted config
	var redacted map[string]interface{}
	if cfg != nil {
		redacted = redactConfig(cfg)
	}

	// Calculate uptime from apiStartTime (package-level in gui/api)
	// We use a simple goroutine-count-based heuristic; the real uptime
	// comes from the API layer. Here we report runtime uptime.
	uptime := "unknown"

	return &DiagnosticsBundle{
		Timestamp:   time.Now(),
		Version:     update.Version,
		System: SystemInfo{
			OS:       runtime.GOOS,
			Arch:     runtime.GOARCH,
			NumCPU:   runtime.NumCPU(),
			GoVer:    runtime.Version(),
			NumGR:    runtime.NumGoroutine(),
			MemAlloc: mem.Alloc,
			MemSys:   mem.Sys,
			Uptime:   uptime,
		},
		Config:      redacted,
		Status:      &status,
		Connections: conns,
		DNS:         dnsDiag,
		Router:      routerDiag,
	}
}

// ExportDiagnosticsZIP writes a ZIP file containing:
//   - diagnostics.json (the full bundle as formatted JSON)
//   - goroutines.txt (runtime stack dump)
//   - config.yaml (redacted copy of current config)
func (e *Engine) ExportDiagnosticsZIP(w io.Writer) error {
	bundle := e.CollectDiagnostics()

	zw := zip.NewWriter(w)
	defer zw.Close()

	// 1. diagnostics.json
	diagJSON, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal diagnostics: %w", err)
	}
	f, err := zw.Create("diagnostics.json")
	if err != nil {
		return fmt.Errorf("create diagnostics.json: %w", err)
	}
	if _, err := f.Write(diagJSON); err != nil {
		return fmt.Errorf("write diagnostics.json: %w", err)
	}

	// 2. goroutines.txt
	buf := make([]byte, 1<<20) // 1MB buffer for stack traces
	n := runtime.Stack(buf, true)
	gf, err := zw.Create("goroutines.txt")
	if err != nil {
		return fmt.Errorf("create goroutines.txt: %w", err)
	}
	if _, err := gf.Write(buf[:n]); err != nil {
		return fmt.Errorf("write goroutines.txt: %w", err)
	}

	// 3. config.yaml (redacted config as JSON, since we don't have yaml marshal dependency here)
	if bundle.Config != nil {
		cfgJSON, err := json.MarshalIndent(bundle.Config, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal config: %w", err)
		}
		cf, err := zw.Create("config.json")
		if err != nil {
			return fmt.Errorf("create config.json: %w", err)
		}
		if _, err := cf.Write(cfgJSON); err != nil {
			return fmt.Errorf("write config.json: %w", err)
		}
	}

	return nil
}

// sensitiveKeys lists field name substrings that should be redacted.
var sensitiveKeys = []string{
	"password",
	"token",
	"key",
	"secret",
	"private_key",
	"private",
}

// redactConfig returns a copy of the config with sensitive fields replaced with "***".
// It works by marshalling to JSON, unmarshalling to a generic map, then walking the map.
func redactConfig(cfg interface{}) map[string]interface{} {
	data, err := json.Marshal(cfg)
	if err != nil {
		return map[string]interface{}{"error": "failed to marshal config"}
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]interface{}{"error": "failed to unmarshal config"}
	}

	redactMap(m)
	return m
}

// redactMap recursively walks a map and replaces sensitive string values with "***".
func redactMap(m map[string]interface{}) {
	for k, v := range m {
		if isSensitiveKey(k) {
			if s, ok := v.(string); ok && s != "" {
				m[k] = "***"
			}
			continue
		}
		switch val := v.(type) {
		case map[string]interface{}:
			redactMap(val)
		case []interface{}:
			redactSlice(val)
		}
	}
}

// redactSlice recursively walks a slice and redacts sensitive values in nested maps.
func redactSlice(s []interface{}) {
	for _, item := range s {
		switch val := item.(type) {
		case map[string]interface{}:
			redactMap(val)
		}
	}
}

// isSensitiveKey returns true if the key name suggests a sensitive value.
func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, sensitive := range sensitiveKeys {
		if strings.Contains(lower, sensitive) {
			return true
		}
	}
	return false
}

