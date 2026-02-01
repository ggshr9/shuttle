package plugin

import (
	"context"
	"log/slog"
	"net"
)

// Logger is a plugin that logs connections.
type Logger struct {
	logger *slog.Logger
}

func NewLogger(logger *slog.Logger) *Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return &Logger{logger: logger}
}

func (l *Logger) Name() string                  { return "logger" }
func (l *Logger) Init(ctx context.Context) error { return nil }
func (l *Logger) Close() error                   { return nil }

func (l *Logger) OnConnect(conn net.Conn, target string) (net.Conn, error) {
	l.logger.Info("connection opened",
		"remote", conn.RemoteAddr(),
		"target", target)
	return conn, nil
}

func (l *Logger) OnDisconnect(conn net.Conn) {
	l.logger.Info("connection closed", "remote", conn.RemoteAddr())
}

var _ ConnPlugin = (*Logger)(nil)
