package subscription

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shuttleX/shuttle/config"
)

func TestFullPipeline_ClashSubscriptionToOutboundConfigs(t *testing.T) {
	clashYAML := []byte(`proxies:
  - name: jp-ss-01
    type: ss
    server: jp1.example.com
    port: 443
    password: hunter2
    cipher: aes-256-gcm
  - name: us-vless-01
    type: vless
    server: us1.example.com
    port: 443
    uuid: 550e8400-e29b-41d4-a716-446655440000
    network: ws
    ws-opts:
      path: /vless-ws
    tls: true
    sni: cdn.example.com
  - name: hk-trojan-01
    type: trojan
    server: hk1.example.com
    port: 443
    password: trojan-pass
    sni: hk1.example.com
`)

	// Step 1: Parse
	endpoints, err := ParseSubscription(string(clashYAML))
	require.NoError(t, err)
	require.Len(t, endpoints, 3)

	// Step 2: Verify type preservation
	assert.Equal(t, "ss", endpoints[0].Type)
	assert.Equal(t, "vless", endpoints[1].Type)
	assert.Equal(t, "trojan", endpoints[2].Type)

	// Step 3: Convert to outbound configs
	configs := ToOutboundConfigs(endpoints)
	require.Len(t, configs, 3)

	// SS → shadowsocks type with cipher
	assert.Equal(t, "shadowsocks", configs[0].Type)
	var ssOpts map[string]any
	require.NoError(t, json.Unmarshal(configs[0].Options, &ssOpts))
	assert.Equal(t, "aes-256-gcm", ssOpts["method"]) // cipher normalized to method by buildAdapterOptions
	assert.Equal(t, "jp1.example.com", ssOpts["server"])
	assert.Equal(t, "hunter2", ssOpts["password"])

	// VLESS → vless with ws-opts
	assert.Equal(t, "vless", configs[1].Type)
	var vlessOpts map[string]any
	require.NoError(t, json.Unmarshal(configs[1].Options, &vlessOpts))
	assert.Equal(t, "ws", vlessOpts["network"])
	assert.Equal(t, "cdn.example.com", vlessOpts["sni"])

	// Trojan → trojan
	assert.Equal(t, "trojan", configs[2].Type)
	var trojanOpts map[string]any
	require.NoError(t, json.Unmarshal(configs[2].Options, &trojanOpts))
	assert.Equal(t, "trojan-pass", trojanOpts["password"])
}

func TestFullPipeline_SingboxSubscriptionToOutboundConfigs(t *testing.T) {
	singboxJSON := []byte(`{
		"outbounds": [
			{
				"type": "shadowsocks",
				"tag": "sg-ss",
				"server": "sg1.example.com",
				"server_port": 8388,
				"method": "chacha20-ietf-poly1305",
				"password": "sspass"
			},
			{
				"type": "direct",
				"tag": "direct"
			}
		]
	}`)

	endpoints, err := ParseSubscription(string(singboxJSON))
	require.NoError(t, err)
	require.Len(t, endpoints, 1) // direct skipped

	assert.Equal(t, "shadowsocks", endpoints[0].Type)
	assert.Equal(t, "chacha20-ietf-poly1305", endpoints[0].Options["method"])

	configs := ToOutboundConfigs(endpoints)
	require.Len(t, configs, 1)
	assert.Equal(t, "shadowsocks", configs[0].Type)

	var opts map[string]any
	require.NoError(t, json.Unmarshal(configs[0].Options, &opts))
	assert.Equal(t, "chacha20-ietf-poly1305", opts["method"])
	assert.Equal(t, "sg1.example.com", opts["server"])
}

func TestFullPipeline_MixedShuttleAndThirdParty(t *testing.T) {
	// Test that empty Type → "proxy" fallback works in the pipeline
	endpoints := []config.ServerEndpoint{
		{Addr: "my.shuttle.server:443", Name: "shuttle-1", Password: "pw", Type: ""},
		{Addr: "ss.example.com:8388", Name: "ss-1", Password: "secret", Type: "ss", Options: map[string]any{"cipher": "aes-256-gcm"}},
	}

	configs := ToOutboundConfigs(endpoints)
	assert.Equal(t, "proxy", configs[0].Type)
	assert.Equal(t, "shadowsocks", configs[1].Type)
}
