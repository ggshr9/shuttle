package subscription

import (
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
	_, err := ParseURI("vmess://somedata")
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
