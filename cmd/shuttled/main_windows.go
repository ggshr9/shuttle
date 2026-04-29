//go:build windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/ggshr9/shuttle/internal/paths"
	"golang.org/x/sys/windows/svc"
)

func servicePreflight() {
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		return
	}
	// Route logs to a file so service output is captured.
	logDir := paths.Resolve(paths.ScopeSystem).LogDir
	_ = os.MkdirAll(logDir, 0755)
	logPath := filepath.Join(logDir, "shuttled.log")
	if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{})))
	}
	if err := svc.Run("shuttled", &winSvcHandler{}); err != nil {
		fmt.Fprintf(os.Stderr, "svc.Run: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

type winSvcHandler struct{}

func (h *winSvcHandler) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	changes <- svc.Status{State: svc.StartPending}

	cfgPath := findConfigArg(os.Args)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runWithContext(ctx, cfgPath)
	}()

	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case <-done:
				case <-time.After(10 * time.Second):
				}
				changes <- svc.Status{State: svc.Stopped}
				return
			}
		case <-done:
			// runWithContext returned unexpectedly (e.g., server start failed).
			// Report stopped so SCM does not show the service as healthy.
			changes <- svc.Status{State: svc.Stopped}
			return
		}
	}
}

func findConfigArg(args []string) string {
	for i, a := range args {
		if a == "-c" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
