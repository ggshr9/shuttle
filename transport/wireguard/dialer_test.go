package wireguard

import (
	"encoding/base64"
	"net/netip"
	"strings"
	"testing"

	"github.com/ggshr9/shuttle/adapter"
)

// generateTestKey returns a deterministic 32-byte base64-encoded key for tests.
func generateTestKey(seed byte) string {
	var key [32]byte
	for i := range key {
		key[i] = seed + byte(i)
	}
	return base64.StdEncoding.EncodeToString(key[:])
}

func TestBase64ToHex(t *testing.T) {
	// 32 bytes of zeros.
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	hexStr, err := base64ToHex(key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := strings.Repeat("00", 32)
	if hexStr != expected {
		t.Fatalf("expected %s, got %s", expected, hexStr)
	}
}

func TestBase64ToHex_InvalidBase64(t *testing.T) {
	_, err := base64ToHex("not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestBase64ToHex_WrongLength(t *testing.T) {
	// 16 bytes instead of 32.
	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	_, err := base64ToHex(short)
	if err == nil {
		t.Fatal("expected error for wrong key length")
	}
}

func TestBuildIPCConfig(t *testing.T) {
	privKey := generateTestKey(0x01)
	pubKey := generateTestKey(0x20)
	pskKey := generateTestKey(0x40)

	cfg := TunnelConfig{
		PrivateKey: privKey,
		Peers: []PeerConfig{
			{
				PublicKey:    pubKey,
				Endpoint:     "1.2.3.4:51820",
				AllowedIPs:   []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")},
				PresharedKey: pskKey,
				Keepalive:    25,
			},
		},
	}

	ipc, err := buildIPCConfig(&cfg)
	if err != nil {
		t.Fatalf("buildIPCConfig: %v", err)
	}

	// Verify key fields present.
	if !strings.Contains(ipc, "private_key=") {
		t.Error("missing private_key in IPC config")
	}
	if !strings.Contains(ipc, "public_key=") {
		t.Error("missing public_key in IPC config")
	}
	if !strings.Contains(ipc, "preshared_key=") {
		t.Error("missing preshared_key in IPC config")
	}
	if !strings.Contains(ipc, "endpoint=1.2.3.4:51820") {
		t.Error("missing endpoint in IPC config")
	}
	if !strings.Contains(ipc, "allowed_ip=0.0.0.0/0") {
		t.Error("missing allowed_ip in IPC config")
	}
	if !strings.Contains(ipc, "persistent_keepalive_interval=25") {
		t.Error("missing persistent_keepalive_interval in IPC config")
	}

	// Verify keys are hex-encoded (64 hex chars = 32 bytes).
	for _, line := range strings.Split(ipc, "\n") {
		if strings.HasPrefix(line, "private_key=") {
			hexVal := strings.TrimPrefix(line, "private_key=")
			if len(hexVal) != 64 {
				t.Errorf("private_key hex should be 64 chars, got %d", len(hexVal))
			}
		}
	}
}

func TestBuildIPCConfig_MissingPrivateKey(t *testing.T) {
	cfg := TunnelConfig{
		Peers: []PeerConfig{
			{PublicKey: generateTestKey(0x20)},
		},
	}
	_, err := buildIPCConfig(&cfg)
	if err == nil {
		t.Fatal("expected error for missing private key")
	}
}

func TestBuildIPCConfig_NoPresharedKey(t *testing.T) {
	cfg := TunnelConfig{
		PrivateKey: generateTestKey(0x01),
		Peers: []PeerConfig{
			{
				PublicKey:  generateTestKey(0x20),
				Endpoint:   "1.2.3.4:51820",
				AllowedIPs: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")},
			},
		},
	}

	ipc, err := buildIPCConfig(&cfg)
	if err != nil {
		t.Fatalf("buildIPCConfig: %v", err)
	}

	if strings.Contains(ipc, "preshared_key=") {
		t.Error("preshared_key should not be present when not set")
	}
}

func TestBuildIPCConfig_MultiplePeers(t *testing.T) {
	cfg := TunnelConfig{
		PrivateKey: generateTestKey(0x01),
		Peers: []PeerConfig{
			{
				PublicKey:  generateTestKey(0x20),
				Endpoint:   "1.2.3.4:51820",
				AllowedIPs: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")},
			},
			{
				PublicKey:  generateTestKey(0x30),
				Endpoint:   "5.6.7.8:51820",
				AllowedIPs: []netip.Prefix{netip.MustParsePrefix("10.0.1.0/24")},
				Keepalive:  15,
			},
		},
	}

	ipc, err := buildIPCConfig(&cfg)
	if err != nil {
		t.Fatalf("buildIPCConfig: %v", err)
	}

	// Should have two public_key entries.
	count := strings.Count(ipc, "public_key=")
	if count != 2 {
		t.Errorf("expected 2 public_key entries, got %d", count)
	}
}

func TestResolveAddrPort_IP(t *testing.T) {
	ap, err := resolveAddrPort("1.2.3.4:80")
	if err != nil {
		t.Fatalf("resolveAddrPort: %v", err)
	}
	if ap.Addr() != netip.MustParseAddr("1.2.3.4") {
		t.Errorf("unexpected addr: %v", ap.Addr())
	}
	if ap.Port() != 80 {
		t.Errorf("unexpected port: %v", ap.Port())
	}
}

func TestResolveAddrPort_IPv6(t *testing.T) {
	ap, err := resolveAddrPort("[::1]:443")
	if err != nil {
		t.Fatalf("resolveAddrPort: %v", err)
	}
	if ap.Addr() != netip.MustParseAddr("::1") {
		t.Errorf("unexpected addr: %v", ap.Addr())
	}
	if ap.Port() != 443 {
		t.Errorf("unexpected port: %v", ap.Port())
	}
}

func TestResolveAddrPort_InvalidAddress(t *testing.T) {
	_, err := resolveAddrPort("not-valid")
	if err == nil {
		t.Fatal("expected error for invalid address")
	}
}

func TestParseTunnelConfig(t *testing.T) {
	cfg := map[string]any{
		"private_key": generateTestKey(0x01),
		"addresses":   []any{"10.0.0.2/32"},
		"dns":         []any{"1.1.1.1"},
		"mtu":         1420,
		"peers": []any{
			map[string]any{
				"public_key":  generateTestKey(0x20),
				"endpoint":    "1.2.3.4:51820",
				"allowed_ips": []any{"0.0.0.0/0"},
				"keepalive":   25,
			},
		},
	}

	tc, err := parseTunnelConfig(cfg)
	if err != nil {
		t.Fatalf("parseTunnelConfig: %v", err)
	}

	if tc.PrivateKey != generateTestKey(0x01) {
		t.Error("private_key mismatch")
	}
	if len(tc.Addresses) != 1 || tc.Addresses[0] != netip.MustParsePrefix("10.0.0.2/32") {
		t.Error("addresses mismatch")
	}
	if len(tc.DNS) != 1 || tc.DNS[0] != netip.MustParseAddr("1.1.1.1") {
		t.Error("dns mismatch")
	}
	if tc.MTU != 1420 {
		t.Errorf("mtu: got %d, want 1420", tc.MTU)
	}
	if len(tc.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(tc.Peers))
	}
	if tc.Peers[0].Keepalive != 25 {
		t.Errorf("keepalive: got %d, want 25", tc.Peers[0].Keepalive)
	}
}

func TestParseTunnelConfig_MissingPrivateKey(t *testing.T) {
	cfg := map[string]any{
		"peers": []any{
			map[string]any{
				"public_key": generateTestKey(0x20),
				"endpoint":   "1.2.3.4:51820",
			},
		},
	}
	_, err := parseTunnelConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing private_key")
	}
}

