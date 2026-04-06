package subscription

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestParseURI_ShadowsocksSIP002(t *testing.T) {
	// SIP002 format: ss://method:password@host:port#name
	uri := "ss://chacha20-ietf-poly1305:mypassword@1.2.3.4:8388#My%20Server"
	node, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node.Type != "ss" {
		t.Errorf("Type = %q, want %q", node.Type, "ss")
	}
	if node.Server != "1.2.3.4" {
		t.Errorf("Server = %q, want %q", node.Server, "1.2.3.4")
	}
	if node.Port != 8388 {
		t.Errorf("Port = %d, want %d", node.Port, 8388)
	}
	if node.Name != "My Server" {
		t.Errorf("Name = %q, want %q", node.Name, "My Server")
	}
	if node.Options["method"] != "chacha20-ietf-poly1305" {
		t.Errorf("Options[method] = %v, want %q", node.Options["method"], "chacha20-ietf-poly1305")
	}
	if node.Options["password"] != "mypassword" {
		t.Errorf("Options[password] = %v, want %q", node.Options["password"], "mypassword")
	}
}

func TestParseURI_ShadowsocksLegacy(t *testing.T) {
	// Legacy format: ss://base64(method:password)@host:port#name
	// base64("aes-256-gcm:testpass") = "YWVzLTI1Ni1nY206dGVzdHBhc3M="
	// We construct this dynamically to avoid encoding errors.
	import_b64 := "YWVzLTI1Ni1nY206dGVzdHBhc3NAMS4yLjMuNDo0NDM=" // aes-256-gcm:testpass@1.2.3.4:443
	uri := "ss://" + import_b64 + "#Legacy"
	node, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node.Type != "ss" {
		t.Errorf("Type = %q, want %q", node.Type, "ss")
	}
	if node.Server != "1.2.3.4" {
		t.Errorf("Server = %q, want %q", node.Server, "1.2.3.4")
	}
	if node.Port != 443 {
		t.Errorf("Port = %d, want %d", node.Port, 443)
	}
	if node.Name != "Legacy" {
		t.Errorf("Name = %q, want %q", node.Name, "Legacy")
	}
	if node.Options["method"] != "aes-256-gcm" {
		t.Errorf("Options[method] = %v", node.Options["method"])
	}
	if node.Options["password"] != "testpass" {
		t.Errorf("Options[password] = %v", node.Options["password"])
	}
}

func TestParseURI_VLESS(t *testing.T) {
	uri := "vless://550e8400-e29b-41d4-a716-446655440000@example.com:443?type=tcp&security=tls&sni=example.com#VLESS%20Node"
	node, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node.Type != "vless" {
		t.Errorf("Type = %q, want %q", node.Type, "vless")
	}
	if node.Server != "example.com" {
		t.Errorf("Server = %q, want %q", node.Server, "example.com")
	}
	if node.Port != 443 {
		t.Errorf("Port = %d, want %d", node.Port, 443)
	}
	if node.Name != "VLESS Node" {
		t.Errorf("Name = %q, want %q", node.Name, "VLESS Node")
	}
	if node.Options["uuid"] != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("Options[uuid] = %v", node.Options["uuid"])
	}
	if node.Options["sni"] != "example.com" {
		t.Errorf("Options[sni] = %v", node.Options["sni"])
	}
	if node.Options["security"] != "tls" {
		t.Errorf("Options[security] = %v", node.Options["security"])
	}
}

func TestParseURI_Trojan(t *testing.T) {
	uri := "trojan://s3cr3tpassword@vpn.example.com:443?sni=vpn.example.com#Trojan%20Node"
	node, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node.Type != "trojan" {
		t.Errorf("Type = %q, want %q", node.Type, "trojan")
	}
	if node.Server != "vpn.example.com" {
		t.Errorf("Server = %q, want %q", node.Server, "vpn.example.com")
	}
	if node.Port != 443 {
		t.Errorf("Port = %d, want %d", node.Port, 443)
	}
	if node.Name != "Trojan Node" {
		t.Errorf("Name = %q, want %q", node.Name, "Trojan Node")
	}
	if node.Options["password"] != "s3cr3tpassword" {
		t.Errorf("Options[password] = %v", node.Options["password"])
	}
	if node.Options["sni"] != "vpn.example.com" {
		t.Errorf("Options[sni] = %v", node.Options["sni"])
	}
}

func TestParseURI_UnsupportedScheme(t *testing.T) {
	_, err := ParseURI("unknown://somedata")
	if err == nil {
		t.Fatal("expected error for unsupported scheme, got nil")
	}
}

