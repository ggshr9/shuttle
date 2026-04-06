package shared_test

import (
	"bytes"
	"testing"

	"github.com/shuttleX/shuttle/transport/shared"
)

func TestEncodeDecodeAddr_Domain(t *testing.T) {
	var buf bytes.Buffer
	if err := shared.EncodeAddr(&buf, "tcp", "example.com:443"); err != nil {
		t.Fatalf("EncodeAddr: %v", err)
	}

	network, address, err := shared.DecodeAddr(&buf)
	if err != nil {
		t.Fatalf("DecodeAddr: %v", err)
	}
	if network != "tcp" {
		t.Errorf("network = %q, want %q", network, "tcp")
	}
	if address != "example.com:443" {
		t.Errorf("address = %q, want %q", address, "example.com:443")
	}
}

func TestEncodeDecodeAddr_IPv4(t *testing.T) {
	var buf bytes.Buffer
	if err := shared.EncodeAddr(&buf, "tcp", "192.168.1.1:8080"); err != nil {
		t.Fatalf("EncodeAddr: %v", err)
	}

	network, address, err := shared.DecodeAddr(&buf)
	if err != nil {
		t.Fatalf("DecodeAddr: %v", err)
	}
	if network != "tcp" {
		t.Errorf("network = %q, want %q", network, "tcp")
	}
	if address != "192.168.1.1:8080" {
		t.Errorf("address = %q, want %q", address, "192.168.1.1:8080")
	}
}

func TestEncodeDecodeAddr_IPv6(t *testing.T) {
	var buf bytes.Buffer
	// net.SplitHostPort requires brackets for IPv6
	if err := shared.EncodeAddr(&buf, "tcp", "[::1]:9090"); err != nil {
		t.Fatalf("EncodeAddr: %v", err)
	}

	network, address, err := shared.DecodeAddr(&buf)
	if err != nil {
		t.Fatalf("DecodeAddr: %v", err)
	}
	if network != "tcp" {
		t.Errorf("network = %q, want %q", network, "tcp")
	}
	if address != "[::1]:9090" {
		t.Errorf("address = %q, want %q", address, "[::1]:9090")
	}
}

func TestEncodeDecodeAddr_UDP(t *testing.T) {
	// EncodeAddr accepts any network string; network is not encoded in the wire format.
	// DecodeAddr always returns "tcp". This verifies roundtrip with UDP source network.
	var buf bytes.Buffer
	if err := shared.EncodeAddr(&buf, "udp", "8.8.8.8:53"); err != nil {
		t.Fatalf("EncodeAddr: %v", err)
	}

	network, address, err := shared.DecodeAddr(&buf)
	if err != nil {
		t.Fatalf("DecodeAddr: %v", err)
	}
	// DecodeAddr returns "tcp" as default; caller is responsible for tracking protocol.
	if network != "tcp" {
		t.Errorf("network = %q, want %q", network, "tcp")
	}
	if address != "8.8.8.8:53" {
		t.Errorf("address = %q, want %q", address, "8.8.8.8:53")
	}
}

func TestEncodeAddr_WireFormat_Domain(t *testing.T) {
	var buf bytes.Buffer
	if err := shared.EncodeAddr(&buf, "tcp", "a.io:80"); err != nil {
		t.Fatalf("EncodeAddr: %v", err)
	}

	b := buf.Bytes()
	// [0x03][len=4][a][.][i][o][0x00][0x50]
	if b[0] != shared.AddrTypeDomain {
		t.Errorf("atype = 0x%02x, want 0x%02x", b[0], shared.AddrTypeDomain)
	}
	if b[1] != 4 {
		t.Errorf("domain length = %d, want 4", b[1])
	}
	if string(b[2:6]) != "a.io" {
		t.Errorf("domain = %q, want %q", string(b[2:6]), "a.io")
	}
	if b[6] != 0x00 || b[7] != 0x50 {
		t.Errorf("port bytes = [0x%02x, 0x%02x], want [0x00, 0x50]", b[6], b[7])
	}
}

func TestEncodeAddr_WireFormat_IPv4(t *testing.T) {
	var buf bytes.Buffer
	if err := shared.EncodeAddr(&buf, "tcp", "1.2.3.4:256"); err != nil {
		t.Fatalf("EncodeAddr: %v", err)
	}

	b := buf.Bytes()
	if b[0] != shared.AddrTypeIPv4 {
		t.Errorf("atype = 0x%02x, want 0x%02x", b[0], shared.AddrTypeIPv4)
	}
	if b[1] != 1 || b[2] != 2 || b[3] != 3 || b[4] != 4 {
		t.Errorf("ipv4 bytes = %v, want [1 2 3 4]", b[1:5])
	}
	// port 256 = 0x01 0x00
	if b[5] != 0x01 || b[6] != 0x00 {
		t.Errorf("port bytes = [0x%02x, 0x%02x], want [0x01, 0x00]", b[5], b[6])
	}
}

func TestEncodeAddr_WireFormat_IPv6(t *testing.T) {
	var buf bytes.Buffer
	if err := shared.EncodeAddr(&buf, "tcp", "[2001:db8::1]:443"); err != nil {
		t.Fatalf("EncodeAddr: %v", err)
	}

	b := buf.Bytes()
	if b[0] != shared.AddrTypeIPv6 {
		t.Errorf("atype = 0x%02x, want 0x%02x", b[0], shared.AddrTypeIPv6)
	}
	// 16 bytes for IPv6 + 2 bytes port = total 19 bytes
	if len(b) != 19 {
		t.Errorf("total encoded length = %d, want 19", len(b))
	}
}

func TestDecodeAddr_UnknownType(t *testing.T) {
	buf := bytes.NewReader([]byte{0x02})
	_, _, err := shared.DecodeAddr(buf)
	if err == nil {
		t.Fatal("expected error for unknown atype 0x02, got nil")
	}
}