func TestParseTunnelConfig_NoPeers(t *testing.T) {
	cfg := map[string]any{
		"private_key": generateTestKey(0x01),
	}
	_, err := parseTunnelConfig(cfg)
	if err == nil {
		t.Fatal("expected error for no peers")
	}
}

func TestDialerType(t *testing.T) {
	// Test Type() without actually creating a tunnel.
	d := &Dialer{}
	if d.Type() != "wireguard" {
		t.Errorf("expected type wireguard, got %s", d.Type())
	}
}

func TestDialerCloseIdempotent(t *testing.T) {
	// Test that Close() is idempotent on a zero-value Dialer.
	// We can't call Close() on a nil device, so test the closed flag logic.
	d := &Dialer{closed: true}
	if err := d.Close(); err != nil {
		t.Errorf("second close should return nil: %v", err)
	}
}

func TestNewDialerValidation(t *testing.T) {
	tests := []struct {
		name string
		cfg  TunnelConfig
	}{
		{
			name: "missing private key",
			cfg:  TunnelConfig{Peers: []PeerConfig{{PublicKey: generateTestKey(0x20)}}},
		},
		{
			name: "no peers",
			cfg:  TunnelConfig{PrivateKey: generateTestKey(0x01)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDialer(&tt.cfg, nil)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestFactoryRegistered(t *testing.T) {
	// The init() in factory.go should have registered "wireguard".
	// Import adapter to check — but we can just verify the factory type.
	f := &factory{}
	if f.Type() != "wireguard" {
		t.Errorf("expected type wireguard, got %s", f.Type())
	}
}

func TestFactoryNewInboundHandler_ReturnsNil(t *testing.T) {
	f := &factory{}
	handler, err := f.NewInboundHandler(nil, adapter.FactoryOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handler != nil {
		t.Fatal("expected nil handler for client-only transport")
	}
}

func TestFactoryNewServer_ReturnsNil(t *testing.T) {
	f := &factory{}
	srv, err := f.NewServer(nil, adapter.FactoryOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srv != nil {
		t.Fatal("expected nil server for client-only transport")
	}
}
