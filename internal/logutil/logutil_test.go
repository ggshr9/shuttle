package logutil

import (
	"log/slog"
	"testing"
)

func TestNewLoggerText(t *testing.T) {
	logger := NewLogger("info", "text")
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	handler := logger.Handler()
	if _, ok := handler.(*slog.TextHandler); !ok {
		t.Fatalf("expected *slog.TextHandler, got %T", handler)
	}
}

func TestNewLoggerJSON(t *testing.T) {
	logger := NewLogger("info", "json")
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	handler := logger.Handler()
	if _, ok := handler.(*slog.JSONHandler); !ok {
		t.Fatalf("expected *slog.JSONHandler, got %T", handler)
	}
}

func TestNewLoggerLevels(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run("level_"+tt.input, func(t *testing.T) {
			logger := NewLogger(tt.input, "text")
			if logger == nil {
				t.Fatal("expected non-nil logger")
			}
			if !logger.Handler().Enabled(nil, tt.expected) {
				t.Errorf("expected level %v to be enabled for input %q", tt.expected, tt.input)
			}
		})
	}
}

func TestNewLoggerDefaultFormat(t *testing.T) {
	// Empty format should default to text
	logger := NewLogger("info", "")
	handler := logger.Handler()
	if _, ok := handler.(*slog.TextHandler); !ok {
		t.Fatalf("expected *slog.TextHandler for empty format, got %T", handler)
	}
}
