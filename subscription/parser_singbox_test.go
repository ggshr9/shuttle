package subscription

import (
	"testing"
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
