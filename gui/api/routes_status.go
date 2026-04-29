package api

import (
	"math"
	"net/http"
	"runtime"
	"time"

	"github.com/ggshr9/shuttle/engine"
	"github.com/ggshr9/shuttle/update"
)

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
		writeJSON(w, map[string]any{
			"engine_state":    status.State,
			"circuit_breaker": status.CircuitState,
			"streams":         status.Streams,
			"transport":       status.Transport,
			"uptime_seconds":  int64(time.Since(apiStartTime).Seconds()),
			"goroutines":      runtime.NumGoroutine(),
		})
	})

	mux.HandleFunc("GET /api/system/resources", func(w http.ResponseWriter, r *http.Request) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		writeJSON(w, map[string]any{
			"goroutines":     runtime.NumGoroutine(),
			"mem_alloc_mb":   math.Round(float64(m.Alloc)/1024/1024*100) / 100,
			"mem_sys_mb":     math.Round(float64(m.Sys)/1024/1024*100) / 100,
			"mem_gc_cycles":  m.NumGC,
			"num_cpu":        runtime.NumCPU(),
			"uptime_seconds": int64(time.Since(apiStartTime).Seconds()),
		})
	})
}
