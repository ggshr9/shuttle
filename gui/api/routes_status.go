package api

import (
	"math"
	"net/http"
	"runtime"
	"time"

	"github.com/ggshr9/shuttle/engine"
	"github.com/ggshr9/shuttle/update"
)

// debugStateDTO mirrors the frontend's DebugState interface
// (gui/web/src/lib/api/types.ts). Field renames here MUST be matched
// on the TS side; a typed Go struct gives the Go compiler a chance
// to flag drift instead of silently serialising the wrong shape.
type debugStateDTO struct {
	EngineState    string `json:"engine_state"`
	CircuitBreaker string `json:"circuit_breaker"`
	Streams        int64  `json:"streams"`
	Transport      string `json:"transport"`
	UptimeSeconds  int64  `json:"uptime_seconds"`
	Goroutines     int    `json:"goroutines"`
}

// systemResourcesDTO mirrors SystemResources on the TS side.
type systemResourcesDTO struct {
	Goroutines    int     `json:"goroutines"`
	MemAllocMB    float64 `json:"mem_alloc_mb"`
	MemSysMB      float64 `json:"mem_sys_mb"`
	MemGCCycles   uint32  `json:"mem_gc_cycles"`
	NumCPU        int     `json:"num_cpu"`
	UptimeSeconds int64   `json:"uptime_seconds"`
}

func registerStatusRoutes(mux *http.ServeMux, eng *engine.Engine) {
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, eng.Status())
	})

	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{
			"version": update.GetCurrentVersion(),
		})
	})

	mux.HandleFunc("GET /api/debug/state", func(w http.ResponseWriter, r *http.Request) {
		status := eng.Status()
		var streams int64
		if status.Streams != nil {
			streams = status.Streams.ActiveStreams
		}
		writeJSON(w, debugStateDTO{
			EngineState:    status.State,
			CircuitBreaker: status.CircuitState,
			Streams:        streams,
			Transport:      status.Transport,
			UptimeSeconds:  int64(time.Since(apiStartTime).Seconds()),
			Goroutines:     runtime.NumGoroutine(),
		})
	})

	mux.HandleFunc("GET /api/system/resources", func(w http.ResponseWriter, r *http.Request) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		writeJSON(w, systemResourcesDTO{
			Goroutines:    runtime.NumGoroutine(),
			MemAllocMB:    math.Round(float64(m.Alloc)/1024/1024*100) / 100,
			MemSysMB:      math.Round(float64(m.Sys)/1024/1024*100) / 100,
			MemGCCycles:   m.NumGC,
			NumCPU:        runtime.NumCPU(),
			UptimeSeconds: int64(time.Since(apiStartTime).Seconds()),
		})
	})
}
