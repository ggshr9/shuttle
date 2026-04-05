package admin

import (
	"fmt"
	"io"
	"runtime"
)

// WritePrometheusMetrics writes metrics in Prometheus text exposition format.
func WritePrometheusMetrics(w io.Writer, info *ServerInfo, users *UserStore) {
	// Server metrics
	writeGauge(w, "shuttle_active_connections", "Number of active connections", float64(info.ActiveConns.Load()))
	writeCounter(w, "shuttle_total_connections", "Total connections since start", float64(info.TotalConns.Load()))
	writeCounter(w, "shuttle_bytes_sent_total", "Total bytes sent", float64(info.BytesSent.Load()))
	writeCounter(w, "shuttle_bytes_received_total", "Total bytes received", float64(info.BytesRecv.Load()))

	// Go runtime metrics
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	writeGauge(w, "shuttle_goroutines", "Number of goroutines", float64(runtime.NumGoroutine()))
	writeGauge(w, "shuttle_memory_alloc_bytes", "Allocated memory in bytes", float64(mem.Alloc))
	writeGauge(w, "shuttle_memory_sys_bytes", "System memory in bytes", float64(mem.Sys))
	writeCounter(w, "shuttle_gc_total", "Total GC cycles", float64(mem.NumGC))

	// Per-user metrics: emit HELP/TYPE once, then one line per user.
	if users != nil {
		userList := users.List()
		if len(userList) > 0 {
			fmt.Fprintf(w, "# HELP shuttle_user_bytes_sent_total Total bytes sent by user\n")
			fmt.Fprintf(w, "# TYPE shuttle_user_bytes_sent_total counter\n")
			for _, u := range userList {
				fmt.Fprintf(w, "shuttle_user_bytes_sent_total{user=%q} %g\n", u.Name, float64(u.BytesSent))
			}
			fmt.Fprintf(w, "# HELP shuttle_user_bytes_received_total Total bytes received by user\n")
			fmt.Fprintf(w, "# TYPE shuttle_user_bytes_received_total counter\n")
			for _, u := range userList {
				fmt.Fprintf(w, "shuttle_user_bytes_received_total{user=%q} %g\n", u.Name, float64(u.BytesRecv))
			}
			fmt.Fprintf(w, "# HELP shuttle_user_active_connections Active connections for user\n")
			fmt.Fprintf(w, "# TYPE shuttle_user_active_connections gauge\n")
			for _, u := range userList {
				fmt.Fprintf(w, "shuttle_user_active_connections{user=%q} %g\n", u.Name, float64(u.ActiveConns))
			}
		}
	}
}

func writeGauge(w io.Writer, name, help string, value float64) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %g\n", name, help, name, name, value)
}

func writeCounter(w io.Writer, name, help string, value float64) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n%s %g\n", name, help, name, name, value)
}