func TestParseURI_SSMissingPassword(t *testing.T) {
	// A URI without userinfo should fail
	_, err := ParseURI("ss://1.2.3.4:8388")
	if err == nil {
		t.Fatal("expected error for missing method/password, got nil")
	}
}

func TestParseURI_VLESSMissingUUID(t *testing.T) {
	_, err := ParseURI("vless://example.com:443")
	if err == nil {
		t.Fatal("expected error for missing UUID, got nil")
	}
}

func TestParseURI_TrojanMissingPassword(t *testing.T) {
	_, err := ParseURI("trojan://example.com:443")
	if err == nil {
		t.Fatal("expected error for missing password, got nil")
	}
}

func TestParseURI_Hysteria2(t *testing.T) {
	uri := "hy2://s3cr3tpass@hy2.example.com:443?sni=hy2.example.com&insecure=0#Hysteria2%20Node"
	node, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node.Type != "hysteria2" {
		t.Errorf("Type = %q, want %q", node.Type, "hysteria2")
	}
	if node.Server != "hy2.example.com" {
		t.Errorf("Server = %q, want %q", node.Server, "hy2.example.com")
	}
	if node.Port != 443 {
		t.Errorf("Port = %d, want %d", node.Port, 443)
	}
	if node.Name != "Hysteria2 Node" {
		t.Errorf("Name = %q, want %q", node.Name, "Hysteria2 Node")
	}
	if node.Options["password"] != "s3cr3tpass" {
		t.Errorf("Options[password] = %v, want %q", node.Options["password"], "s3cr3tpass")
	}
	if node.Options["sni"] != "hy2.example.com" {
		t.Errorf("Options[sni] = %v, want %q", node.Options["sni"], "hy2.example.com")
	}
}

func TestParseURI_Hysteria2Scheme(t *testing.T) {
	// Test the full hysteria2:// scheme (not the hy2:// alias)
	uri := "hysteria2://mypassword@192.168.1.1:8443?sni=example.com#HY2-Full"
	node, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node.Type != "hysteria2" {
		t.Errorf("Type = %q, want %q", node.Type, "hysteria2")
	}
	if node.Server != "192.168.1.1" {
		t.Errorf("Server = %q, want %q", node.Server, "192.168.1.1")
	}
	if node.Port != 8443 {
		t.Errorf("Port = %d, want %d", node.Port, 8443)
	}
	if node.Name != "HY2-Full" {
		t.Errorf("Name = %q, want %q", node.Name, "HY2-Full")
	}
	if node.Options["password"] != "mypassword" {
		t.Errorf("Options[password] = %v", node.Options["password"])
	}
}

func TestParseURI_VMess(t *testing.T) {
	// Build a valid vmess base64 JSON payload dynamically.
	payload := map[string]interface{}{
		"ps":   "VMess Node",
		"add":  "vmess.example.com",
		"port": 443,
		"id":   "550e8400-e29b-41d4-a716-446655440000",
		"scy":  "auto",
		"net":  "tcp",
		"host": "",
		"path": "",
		"tls":  "tls",
		"sni":  "vmess.example.com",
	}
	raw, _ := json.Marshal(payload)
	b64 := base64.StdEncoding.EncodeToString(raw)
	uri := "vmess://" + b64

	node, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node.Type != "vmess" {
		t.Errorf("Type = %q, want %q", node.Type, "vmess")
	}
	if node.Server != "vmess.example.com" {
		t.Errorf("Server = %q, want %q", node.Server, "vmess.example.com")
	}
	if node.Port != 443 {
		t.Errorf("Port = %d, want %d", node.Port, 443)
	}
	if node.Name != "VMess Node" {
		t.Errorf("Name = %q, want %q", node.Name, "VMess Node")
	}
	if node.Options["uuid"] != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("Options[uuid] = %v", node.Options["uuid"])
	}
	if node.Options["sni"] != "vmess.example.com" {
		t.Errorf("Options[sni] = %v", node.Options["sni"])
	}
	if node.Options["tls"] != "tls" {
		t.Errorf("Options[tls] = %v", node.Options["tls"])
	}
	if node.Options["cipher"] != "auto" {
		t.Errorf("Options[cipher] = %v", node.Options["cipher"])
	}
}

func TestParseURI_VMessStringPort(t *testing.T) {
	// Port encoded as a JSON string (some clients emit this).
	payload := map[string]interface{}{
		"ps":   "VMess String Port",
		"add":  "1.2.3.4",
		"port": "8080",
		"id":   "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
	}
	raw, _ := json.Marshal(payload)
	b64 := base64.StdEncoding.EncodeToString(raw)
	node, err := ParseURI("vmess://" + b64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node.Port != 8080 {
		t.Errorf("Port = %d, want 8080", node.Port)
	}
}
