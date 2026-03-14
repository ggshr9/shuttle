package mobile

import (
	"context"
	"log/slog"
	"strings"
	"sync"
)

const defaultLogBufferSize = 500

// logBuffer is a thread-safe ring buffer that captures recent log lines.
type logBuffer struct {
	mu    sync.Mutex
	lines []string
	pos   int  // next write position
	full  bool // whether the buffer has wrapped around
	size  int
}

// newLogBuffer creates a ring buffer that holds up to size lines.
func newLogBuffer(size int) *logBuffer {
	if size <= 0 {
		size = defaultLogBufferSize
	}
	return &logBuffer{
		lines: make([]string, size),
		size:  size,
	}
}

// write appends a line to the ring buffer.
func (b *logBuffer) write(line string) {
	b.mu.Lock()
	b.lines[b.pos] = line
	b.pos++
	if b.pos >= b.size {
		b.pos = 0
		b.full = true
	}
	b.mu.Unlock()
}

// recent returns up to maxLines of the most recent log lines.
func (b *logBuffer) recent(maxLines int) string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if maxLines <= 0 {
		maxLines = b.size
	}

	var count int
	if b.full {
		count = b.size
	} else {
		count = b.pos
	}
	if count == 0 {
		return ""
	}
	if maxLines > count {
		maxLines = count
	}

	result := make([]string, maxLines)
	// Read the last maxLines entries in order.
	start := b.pos - maxLines
	if start < 0 {
		start += b.size
	}
	for i := 0; i < maxLines; i++ {
		idx := (start + i) % b.size
		result[i] = b.lines[idx]
	}
	return strings.Join(result, "\n")
}

// logHandler is a slog.Handler that writes formatted log lines into a logBuffer.
type logHandler struct {
	buf   *logBuffer
	attrs []slog.Attr
	group string
}

func newLogHandler(buf *logBuffer) *logHandler {
	return &logHandler{buf: buf}
}

func (h *logHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (h *logHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Time.Format("15:04:05"))
	b.WriteString(" ")
	b.WriteString(r.Level.String())
	b.WriteString(" ")
	if h.group != "" {
		b.WriteString(h.group)
		b.WriteString(".")
	}
	b.WriteString(r.Message)

	// Append pre-set attrs
	for _, a := range h.attrs {
		b.WriteString(" ")
		b.WriteString(a.Key)
		b.WriteString("=")
		b.WriteString(a.Value.String())
	}

	// Append record attrs
	r.Attrs(func(a slog.Attr) bool {
		b.WriteString(" ")
		b.WriteString(a.Key)
		b.WriteString("=")
		b.WriteString(a.Value.String())
		return true
	})

	h.buf.write(b.String())
	return nil
}

func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &logHandler{buf: h.buf, attrs: newAttrs, group: h.group}
}

func (h *logHandler) WithGroup(name string) slog.Handler {
	newGroup := name
	if h.group != "" {
		newGroup = h.group + "." + name
	}
	return &logHandler{buf: h.buf, attrs: h.attrs, group: newGroup}
}
