package proxy

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
)

func TestParseSOCKS5UDPHeader_IPv4(t *testing.T) {
	// Build a SOCKS5 UDP header: RSV(2) + FRAG(1) + ATYP(1) + IPv4(4) + PORT(2) + DATA
	var buf bytes.Buffer
	buf.Write([]byte{0x00, 0x00}) // RSV
	buf.WriteByte(0x00)           // FRAG
	buf.WriteByte(atypIPv4)       // ATYP
	buf.Write(net.IPv4(8, 8, 8, 8).To4())
	binary.Write(&buf, binary.BigEndian, uint16(53))
	buf.Write([]byte("dns query"))

	data := buf.Bytes()
	addr, headerLen, err := parseSOCKS5UDPHeader(data)
	if err != nil {
		t.Fatalf("parseSOCKS5UDPHeader: %v", err)
	}
	if addr != "8.8.8.8:53" {
		t.Errorf("addr = %q, want %q", addr, "8.8.8.8:53")
	}
	payload := data[headerLen:]
	if string(payload) != "dns query" {
		t.Errorf("payload = %q, want %q", payload, "dns query")
	}
}

func TestParseSOCKS5UDPHeader_Domain(t *testing.T) {
	var buf bytes.Buffer
	buf.Write([]byte{0x00, 0x00}) // RSV
	buf.WriteByte(0x00)           // FRAG
	buf.WriteByte(atypDomain)     // ATYP
	domain := "example.com"
	buf.WriteByte(byte(len(domain)))
	buf.WriteString(domain)
	binary.Write(&buf, binary.BigEndian, uint16(443))
	buf.Write([]byte("quic data"))

	data := buf.Bytes()
	addr, headerLen, err := parseSOCKS5UDPHeader(data)
	if err != nil {
		t.Fatalf("parseSOCKS5UDPHeader: %v", err)
	}
	if addr != "example.com:443" {
		t.Errorf("addr = %q, want %q", addr, "example.com:443")
	}
	payload := data[headerLen:]
	if string(payload) != "quic data" {
		t.Errorf("payload = %q, want %q", payload, "quic data")
	}
}

func TestParseSOCKS5UDPHeader_IPv6(t *testing.T) {
	var buf bytes.Buffer
	buf.Write([]byte{0x00, 0x00}) // RSV
	buf.WriteByte(0x00)           // FRAG
	buf.WriteByte(atypIPv6)       // ATYP
	ip6 := net.ParseIP("2001:db8::1").To16()
	buf.Write(ip6)
	binary.Write(&buf, binary.BigEndian, uint16(53))
	buf.Write([]byte("v6 query"))

	data := buf.Bytes()
	addr, headerLen, err := parseSOCKS5UDPHeader(data)
	if err != nil {
		t.Fatalf("parseSOCKS5UDPHeader: %v", err)
	}
	if addr != "[2001:db8::1]:53" {
		t.Errorf("addr = %q, want %q", addr, "[2001:db8::1]:53")
	}
	payload := data[headerLen:]
	if string(payload) != "v6 query" {
		t.Errorf("payload = %q, want %q", payload, "v6 query")
	}
}

func TestParseSOCKS5UDPHeader_TooShort(t *testing.T) {
	_, _, err := parseSOCKS5UDPHeader([]byte{0x00, 0x00})
	if err == nil {
		t.Fatal("expected error for short data")
	}
}

func TestParseSOCKS5UDPHeader_UnsupportedAtyp(t *testing.T) {
	data := []byte{0x00, 0x00, 0x00, 0xFF, 0, 0, 0, 0, 0, 0}
	_, _, err := parseSOCKS5UDPHeader(data)
	if err == nil {
		t.Fatal("expected error for unsupported atyp")
	}
}

func TestBuildSOCKS5UDPResponse_IPv4(t *testing.T) {
	resp, err := buildSOCKS5UDPResponse("1.2.3.4:53", []byte("response"))
	if err != nil {
		t.Fatalf("buildSOCKS5UDPResponse: %v", err)
	}

	// Verify RSV=0, FRAG=0
	if resp[0] != 0 || resp[1] != 0 || resp[2] != 0 {
		t.Errorf("RSV/FRAG not zero: %x %x %x", resp[0], resp[1], resp[2])
	}
	// ATYP should be IPv4
	if resp[3] != atypIPv4 {
		t.Errorf("ATYP = %d, want %d", resp[3], atypIPv4)
	}
	// Parse it back through parseSOCKS5UDPHeader
	addr, headerLen, err := parseSOCKS5UDPHeader(resp)
	if err != nil {
		t.Fatalf("parseSOCKS5UDPHeader: %v", err)
	}
	if addr != "1.2.3.4:53" {
		t.Errorf("addr = %q, want %q", addr, "1.2.3.4:53")
	}
	payload := resp[headerLen:]
	if string(payload) != "response" {
		t.Errorf("payload = %q, want %q", payload, "response")
	}
}

func TestBuildSOCKS5UDPResponse_Domain(t *testing.T) {
	resp, err := buildSOCKS5UDPResponse("example.com:443", []byte("data"))
	if err != nil {
		t.Fatalf("buildSOCKS5UDPResponse: %v", err)
	}

	addr, headerLen, err := parseSOCKS5UDPHeader(resp)
	if err != nil {
		t.Fatalf("parseSOCKS5UDPHeader: %v", err)
	}
	if addr != "example.com:443" {
		t.Errorf("addr = %q, want %q", addr, "example.com:443")
	}
	if string(resp[headerLen:]) != "data" {
		t.Errorf("payload = %q, want %q", resp[headerLen:], "data")
	}
}

func TestBuildSOCKS5UDPResponse_InvalidAddr(t *testing.T) {
	_, err := buildSOCKS5UDPResponse("no-port", []byte("x"))
	if err == nil {
		t.Fatal("expected error for invalid addr")
	}
}
