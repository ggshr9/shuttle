package qrterm

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintBasic(t *testing.T) {
	var buf bytes.Buffer
	err := Print(&buf, "hello")
	if err != nil {
		t.Fatalf("Print: %v", err)
	}
	output := buf.String()
	if len(output) == 0 {
		t.Fatal("expected non-empty QR output")
	}
	// QR code should contain Unicode block characters
	if !strings.ContainsAny(output, "█▀▄ ") {
		t.Fatal("expected Unicode block characters in output")
	}
}

func TestPrintURL(t *testing.T) {
	var buf bytes.Buffer
	err := Print(&buf, "https://example.com/share/abc123")
	if err != nil {
		t.Fatalf("Print URL: %v", err)
	}
	// Should produce multiple lines
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) < 5 {
		t.Fatalf("expected at least 5 lines, got %d", len(lines))
	}
}

func TestPrintEmpty(t *testing.T) {
	var buf bytes.Buffer
	// Empty string cannot be encoded as QR — should return error
	err := Print(&buf, "")
	if err == nil {
		t.Fatal("expected error for empty string")
	}
}

func TestPrintLongString(t *testing.T) {
	var buf bytes.Buffer
	long := strings.Repeat("a", 500)
	err := Print(&buf, long)
	if err != nil {
		t.Fatalf("Print long string: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("expected output for long string")
	}
}

func TestPrintDeterministic(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	Print(&buf1, "deterministic-test")
	Print(&buf2, "deterministic-test")
	if !bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
		t.Fatal("QR output should be deterministic for the same input")
	}
}

func TestPrintDifferentInputs(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	Print(&buf1, "input-one")
	Print(&buf2, "input-two")
	if bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
		t.Fatal("different inputs should produce different QR codes")
	}
}
