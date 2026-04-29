package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/shuttleX/shuttle/engine"
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
		writeMetric(w, "shuttle_draining_connections", "Connections being drained during migration", "gauge", status.DrainingConns)

		// Engine-wide metrics derived from the MetricsSnapshot. Note: the
		// legacy unlabelled `shuttle_circuit_breaker_state` (single value
		// from status.CircuitState) is NOT emitted above — it has been
		// replaced by the labelled per-outbound variant below, with a
		// deprecated unlabelled shim retained for one minor version.
		snap := eng.Metrics()

		// Routing decisions, keyed as "<decision>/<rule>".
		fmt.Fprintf(w, "# HELP shuttle_routing_decisions_total Routing decisions by decision and rule type\n")
		fmt.Fprintf(w, "# TYPE shuttle_routing_decisions_total counter\n")
		for k, v := range snap.RoutingDecisions {
			parts := strings.SplitN(k, "/", 2)
			if len(parts) != 2 {
				continue
			}
			fmt.Fprintf(w, "shuttle_routing_decisions_total{decision=%q,rule=%q} %d\n", parts[0], parts[1], v)
		}
		fmt.Fprintln(w)

		// Per-outbound circuit breaker state (gauge: 0=closed, 1=open, 2=half-open).
		fmt.Fprintf(w, "# HELP shuttle_circuit_breaker_state Per-outbound circuit breaker state (0=closed,1=open,2=half-open)\n")
		fmt.Fprintf(w, "# TYPE shuttle_circuit_breaker_state gauge\n")
		for outbound, state := range snap.CircuitBreakers {
			v := 0
			switch state {
			case "open":
				v = 1
			case "half-open":
				v = 2
			}
			fmt.Fprintf(w, "shuttle_circuit_breaker_state{outbound=%q} %d\n", outbound, v)
		}
		// DEPRECATED: backward-compat shim emitting the unlabelled global
		// state derived from the worst (max-severity) per-outbound state.
		// Removed in v0.5 — consumers should migrate to the labelled form.
		worst := 0
		for _, state := range snap.CircuitBreakers {
			switch state {
			case "open":
				if worst < 1 {
					worst = 1
				}
			case "half-open":
				if worst < 2 {
					worst = 2
				}
			}
		}
		fmt.Fprintf(w, "# DEPRECATED: shuttle_circuit_breaker_state (unlabelled) — use the labelled variant; removed in v0.5.\n")
		fmt.Fprintf(w, "shuttle_circuit_breaker_state %d\n", worst)
		fmt.Fprintln(w)

		// Subscription refresh attempts and last-refresh timestamps.
		fmt.Fprintf(w, "# HELP shuttle_subscription_refresh_total Subscription refresh attempts\n")
		fmt.Fprintf(w, "# TYPE shuttle_subscription_refresh_total counter\n")
		for id, stats := range snap.Subscriptions {
			fmt.Fprintf(w, "shuttle_subscription_refresh_total{subscription=%q,result=\"ok\"} %d\n", id, stats.OK)
			fmt.Fprintf(w, "shuttle_subscription_refresh_total{subscription=%q,result=\"fail\"} %d\n", id, stats.Fail)
		}
		fmt.Fprintln(w)
		fmt.Fprintf(w, "# HELP shuttle_subscription_last_refresh_timestamp Unix timestamp of last refresh attempt\n")
		fmt.Fprintf(w, "# TYPE shuttle_subscription_last_refresh_timestamp gauge\n")
		for id, stats := range snap.Subscriptions {
			fmt.Fprintf(w, "shuttle_subscription_last_refresh_timestamp{subscription=%q} %d\n", id, stats.LastRefresh.Unix())
		}
		fmt.Fprintln(w)

		// Handshake durations — emitted as a Prometheus SUMMARY (count + sum)
		// rather than a full histogram. The engine stores raw observations
		// per transport (ring-buffered, capped at 1024); on the client we
		// have no preconfigured bucket boundaries, so a count+sum summary
		// is the honest exposition. The server-side counterpart
		// (cmd/shuttled) emits a full histogram with its own buckets.
		fmt.Fprintf(w, "# HELP shuttle_handshake_duration_seconds Client-side handshake duration (summary)\n")
		fmt.Fprintf(w, "# TYPE shuttle_handshake_duration_seconds summary\n")
		for transport, observations := range snap.HandshakeDurationsNanos {
			if len(observations) == 0 {
				continue
			}
			var sum int64
			for _, n := range observations {
				sum += n
			}
			fmt.Fprintf(w, "shuttle_handshake_duration_seconds_count{transport=%q} %d\n", transport, len(observations))
			fmt.Fprintf(w, "shuttle_handshake_duration_seconds_sum{transport=%q} %f\n", transport, float64(sum)/1e9)
		}
		fmt.Fprintln(w)
	})
}
