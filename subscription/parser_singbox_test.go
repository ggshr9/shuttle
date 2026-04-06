package subscription

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSingbox_Basic(t *testing.T) {
	input := []byte(`{
		"outbounds": [
			{
				"type": "shadowsocks",
				"tag": "ss-node",
				"server": "ss.example.com",
				"server_port": 8388,
				"method": "aes-256-gcm",
				"password": "secret123"
			},
			{
				"type": "trojan",
				"tag": "trojan-node",
				"server": "trojan.example.com",
				"server_port": 443,
				"password": "trojanpass"
			}
		]
	}`)

	servers, err := parseSingbox(input)
	if err != nil {
		t.Fatalf("parseSingbox() error = %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("parseSingbox() len = %d, want 2", len(servers))
	}

	if servers[0].Addr != "ss.example.com:8388" {
		t.Errorf("servers[0].Addr = %q, want %q", servers[0].Addr, "ss.example.com:8388")
	}
	if servers[0].Name != "ss-node" {
		t.Errorf("servers[0].Name = %q, want %q", servers[0].Name, "ss-node")
	}
	if servers[0].Password != "secret123" {
		t.Errorf("servers[0].Password = %q, want %q", servers[0].Password, "secret123")
	}

	if servers[1].Addr != "trojan.example.com:443" {
		t.Errorf("servers[1].Addr = %q, want %q", servers[1].Addr, "trojan.example.com:443")
	}
	if servers[1].Name != "trojan-node" {
		t.Errorf("servers[1].Name = %q, want %q", servers[1].Name, "trojan-node")
	}
	if servers[1].Password != "trojanpass" {
		t.Errorf("servers[1].Password = %q, want %q", servers[1].Password, "trojanpass")
	}
}

func TestParseSingbox_SkipNonProxy(t *testing.T) {
	input := []byte(`{
		"outbounds": [
			{
				"type": "direct",
				"tag": "direct-out"
			},
			{
				"type": "block",
				"tag": "block-out"
			},
			{
				"type": "dns",
				"tag": "dns-out"
			},
			{
				"type": "selector",
				"tag": "selector-out"
			},
			{
				"type": "urltest",
				"tag": "urltest-out"
			},
			{
				"type": "vmess",
				"tag": "vmess-node",
				"server": "vmess.example.com",
				"server_port": 443,
				"password": "vmesspass"
			}
		]
	}`)

	servers, err := parseSingbox(input)
	if err != nil {
		t.Fatalf("parseSingbox() error = %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("parseSingbox() len = %d, want 1 (non-proxy types filtered)", len(servers))
	}
	if servers[0].Name != "vmess-node" {
		t.Errorf("servers[0].Name = %q, want %q", servers[0].Name, "vmess-node")
	}
}

func TestParseSingbox_WithTLS(t *testing.T) {
	input := []byte(`{
		"outbounds": [
			{
				"type": "trojan",
				"tag": "trojan-tls",
				"server": "tls.example.com",
				"server_port": 443,
				"password": "tlspass",
				"tls": {
					"server_name": "sni.example.com"
				}
			}
		]
	}`)

	servers, err := parseSingbox(input)
	if err != nil {
		t.Fatalf("parseSingbox() error = %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("parseSingbox() len = %d, want 1", len(servers))
	}
	if servers[0].SNI != "sni.example.com" {
		t.Errorf("servers[0].SNI = %q, want %q", servers[0].SNI, "sni.example.com")
	}
	if servers[0].Addr != "tls.example.com:443" {
		t.Errorf("servers[0].Addr = %q, want %q", servers[0].Addr, "tls.example.com:443")
	}
}

func TestIsSingboxFormat(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "valid sing-box with outbounds",
			input: `{"outbounds": [{"type": "direct", "tag": "direct"}]}`,
			want:  true,
		},
		{
			name:  "sing-box with outbounds and other fields",
			input: `{"log": {}, "outbounds": [], "route": {}}`,
			want:  true,
		},
		{
			name:  "SIP008 format (no outbounds key)",
			input: `{"version": 1, "servers": []}`,
			want:  false,
		},
		{
			name:  "plain JSON array (not sing-box)",
			input: `[{"addr": "example.com:443"}]`,
			want:  false,
		},
		{
			name:  "invalid JSON",
			input: `not json at all`,
			want:  false,
		},
		{
			name:  "empty object",
			input: `{}`,
			want:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isSingboxFormat([]byte(tc.input))
			if got != tc.want {
				t.Errorf("isSingboxFormat() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseSingbox_TLSBlockInOptions(t *testing.T) {
	// The full TLS block must pass through in Options so factories can read
	// enabled, insecure, alpn, etc. SNI is also extracted to ep.SNI.
	input := []byte(`{
		"outbounds": [
			{
				"type": "trojan",
				"tag": "trojan-full-tls",
				"server": "tls.example.com",
				"server_port": 443,
				"password": "pass",
				"tls": {
					"enabled": true,
					"server_name": "sni.example.com",
					"insecure": false,
					"alpn": ["h2", "http/1.1"]
				}
			}
		]
	}`)

	servers, err := parseSingbox(input)
	require.NoError(t, err)
	require.Len(t, servers, 1)

	ep := servers[0]
	// SNI extracted to top-level field.
	assert.Equal(t, "sni.example.com", ep.SNI)

	// Full TLS block must be present in Options.
	require.NotNil(t, ep.Options, "Options must not be nil when tls block is present")
	tlsVal, ok := ep.Options["tls"]
	require.True(t, ok, "tls key must be present in Options")
	tlsMap, ok := tlsVal.(map[string]any)
	require.True(t, ok, "tls value must be a map")
	assert.Equal(t, true, tlsMap["enabled"])
	assert.Equal(t, false, tlsMap["insecure"])
	alpn, ok := tlsMap["alpn"].([]any)
	require.True(t, ok, "alpn must be a slice")
	assert.Equal(t, []any{"h2", "http/1.1"}, alpn)
}

func TestParseSingbox_PreservesTypeAndOptions(t *testing.T) {
	jsonData := []byte(`{
		"outbounds": [
			{
				"type": "shadowsocks",
				"tag": "ss-out",
				"server": "1.2.3.4",
				"server_port": 443,
				"method": "aes-256-gcm",
				"password": "secret",
				"multiplex": {"enabled": true, "protocol": "smux"}
			},
			{
				"type": "vless",
				"tag": "vless-out",
				"server": "5.6.7.8",
				"server_port": 443,
				"uuid": "abc-123",
				"flow": "xtls-rprx-vision",
				"tls": {"enabled": true, "server_name": "example.com"}
			},
			{
				"type": "direct",
				"tag": "direct-out"
			}
		]
	}`)
	endpoints, err := parseSingbox(jsonData)
	require.NoError(t, err)
	require.Len(t, endpoints, 2) // direct is skipped

	// SS
	assert.Equal(t, "shadowsocks", endpoints[0].Type)
	assert.Equal(t, "aes-256-gcm", endpoints[0].Options["method"])
	mux, ok := endpoints[0].Options["multiplex"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, mux["enabled"])

	// VLESS
	assert.Equal(t, "vless", endpoints[1].Type)
	assert.Equal(t, "abc-123", endpoints[1].Password)
	assert.Equal(t, "example.com", endpoints[1].SNI)
	assert.Equal(t, "xtls-rprx-vision", endpoints[1].Options["flow"])

	// Promoted fields NOT in Options
	assert.Nil(t, endpoints[0].Options["type"])
	assert.Nil(t, endpoints[0].Options["tag"])
	assert.Nil(t, endpoints[0].Options["server"])
	assert.Nil(t, endpoints[0].Options["server_port"])
	assert.Nil(t, endpoints[0].Options["password"])
}
