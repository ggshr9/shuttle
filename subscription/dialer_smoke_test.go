package subscription

import (
	"encoding/json"
	"testing"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/config"
	_ "github.com/shuttleX/shuttle/transport/shadowsocks" // register factory
	_ "github.com/shuttleX/shuttle/transport/trojan"      // register factory
	_ "github.com/shuttleX/shuttle/transport/vless"       // register factory

	"github.com/stretchr/testify/require"
)

// TestDialerFactory_ShadowsocksFromSubscriptionOptions verifies that the Shadowsocks
// factory accepts the option keys produced by buildAdapterOptions / ToOutboundConfigs.
//
// buildAdapterOptions now normalizes "cipher" → "method" for shadowsocks adapters,
// so a Clash YAML entry with `cipher: aes-256-gcm` correctly reaches the factory
// as "method".  This test simulates the full converter path using "cipher" as input.
func TestDialerFactory_ShadowsocksFromSubscriptionOptions(t *testing.T) {
	factory := adapter.GetDialerFactory("shadowsocks")
	if factory == nil {
		t.Skip("shadowsocks factory not registered")
	}

	// Simulate what the converter now produces for a Clash SS proxy: "cipher" is
	// normalized to "method" by buildAdapterOptions before reaching the factory.
	opts := map[string]any{
		"server":   "1.2.3.4",
		"method":   "aes-256-gcm", // normalized from "cipher" by buildAdapterOptions
		"password": "test",
	}

	dialer, err := factory.NewDialer(opts, adapter.FactoryOptions{})
	require.NoError(t, err)
	require.NotNil(t, dialer)
	defer dialer.Close()
	require.Equal(t, "shadowsocks", dialer.Type())
}

// TestDialerFactory_ShadowsocksCipherNormalized verifies end-to-end that
// buildAdapterOptions renames "cipher" to "method" for shadowsocks adapters.
// This was previously a documented mismatch; the fix is now in place.
func TestDialerFactory_ShadowsocksCipherNormalized(t *testing.T) {
	factory := adapter.GetDialerFactory("shadowsocks")
	if factory == nil {
		t.Skip("shadowsocks factory not registered")
	}

	// Simulate the full converter pipeline: ServerEndpoint with "cipher" in Options.
	ep := config.ServerEndpoint{
		Name:     "test-ss",
		Type:     "ss",
		Addr:     "1.2.3.4:8388",
		Password: "test",
		Options:  map[string]any{"cipher": "aes-256-gcm"},
	}
	raw := buildAdapterOptions(&ep, "shadowsocks")

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Equal(t, "aes-256-gcm", got["method"], "cipher should be normalized to method")
	require.NotContains(t, got, "cipher", "cipher key should be removed after normalization")

	// The factory must accept the normalized options.
	dialer, err := factory.NewDialer(got, adapter.FactoryOptions{})
	require.NoError(t, err)
	require.NotNil(t, dialer)
	dialer.Close()
}

// TestDialerFactory_VLESSFromSubscriptionOptions verifies that the VLESS factory
// accepts the option keys produced by buildAdapterOptions / ToOutboundConfigs.
//
// buildAdapterOptions now writes ServerEndpoint.Password as "uuid" for vless/vmess
// adapters, matching what the VLESS factory reads from cfg["uuid"].
func TestDialerFactory_VLESSFromSubscriptionOptions(t *testing.T) {
	factory := adapter.GetDialerFactory("vless")
	if factory == nil {
		t.Skip("vless factory not registered")
	}

	// Simulate the full converter pipeline: ServerEndpoint.Password is written
	// as "uuid" by buildAdapterOptions for vless adapters.
	opts := map[string]any{
		"server": "5.6.7.8",
		"uuid":   "550e8400-e29b-41d4-a716-446655440000", // normalized from Password by buildAdapterOptions
		"sni":    "example.com",
	}

	dialer, err := factory.NewDialer(opts, adapter.FactoryOptions{})
	require.NoError(t, err)
	require.NotNil(t, dialer)
	defer dialer.Close()
	require.Equal(t, "vless", dialer.Type())
}

// TestDialerFactory_VLESSPasswordNormalized verifies end-to-end that
// buildAdapterOptions writes ServerEndpoint.Password as "uuid" for vless adapters.
// This was previously a documented mismatch; the fix is now in place.
func TestDialerFactory_VLESSPasswordNormalized(t *testing.T) {
	factory := adapter.GetDialerFactory("vless")
	if factory == nil {
		t.Skip("vless factory not registered")
	}

	// Simulate the full converter pipeline: ServerEndpoint with UUID in Password.
	ep := config.ServerEndpoint{
		Name:     "test-vless",
		Type:     "vless",
		Addr:     "5.6.7.8:443",
		Password: "550e8400-e29b-41d4-a716-446655440000",
		SNI:      "example.com",
	}
	raw := buildAdapterOptions(&ep, "vless")

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Equal(t, "550e8400-e29b-41d4-a716-446655440000", got["uuid"], "password should be normalized to uuid")
	require.NotContains(t, got, "password", "password key should not appear for vless")

	// The factory must accept the normalized options.
	dialer, err := factory.NewDialer(got, adapter.FactoryOptions{})
	require.NoError(t, err)
	require.NotNil(t, dialer)
	dialer.Close()
}

// TestDialerFactory_TrojanFromSubscriptionOptions verifies that the Trojan factory
// accepts the option keys produced by buildAdapterOptions / ToOutboundConfigs.
//
// The Trojan factory uses "server", "password", and optionally "sni" — which
// matches exactly what buildAdapterOptions produces.  No key mismatch exists for
// Trojan.
func TestDialerFactory_TrojanFromSubscriptionOptions(t *testing.T) {
	factory := adapter.GetDialerFactory("trojan")
	if factory == nil {
		t.Skip("trojan factory not registered")
	}

	opts := map[string]any{
		"server":   "9.10.11.12",
		"password": "trojan-pass",
		"sni":      "example.com",
	}

	dialer, err := factory.NewDialer(opts, adapter.FactoryOptions{})
	require.NoError(t, err)
	require.NotNil(t, dialer)
	defer dialer.Close()
	require.Equal(t, "trojan", dialer.Type())
}
