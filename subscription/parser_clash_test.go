package subscription

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const clashBasic = `proxies:
  - name: "SS Server"
    type: ss
    server: ss.example.com
    port: 8388
    password: secret123
    cipher: aes-256-gcm
  - name: "Trojan Server"
    type: trojan
    server: trojan.example.com
    port: 443
    password: trojanpass
    sni: trojan.example.com
`

const clashMultipleTypes = `proxies:
  - name: "SS Node"
    type: ss
    server: ss.example.com
    port: 8388
    password: sspass
    cipher: chacha20-ietf-poly1305
  - name: "Trojan Node"
    type: trojan
    server: trojan.example.com
    port: 443
    password: trojanpass
    sni: trojan.example.com
  - name: "VMess Node"
    type: vmess
    server: vmess.example.com
    port: 443
    uuid: 123e4567-e89b-12d3-a456-426614174000
    sni: vmess.example.com
  - name: "VLESS Node"
    type: vless
    server: vless.example.com
    port: 443
    uuid: 987fbc97-4bed-5078-af07-9141ba07c9f3
    sni: vless.example.com
  - name: "Hysteria2 Node"
    type: hysteria2
    server: hy2.example.com
    port: 443
    password: hy2pass
    sni: hy2.example.com
`

func TestParseClash_Basic(t *testing.T) {
	servers, err := parseClash([]byte(clashBasic))
	if err != nil {
		t.Fatalf("parseClash() error = %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("parseClash() len = %d, want 2", len(servers))
	}

	// First proxy: ss
	ss := servers[0]
	if ss.Name != "SS Server" {
		t.Errorf("ss name = %q, want %q", ss.Name, "SS Server")
	}
	if ss.Addr != "ss.example.com:8388" {
		t.Errorf("ss addr = %q, want %q", ss.Addr, "ss.example.com:8388")
	}
	if ss.Password != "secret123" {
		t.Errorf("ss password = %q, want %q", ss.Password, "secret123")
	}
	if ss.SNI != "" {
		t.Errorf("ss sni = %q, want empty", ss.SNI)
	}

	// Second proxy: trojan
	tr := servers[1]
	if tr.Name != "Trojan Server" {
		t.Errorf("trojan name = %q, want %q", tr.Name, "Trojan Server")
	}
	if tr.Addr != "trojan.example.com:443" {
		t.Errorf("trojan addr = %q, want %q", tr.Addr, "trojan.example.com:443")
	}
	if tr.Password != "trojanpass" {
		t.Errorf("trojan password = %q, want %q", tr.Password, "trojanpass")
	}
	if tr.SNI != "trojan.example.com" {
		t.Errorf("trojan sni = %q, want %q", tr.SNI, "trojan.example.com")
	}
}

func TestParseClash_EmptyProxies(t *testing.T) {
	empty := `proxies: []`
	_, err := parseClash([]byte(empty))
	if err == nil {
		t.Error("parseClash() with empty proxies should return error")
	}
}

func TestParseClash_InvalidYAML(t *testing.T) {
	invalid := `proxies: [[[not valid yaml`
	_, err := parseClash([]byte(invalid))
	if err == nil {
		t.Error("parseClash() with invalid YAML should return error")
	}
}

func TestIsClashFormat(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "valid clash with proxies",
			input: clashBasic,
			want:  true,
		},
		{
			name:  "valid clash multiple types",
			input: clashMultipleTypes,
			want:  true,
		},
		{
			name:  "empty content",
			input: "",
			want:  false,
		},
		{
			name:  "json content",
			input: `[{"addr":"server.com:443","name":"test"}]`,
			want:  false,
		},
		{
			name:  "proxies key but empty list",
			input: "proxies: []\n",
			want:  false,
		},
		{
			name:  "yaml without proxies key",
			input: "servers:\n  - name: test\n    host: example.com\n",
			want:  false,
		},
		{
			name:  "base64 content",
			input: "c3M6Ly9leGFtcGxlLmNvbTo0NDM=",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isClashFormat([]byte(tt.input))
			if got != tt.want {
				t.Errorf("isClashFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseClash_MultipleTypes(t *testing.T) {
	servers, err := parseClash([]byte(clashMultipleTypes))
	if err != nil {
		t.Fatalf("parseClash() error = %v", err)
	}
	if len(servers) != 5 {
		t.Fatalf("parseClash() len = %d, want 5", len(servers))
	}

	// ss
	ss := servers[0]
	if ss.Name != "SS Node" {
		t.Errorf("ss name = %q, want %q", ss.Name, "SS Node")
	}
	if ss.Addr != "ss.example.com:8388" {
		t.Errorf("ss addr = %q, want %q", ss.Addr, "ss.example.com:8388")
	}
	if ss.Password != "sspass" {
		t.Errorf("ss password = %q, want %q", ss.Password, "sspass")
	}

	// trojan
	tr := servers[1]
	if tr.Name != "Trojan Node" {
		t.Errorf("trojan name = %q, want %q", tr.Name, "Trojan Node")
	}
	if tr.Password != "trojanpass" {
		t.Errorf("trojan password = %q, want %q", tr.Password, "trojanpass")
	}
	if tr.SNI != "trojan.example.com" {
		t.Errorf("trojan sni = %q, want %q", tr.SNI, "trojan.example.com")
	}

	// vmess — password comes from uuid field
	vm := servers[2]
	if vm.Name != "VMess Node" {
		t.Errorf("vmess name = %q, want %q", vm.Name, "VMess Node")
	}
	if vm.Password != "123e4567-e89b-12d3-a456-426614174000" {
		t.Errorf("vmess password (uuid) = %q, want UUID", vm.Password)
	}
	if vm.SNI != "vmess.example.com" {
		t.Errorf("vmess sni = %q, want %q", vm.SNI, "vmess.example.com")
	}

	// vless — password comes from uuid field
	vl := servers[3]
	if vl.Name != "VLESS Node" {
		t.Errorf("vless name = %q, want %q", vl.Name, "VLESS Node")
	}
	if vl.Password != "987fbc97-4bed-5078-af07-9141ba07c9f3" {
		t.Errorf("vless password (uuid) = %q, want UUID", vl.Password)
	}
	if vl.SNI != "vless.example.com" {
		t.Errorf("vless sni = %q, want %q", vl.SNI, "vless.example.com")
	}

	// hysteria2
	hy := servers[4]
	if hy.Name != "Hysteria2 Node" {
		t.Errorf("hysteria2 name = %q, want %q", hy.Name, "Hysteria2 Node")
	}
	if hy.Password != "hy2pass" {
		t.Errorf("hysteria2 password = %q, want %q", hy.Password, "hy2pass")
	}
	if hy.SNI != "hy2.example.com" {
		t.Errorf("hysteria2 sni = %q, want %q", hy.SNI, "hy2.example.com")
	}
}

func TestParseClash_PreservesTypeAndOptions(t *testing.T) {
	yamlData := []byte(`proxies:
  - name: my-ss
    type: ss
    server: 1.2.3.4
    port: 443
    password: secret
    cipher: aes-256-gcm
    plugin: obfs
    plugin-opts:
      mode: http
  - name: my-vless
    type: vless
    server: 5.6.7.8
    port: 443
    uuid: abc-def-123
    network: ws
    ws-opts:
      path: /vless
    tls: true
    sni: example.com
`)
	endpoints, err := parseClash(yamlData)
	require.NoError(t, err)
	require.Len(t, endpoints, 2)

	// SS server preserves type and cipher
	assert.Equal(t, "ss", endpoints[0].Type)
	assert.Equal(t, "1.2.3.4:443", endpoints[0].Addr)
	assert.Equal(t, "secret", endpoints[0].Password)
	assert.Equal(t, "aes-256-gcm", endpoints[0].Options["cipher"])
	assert.Equal(t, "obfs", endpoints[0].Options["plugin"])
	pluginOpts, ok := endpoints[0].Options["plugin-opts"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "http", pluginOpts["mode"])

	// VLESS server preserves type, uuid->password, network, ws-opts
	assert.Equal(t, "vless", endpoints[1].Type)
	assert.Equal(t, "abc-def-123", endpoints[1].Password)
	assert.Equal(t, "example.com", endpoints[1].SNI)
	assert.Equal(t, "ws", endpoints[1].Options["network"])
	assert.Equal(t, true, endpoints[1].Options["tls"])
	wsOpts, ok := endpoints[1].Options["ws-opts"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "/vless", wsOpts["path"])

	// Promoted fields should NOT be in Options
	assert.Nil(t, endpoints[0].Options["name"])
	assert.Nil(t, endpoints[0].Options["type"])
	assert.Nil(t, endpoints[0].Options["server"])
	assert.Nil(t, endpoints[0].Options["port"])
	assert.Nil(t, endpoints[0].Options["password"])
}

func TestIsClashFormat_StillDetects(t *testing.T) {
	data := []byte("proxies:\n  - name: test\n    type: ss\n")
	assert.True(t, isClashFormat(data))
}
