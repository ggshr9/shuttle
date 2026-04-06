package provider

import (
	"encoding/base64"
	"strings"
	"testing"
)

// ── DetectFormat ──────────────────────────────────────────────────────────────

func TestDetectFormat_ClashYAML(t *testing.T) {
	data := []byte(`proxies:
  - name: "SG-1"
    type: ss
    server: sg.example.com
    port: 8388
    password: secret
    cipher: chacha20-ietf-poly1305
`)
	if got := DetectFormat(data); got != FormatClash {
		t.Fatalf("expected FormatClash, got %q", got)
	}
}

func TestDetectFormat_SingboxJSON(t *testing.T) {
	data := []byte(`{
  "outbounds": [
    {"type": "shadowsocks", "tag": "sg-ss", "server": "sg.example.com", "server_port": 8388}
  ]
}`)
	if got := DetectFormat(data); got != FormatSingbox {
		t.Fatalf("expected FormatSingbox, got %q", got)
	}
}

func TestDetectFormat_PlainURI(t *testing.T) {
	data := []byte("ss://Y2hhY2hhMjA6c2VjcmV0QHNnLmV4YW1wbGUuY29tOjgzODg=#SG-1\ntrojan://pass@hk.example.com:443#HK-1\n")
	if got := DetectFormat(data); got != FormatPlainURI {
		t.Fatalf("expected FormatPlainURI, got %q", got)
	}
}

func TestDetectFormat_Base64URI(t *testing.T) {
	raw := "ss://Y2hhY2hhMjA6c2VjcmV0QHNnLmV4YW1wbGUuY29tOjgzODg=#SG-1\ntrojan://pass@hk.example.com:443#HK-1"
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	data := []byte(encoded)
	if got := DetectFormat(data); got != FormatBase64URI {
		t.Fatalf("expected FormatBase64URI, got %q", got)
	}
}

func TestDetectFormat_Unknown(t *testing.T) {
	data := []byte("just some random text that is not a proxy list")
	if got := DetectFormat(data); got != FormatUnknown {
		t.Fatalf("expected FormatUnknown, got %q", got)
	}
}

// ── ParseProxyList ────────────────────────────────────────────────────────────

