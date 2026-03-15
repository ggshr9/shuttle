package proxy

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func TestWriteReadUDPFrame_IPv4(t *testing.T) {
	var buf bytes.Buffer
	addr := "1.2.3.4:53"
	payload := []byte("hello dns")

	if err := WriteUDPFrame(&buf, addr, payload); err != nil {
		t.Fatalf("WriteUDPFrame: %v", err)
	}

	gotAddr, gotPayload, err := ReadUDPFrame(&buf)
	if err != nil {
		t.Fatalf("ReadUDPFrame: %v", err)
	}
	if gotAddr != addr {
		t.Errorf("addr = %q, want %q", gotAddr, addr)
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Errorf("payload = %q, want %q", gotPayload, payload)
	}
}

func TestWriteReadUDPFrame_IPv6(t *testing.T) {
	var buf bytes.Buffer
	addr := "[::1]:53"
	payload := []byte("ipv6 data")

	if err := WriteUDPFrame(&buf, addr, payload); err != nil {
		t.Fatalf("WriteUDPFrame: %v", err)
	}

	gotAddr, gotPayload, err := ReadUDPFrame(&buf)
	if err != nil {
		t.Fatalf("ReadUDPFrame: %v", err)
	}
	if gotAddr != addr {
		t.Errorf("addr = %q, want %q", gotAddr, addr)
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Errorf("payload = %q, want %q", gotPayload, payload)
	}
}

func TestWriteReadUDPFrame_Domain(t *testing.T) {
	var buf bytes.Buffer
	addr := "example.com:443"
	payload := []byte("quic init")

	if err := WriteUDPFrame(&buf, addr, payload); err != nil {
		t.Fatalf("WriteUDPFrame: %v", err)
	}

	gotAddr, gotPayload, err := ReadUDPFrame(&buf)
	if err != nil {
		t.Fatalf("ReadUDPFrame: %v", err)
	}
	if gotAddr != addr {
		t.Errorf("addr = %q, want %q", gotAddr, addr)
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Errorf("payload = %q, want %q", gotPayload, payload)
	}
}

func TestWriteReadUDPFrame_ZeroPayload(t *testing.T) {
	var buf bytes.Buffer
	addr := "10.0.0.1:8080"
	payload := []byte{}

	if err := WriteUDPFrame(&buf, addr, payload); err != nil {
		t.Fatalf("WriteUDPFrame: %v", err)
	}

	gotAddr, gotPayload, err := ReadUDPFrame(&buf)
	if err != nil {
		t.Fatalf("ReadUDPFrame: %v", err)
	}
	if gotAddr != addr {
		t.Errorf("addr = %q, want %q", gotAddr, addr)
	}
	if len(gotPayload) != 0 {
		t.Errorf("payload len = %d, want 0", len(gotPayload))
	}
}

func TestWriteUDPFrame_PayloadTooLarge(t *testing.T) {
	var buf bytes.Buffer
	payload := make([]byte, maxUDPPayload+1)
	err := WriteUDPFrame(&buf, "1.2.3.4:53", payload)
	if err == nil {
		t.Fatal("expected error for oversized payload")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWriteUDPFrame_InvalidAddr(t *testing.T) {
	var buf bytes.Buffer
	err := WriteUDPFrame(&buf, "no-port", []byte("x"))
	if err == nil {
		t.Fatal("expected error for invalid addr")
	}
}

func TestReadUDPFrame_BodyTooShort(t *testing.T) {
	// Write a frame with body length = 2 (too short for any valid address)
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, uint16(2))
	buf.Write([]byte{0x01, 0x00}) // atyp IPv4 but only 1 byte of addr

	_, _, err := ReadUDPFrame(&buf)
	if err == nil {
		t.Fatal("expected error for short body")
	}
}

func TestReadUDPFrame_UnsupportedAtyp(t *testing.T) {
	// Write a valid-length frame with an unsupported atyp
	var buf bytes.Buffer
	body := make([]byte, 10)
	body[0] = 0xFF // invalid atyp
	binary.Write(&buf, binary.BigEndian, uint16(len(body)))
	buf.Write(body)

	_, _, err := ReadUDPFrame(&buf)
	if err == nil {
		t.Fatal("expected error for unsupported atyp")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWriteReadUDPFrame_MultipleFrames(t *testing.T) {
	var buf bytes.Buffer
	frames := []struct {
		addr    string
		payload []byte
	}{
		{"1.2.3.4:53", []byte("query1")},
		{"8.8.8.8:53", []byte("query2")},
		{"example.com:443", []byte("quic")},
	}

	for _, f := range frames {
		if err := WriteUDPFrame(&buf, f.addr, f.payload); err != nil {
			t.Fatalf("WriteUDPFrame(%s): %v", f.addr, err)
		}
	}

	for _, f := range frames {
		gotAddr, gotPayload, err := ReadUDPFrame(&buf)
		if err != nil {
			t.Fatalf("ReadUDPFrame: %v", err)
		}
		if gotAddr != f.addr {
			t.Errorf("addr = %q, want %q", gotAddr, f.addr)
		}
		if !bytes.Equal(gotPayload, f.payload) {
			t.Errorf("payload = %q, want %q", gotPayload, f.payload)
		}
	}
}

func TestWriteReadUDPFrame_MaxPayload(t *testing.T) {
	var buf bytes.Buffer
	addr := "1.2.3.4:53"
	payload := make([]byte, maxUDPPayload)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	if err := WriteUDPFrame(&buf, addr, payload); err != nil {
		t.Fatalf("WriteUDPFrame: %v", err)
	}

	gotAddr, gotPayload, err := ReadUDPFrame(&buf)
	if err != nil {
		t.Fatalf("ReadUDPFrame: %v", err)
	}
	if gotAddr != addr {
		t.Errorf("addr = %q, want %q", gotAddr, addr)
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Errorf("payload mismatch (len %d vs %d)", len(gotPayload), len(payload))
	}
}
