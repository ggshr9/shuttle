// Minimal httpbin replacement for cloud e2e testing.
// Implements only the endpoints used by test/e2e/sandbox_test.go.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	addr := "127.0.0.1:18080"
	if a := os.Getenv("HTTPBIN_ADDR"); a != "" {
		addr = a
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/ip", func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		_ = json.NewEncoder(w).Encode(map[string]string{"origin": host})
	})

	mux.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		headers := make(map[string]string)
		for k, v := range r.Header {
			headers[k] = strings.Join(v, ", ")
		}
		args := make(map[string]string)
		for k, v := range r.URL.Query() {
			args[k] = strings.Join(v, ", ")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"headers":    headers,
			"args":       args,
			"url":        fmt.Sprintf("http://%s%s", r.Host, r.RequestURI),
			"origin":     r.RemoteAddr,
			"method":     r.Method,
			"user-agent": r.UserAgent(),
		})
	})

	mux.HandleFunc("/headers", func(w http.ResponseWriter, r *http.Request) {
		headers := make(map[string]string)
		for k, v := range r.Header {
			headers[k] = strings.Join(v, ", ")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"headers": headers})
	})

	mux.HandleFunc("/user-agent", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"user-agent": r.UserAgent()})
	})

	fmt.Fprintf(os.Stderr, "httpbin listening on %s\n", addr)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "httpbin: %v\n", err)
		os.Exit(1)
	}
}