func TestParseProxyList_ClashFormat(t *testing.T) {
	data := []byte(`proxies:
  - name: "SG-1"
    type: ss
    server: sg.example.com
    port: 8388
    password: secret
    cipher: chacha20-ietf-poly1305
  - name: "HK-1"
    type: trojan
    server: hk.example.com
    port: 443
    password: trojanpass
    sni: hk.example.com
`)
	nodes, err := ParseProxyList(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	// First node: SG-1
	sg := nodes[0]
	if sg.Name != "SG-1" {
		t.Errorf("node[0].Name = %q, want SG-1", sg.Name)
	}
	if sg.Type != "ss" {
		t.Errorf("node[0].Type = %q, want ss", sg.Type)
	}
	if sg.Server != "sg.example.com" {
		t.Errorf("node[0].Server = %q, want sg.example.com", sg.Server)
	}
	if sg.Port != 8388 {
		t.Errorf("node[0].Port = %d, want 8388", sg.Port)
	}
	if sg.Options["cipher"] != "chacha20-ietf-poly1305" {
		t.Errorf("node[0].Options[cipher] = %v, want chacha20-ietf-poly1305", sg.Options["cipher"])
	}

	// Second node: HK-1
	hk := nodes[1]
	if hk.Name != "HK-1" {
		t.Errorf("node[1].Name = %q, want HK-1", hk.Name)
	}
	if hk.Type != "trojan" {
		t.Errorf("node[1].Type = %q, want trojan", hk.Type)
	}
}

func TestParseProxyList_SingboxFormat(t *testing.T) {
	data := []byte(`{
  "outbounds": [
    {
      "type": "shadowsocks",
      "tag": "SG-SS",
      "server": "sg.example.com",
      "server_port": 8388,
      "password": "secret",
      "method": "chacha20-ietf-poly1305"
    },
    {
      "type": "direct",
      "tag": "direct"
    }
  ]
}`)
	nodes, err := ParseProxyList(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "direct" should be skipped
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node (direct skipped), got %d", len(nodes))
	}
	n := nodes[0]
	if n.Name != "SG-SS" {
		t.Errorf("Name = %q, want SG-SS", n.Name)
	}
	if n.Server != "sg.example.com" {
		t.Errorf("Server = %q, want sg.example.com", n.Server)
	}
	if n.Port != 8388 {
		t.Errorf("Port = %d, want 8388", n.Port)
	}
}

func TestParseProxyList_PlainURI(t *testing.T) {
	data := []byte("ss://example1\ntrojan://example2\n\n")
	nodes, err := ParseProxyList(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	for _, n := range nodes {
		if n.Type != "raw-uri" {
			t.Errorf("Type = %q, want raw-uri", n.Type)
		}
		if n.Options["uri"] == "" {
			t.Errorf("Options[uri] is empty")
		}
	}
}

func TestParseProxyList_Base64URI(t *testing.T) {
	raw := "ss://line1\nvless://line2"
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	nodes, err := ParseProxyList([]byte(encoded))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
}

func TestParseProxyList_Unknown(t *testing.T) {
	_, err := ParseProxyList([]byte("this is not a proxy list"))
	if err == nil {
		t.Fatal("expected error for unknown format, got nil")
	}
}

func TestParseClashProxies_EmptyProxies(t *testing.T) {
	data := []byte("proxies: []\n")
	// isClashYAML returns true but parseClashProxies should error on empty
	_, err := ParseProxyList(data)
	if err == nil {
		t.Fatal("expected error for empty proxies list")
	}
}

func TestDetectFormat_ClashWithoutProxiesKey(t *testing.T) {
	// A YAML without "proxies:" should not be detected as Clash
	data := []byte("rules:\n  - DOMAIN,example.com,DIRECT\n")
	if got := DetectFormat(data); got == FormatClash {
		t.Fatalf("should not detect as Clash without proxies key")
	}
}

func TestParseURIList_AllEmpty(t *testing.T) {
	data := []byte("   \n\n  \n")
	// isPlainURIList → false, falls through to unknown
	if got := DetectFormat(data); got != FormatUnknown {
		t.Fatalf("expected FormatUnknown for whitespace-only input, got %q", got)
	}
}

func TestDetectFormat_SingboxPriority(t *testing.T) {
	// A JSON that also happens to contain "proxies" should be detected as singbox.
	data := []byte(`{"outbounds": [], "proxies": []}`)
	if got := DetectFormat(data); got != FormatSingbox {
		t.Fatalf("expected FormatSingbox, got %q", got)
	}
}

// Ensure the singbox parser skips all non-proxy outbound types.
func TestParseSingboxOutbounds_SkipNonProxy(t *testing.T) {
	skipTypes := []string{"direct", "block", "dns", "selector", "urltest"}
	for _, typ := range skipTypes {
		data := []byte(`{"outbounds":[{"type":"` + typ + `","tag":"t","server":"s.com","server_port":1}]}`)
		nodes, err := parseSingboxOutbounds(data)
		if err != nil {
			t.Errorf("type %q: unexpected error %v", typ, err)
		}
		if len(nodes) != 0 {
			t.Errorf("type %q: expected 0 nodes, got %d", typ, len(nodes))
		}
	}
}

// Clash proxies key may appear with inline value "proxies: []"
func TestIsClashYAML_InlineEmpty(t *testing.T) {
	data := []byte("proxies: []\n")
	if !isClashYAML(data) {
		t.Error("expected isClashYAML to return true for 'proxies: []'")
	}
}

func TestParseClashProxies_PortAsFloat(t *testing.T) {
	// Ensure port cast from float64 (json/yaml sometimes returns float64 for numbers)
	data := []byte(`proxies:
  - name: "Test"
    type: ss
    server: test.com
    port: 1234
    password: pw
`)
	nodes, err := parseClashProxies(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nodes[0].Port != 1234 {
		t.Errorf("Port = %d, want 1234", nodes[0].Port)
	}
}

func TestParseURIList_TrimsWhitespace(t *testing.T) {
	data := []byte("  ss://line1  \n  trojan://line2  \n")
	nodes, err := parseURIList(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	if !strings.HasPrefix(nodes[0].Options["uri"].(string), "ss://") {
		t.Errorf("URI not trimmed: %v", nodes[0].Options["uri"])
	}
}
