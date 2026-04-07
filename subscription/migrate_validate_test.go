package subscription

import (
	"strings"
	"testing"
)

const sampleClashConfig = `
proxies:
  - name: "ss-server"
    type: ss
    server: 1.2.3.4
    port: 8388
    cipher: aes-256-gcm
    password: secret
  - name: "trojan-server"
    type: trojan
    server: 5.6.7.8
    port: 443
    password: secret2
  - name: "snell-server"
    type: snell
    server: 9.10.11.12
    port: 8080
    psk: psk123

proxy-groups:
  - name: "Proxy"
    type: select
    proxies: ["ss-server", "trojan-server"]
  - name: "Auto"
    type: url-test
    proxies: ["ss-server", "trojan-server"]
    url: "http://www.gstatic.com/generate_204"
    interval: 300
  - name: "Relay"
    type: relay
    proxies: ["ss-server", "trojan-server"]

rules:
  - DOMAIN,example.com,Proxy
  - DOMAIN-SUFFIX,google.com,Proxy
  - GEOIP,CN,DIRECT
  - GEOSITE,cn,DIRECT
  - IP-CIDR,192.168.0.0/16,DIRECT
  - SRC-IP-CIDR,10.0.0.0/8,DIRECT
  - DST-PORT,22,DIRECT
  - SRC-PORT,1024,Proxy
  - RULE-SET,custom,Proxy
  - SUB-RULE,special,Proxy
  - MATCH,DIRECT

dns:
  enable: true
  nameserver:
    - 8.8.8.8
  nameserver-policy:
    "geosite:cn": "114.114.114.114"
  hosts:
    "example.local": "127.0.0.1"
  fake-ip-filter:
    - "*.lan"
`

func TestValidateClashMigration_FullConfig(t *testing.T) {
	report := ValidateClashMigration([]byte(sampleClashConfig))

	if report.ServerCount != 3 {
		t.Errorf("expected 3 servers, got %d", report.ServerCount)
	}
	if report.GroupCount != 3 {
		t.Errorf("expected 3 proxy groups, got %d", report.GroupCount)
	}
	if report.RuleCount != 11 {
		t.Errorf("expected 11 rules, got %d", report.RuleCount)
	}

	// snell should be unsupported
	if !containsStr(report.Unsupported, "snell") {
		t.Errorf("expected 'snell' in unsupported, got: %v", report.Unsupported)
	}

	// SUB-RULE should be unsupported
	if !containsStr(report.Unsupported, "SUB-RULE") {
		t.Errorf("expected 'SUB-RULE' in unsupported, got: %v", report.Unsupported)
	}

	// relay group strategy should be in warnings
	if !containsStr(report.Warnings, "relay") {
		t.Errorf("expected 'relay' in warnings, got: %v", report.Warnings)
	}

	// DNS features should be reported as supported
	if !containsStr(report.Supported, "nameserver-policy") {
		t.Errorf("expected nameserver-policy in supported, got: %v", report.Supported)
	}
	if !containsStr(report.Supported, "hosts table") {
		t.Errorf("expected DNS hosts table in supported, got: %v", report.Supported)
	}
	if !containsStr(report.Supported, "Fake-IP") {
		t.Errorf("expected Fake-IP in supported, got: %v", report.Supported)
	}

	// SRC-IP-CIDR, DST-PORT, SRC-PORT should be supported
	if !containsStr(report.Supported, "SRC-IP-CIDR") {
		t.Errorf("expected SRC-IP-CIDR in supported, got: %v", report.Supported)
	}
	if !containsStr(report.Supported, "DST-PORT") {
		t.Errorf("expected DST-PORT in supported, got: %v", report.Supported)
	}
	if !containsStr(report.Supported, "SRC-PORT") {
		t.Errorf("expected SRC-PORT in supported, got: %v", report.Supported)
	}

	// RULE-SET should be supported
	if !containsStr(report.Supported, "RULE-SET") {
		t.Errorf("expected RULE-SET in supported, got: %v", report.Supported)
	}

	// Always-supported features should be present
	if !containsStr(report.Supported, "Proxy servers import") {
		t.Errorf("expected 'Proxy servers import' in supported, got: %v", report.Supported)
	}
}

func TestValidateClashMigration_InvalidYAML(t *testing.T) {
	report := ValidateClashMigration([]byte("{{not: valid: yaml:::"))
	if len(report.Unsupported) == 0 {
		t.Fatal("expected unsupported entries for invalid YAML")
	}
	if !containsStr(report.Unsupported, "Invalid YAML") {
		t.Errorf("expected 'Invalid YAML' in unsupported, got: %v", report.Unsupported)
	}
}

func TestValidateClashMigration_EmptyConfig(t *testing.T) {
	report := ValidateClashMigration([]byte(""))
	if report.ServerCount != 0 {
		t.Errorf("expected 0 servers, got %d", report.ServerCount)
	}
	if report.GroupCount != 0 {
		t.Errorf("expected 0 groups, got %d", report.GroupCount)
	}
	if report.RuleCount != 0 {
		t.Errorf("expected 0 rules, got %d", report.RuleCount)
	}
	// Always-supported features still present
	if !containsStr(report.Supported, "Proxy servers import") {
		t.Errorf("expected always-supported features, got: %v", report.Supported)
	}
}

func TestValidateClashMigration_AllSupportedProxies(t *testing.T) {
	cfg := `
proxies:
  - name: a
    type: vmess
    server: 1.1.1.1
    port: 443
  - name: b
    type: vless
    server: 2.2.2.2
    port: 443
  - name: c
    type: hysteria2
    server: 3.3.3.3
    port: 443
`
	report := ValidateClashMigration([]byte(cfg))
	if len(report.Unsupported) != 0 {
		t.Errorf("expected no unsupported items, got: %v", report.Unsupported)
	}
	if report.ServerCount != 3 {
		t.Errorf("expected 3 servers, got %d", report.ServerCount)
	}
}

func TestSplitRule(t *testing.T) {
	cases := []struct {
		input    string
		expected []string
	}{
		{"DOMAIN,example.com,PROXY", []string{"DOMAIN", "example.com", "PROXY"}},
		{"MATCH,DIRECT", []string{"MATCH", "DIRECT"}},
		{"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve", []string{"IP-CIDR", "192.168.0.0/16", "DIRECT,no-resolve"}},
		{" GEOIP , CN , DIRECT ", []string{"GEOIP", "CN", "DIRECT"}},
	}
	for _, c := range cases {
		parts := splitRule(c.input)
		if len(parts) != len(c.expected) {
			t.Errorf("splitRule(%q): got %v, want %v", c.input, parts, c.expected)
			continue
		}
		for i, p := range parts {
			if p != c.expected[i] {
				t.Errorf("splitRule(%q)[%d]: got %q, want %q", c.input, i, p, c.expected[i])
			}
		}
	}
}

// containsStr returns true if any element in slice contains substr.
func containsStr(slice []string, substr string) bool {
	for _, s := range slice {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}
