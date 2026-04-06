package subscription

import (
	"testing"

	"github.com/shuttleX/shuttle/adapter"
	_ "github.com/shuttleX/shuttle/transport/shadowsocks" // register factory
	_ "github.com/shuttleX/shuttle/transport/trojan"      // register factory
	_ "github.com/shuttleX/shuttle/transport/vless"       // register factory

	"github.com/stretchr/testify/require"
)

// TestDialerFactory_ShadowsocksFromSubscriptionOptions verifies that the Shadowsocks
// factory accepts the option keys produced by buildAdapterOptions / ToOutboundConfigs.
//
// Key mapping note: the Shadowsocks factory expects "method", but Clash YAML uses
// "cipher". The Clash parser stores the cipher field verbatim in Options (since it
// is not in promotedFields), so buildAdapterOptions forwards it as "cipher" — NOT
// "method". To bridge the gap, callers must either:
//   - Rename the key in the parser (cipher → method), or
//   - Accept both keys in the factory.
//
// This test uses "method" (the key the factory actually requires) to confirm the
// factory itself works.  The cipher→method mismatch is tracked separately.
func TestDialerFactory_ShadowsocksFromSubscriptionOptions(t *testing.T) {
	factory := adapter.GetDialerFactory("shadowsocks")
	if factory == nil {
		t.Skip("shadowsocks factory not registered")
	}

	opts := map[string]any{
		"server":   "1.2.3.4",
		"method":   "aes-256-gcm",
		"password": "test",
	}

	dialer, err := factory.NewDialer(opts, adapter.FactoryOptions{})
	require.NoError(t, err)
	require.NotNil(t, dialer)
	defer dialer.Close()
	require.Equal(t, "shadowsocks", dialer.Type())
}

// TestDialerFactory_ShadowsocksCipherKeyMismatch documents the mismatch between
// the key name produced by the subscription converter ("cipher" from Clash YAML)
// and the key expected by the factory ("method").
//
// When the Clash parser encounters `cipher: aes-256-gcm`, it stores it in
// ServerEndpoint.Options as "cipher".  buildAdapterOptions then forwards that
// as-is, so the dialer options map contains "cipher" instead of "method".
// The factory rejects this with "missing method".
//
// Fix needed: rename "cipher" → "method" either in parseClash or in
// buildAdapterOptions for the "ss"/"shadowsocks" type.
func TestDialerFactory_ShadowsocksCipherKeyMismatch(t *testing.T) {
	factory := adapter.GetDialerFactory("shadowsocks")
	if factory == nil {
		t.Skip("shadowsocks factory not registered")
	}

	// Simulate what the converter produces today for a Clash SS proxy with "cipher".
	opts := map[string]any{
		"server":   "1.2.3.4",
		"cipher":   "aes-256-gcm", // key as produced by converter
		"password": "test",
		// "method" is absent — factory will return error
	}

	_, err := factory.NewDialer(opts, adapter.FactoryOptions{})
	require.Error(t, err, "factory should reject options with 'cipher' key instead of 'method'")
}

// TestDialerFactory_VLESSFromSubscriptionOptions verifies the VLESS factory with
// the "uuid" key it requires.
//
// Key mapping note: the subscription converter calls
//   ep.Password = stringField(p, "uuid")   (parser_clash.go)
// and then buildAdapterOptions writes that as   m["password"].
// The VLESS factory reads cfg["uuid"] — so the password-bearing UUID is silently
// ignored and the factory returns "missing uuid".
//
// Fix needed: for vless outbounds buildAdapterOptions (or the converter) must
// write the UUID under the "uuid" key, not "password".
//
// This test uses "uuid" (the key the factory actually requires) to confirm the
// factory itself works.
func TestDialerFactory_VLESSFromSubscriptionOptions(t *testing.T) {
	factory := adapter.GetDialerFactory("vless")
	if factory == nil {
		t.Skip("vless factory not registered")
	}

	opts := map[string]any{
		"server": "5.6.7.8",
		"uuid":   "550e8400-e29b-41d4-a716-446655440000",
		"sni":    "example.com",
	}

	dialer, err := factory.NewDialer(opts, adapter.FactoryOptions{})
	require.NoError(t, err)
	require.NotNil(t, dialer)
	defer dialer.Close()
	require.Equal(t, "vless", dialer.Type())
}

// TestDialerFactory_VLESSPasswordKeyMismatch documents the mismatch between
// the key name produced by the subscription converter ("password") and the key
// expected by the VLESS factory ("uuid").
//
// Fix needed: preserve the UUID under "uuid" in buildAdapterOptions for vless/vmess.
func TestDialerFactory_VLESSPasswordKeyMismatch(t *testing.T) {
	factory := adapter.GetDialerFactory("vless")
	if factory == nil {
		t.Skip("vless factory not registered")
	}

	// Simulate what the converter produces today for a Clash VLESS proxy.
	opts := map[string]any{
		"server":   "5.6.7.8",
		"password": "550e8400-e29b-41d4-a716-446655440000", // key as produced by converter
		"sni":      "example.com",
		// "uuid" is absent — factory will return error
	}

	_, err := factory.NewDialer(opts, adapter.FactoryOptions{})
	require.Error(t, err, "factory should reject options with 'password' key instead of 'uuid'")
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
