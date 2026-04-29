package api

import (
	"fmt"
	"io"
	"net/http"

	"github.com/ggshr9/shuttle/engine"
)

// writeMetric writes a Prometheus HELP, TYPE, and value triple for a single metric.
func writeMetric(w io.Writer, name, help, typ string, val any) {
	fmt.Fprintf(w, "# HELP %s %s\n", name, help)
	fmt.Fprintf(w, "# TYPE %s %s\n", name, typ)
	fmt.Fprintf(w, "%s %v\n\n", name, val)
}

func registerPrometheusRoutes(mux *http.ServeMux, eng *engine.Engine) {
	mux.HandleFunc("GET /api/prometheus", func(w http.ResponseWriter, r *http.Request) {
		status := eng.Status()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		writeMetric(w, "shuttle_active_connections", "Current active connections", "gauge", status.ActiveConns)
		writeMetric(w, "shuttle_total_connections", "Total connections since start", "counter", status.TotalConns)
		writeMetric(w, "shuttle_bytes_sent", "Total bytes sent", "counter", status.BytesSent)
		writeMetric(w, "shuttle_bytes_received", "Total bytes received", "counter", status.BytesReceived)
		writeMetric(w, "shuttle_upload_speed_bytes", "Current upload speed in bytes/sec", "gauge", status.UploadSpeed)
		writeMetric(w, "shuttle_download_speed_bytes", "Current download speed in bytes/sec", "gauge", status.DownloadSpeed)

		// Circuit breaker state: 0=closed, 1=open, 2=half-open
		cbState := 0
		switch status.CircuitState {
		case "open":
			cbState = 1
		case "half-open":
			cbState = 2
		}
		writeMetric(w, "shuttle_circuit_breaker_state", "Circuit breaker state", "gauge", cbState)

		writeMetric(w, "shuttle_draining_connections", "Connections being drained during migration", "gauge", status.DrainingConns)
	})
}
