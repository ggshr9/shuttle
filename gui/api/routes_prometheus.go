package api

import (
	"fmt"
	"net/http"

	"github.com/shuttleX/shuttle/engine"
)

func registerPrometheusRoutes(mux *http.ServeMux, eng *engine.Engine) {
	mux.HandleFunc("GET /api/prometheus", func(w http.ResponseWriter, r *http.Request) {
		status := eng.Status()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		fmt.Fprintf(w, "# HELP shuttle_active_connections Current active connections\n")
		fmt.Fprintf(w, "# TYPE shuttle_active_connections gauge\n")
		fmt.Fprintf(w, "shuttle_active_connections %d\n\n", status.ActiveConns)

		fmt.Fprintf(w, "# HELP shuttle_total_connections Total connections since start\n")
		fmt.Fprintf(w, "# TYPE shuttle_total_connections counter\n")
		fmt.Fprintf(w, "shuttle_total_connections %d\n\n", status.TotalConns)

		fmt.Fprintf(w, "# HELP shuttle_bytes_sent Total bytes sent\n")
		fmt.Fprintf(w, "# TYPE shuttle_bytes_sent counter\n")
		fmt.Fprintf(w, "shuttle_bytes_sent %d\n\n", status.BytesSent)

		fmt.Fprintf(w, "# HELP shuttle_bytes_received Total bytes received\n")
		fmt.Fprintf(w, "# TYPE shuttle_bytes_received counter\n")
		fmt.Fprintf(w, "shuttle_bytes_received %d\n\n", status.BytesReceived)

		fmt.Fprintf(w, "# HELP shuttle_upload_speed_bytes Current upload speed in bytes/sec\n")
		fmt.Fprintf(w, "# TYPE shuttle_upload_speed_bytes gauge\n")
		fmt.Fprintf(w, "shuttle_upload_speed_bytes %d\n\n", status.UploadSpeed)

		fmt.Fprintf(w, "# HELP shuttle_download_speed_bytes Current download speed in bytes/sec\n")
		fmt.Fprintf(w, "# TYPE shuttle_download_speed_bytes gauge\n")
		fmt.Fprintf(w, "shuttle_download_speed_bytes %d\n\n", status.DownloadSpeed)

		// Circuit breaker state: 0=closed, 1=open, 2=half-open
		fmt.Fprintf(w, "# HELP shuttle_circuit_breaker_state Circuit breaker state\n")
		fmt.Fprintf(w, "# TYPE shuttle_circuit_breaker_state gauge\n")
		cbState := 0
		switch status.CircuitState {
		case "open":
			cbState = 1
		case "half-open":
			cbState = 2
		}
		fmt.Fprintf(w, "shuttle_circuit_breaker_state %d\n\n", cbState)

		fmt.Fprintf(w, "# HELP shuttle_draining_connections Connections being drained during migration\n")
		fmt.Fprintf(w, "# TYPE shuttle_draining_connections gauge\n")
		fmt.Fprintf(w, "shuttle_draining_connections %d\n\n", status.DrainingConns)
	})
}
