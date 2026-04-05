package config

import "testing"

func TestValidate_InboundEmptyTag(t *testing.T) {
	cfg := validClientConfig()
	cfg.Inbounds = []InboundConfig{{Tag: "", Type: "socks5"}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty inbound tag, got nil")
	}
}

func TestValidate_InboundEmptyType(t *testing.T) {
	cfg := validClientConfig()
	cfg.Inbounds = []InboundConfig{{Tag: "in-1", Type: ""}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty inbound type, got nil")
	}
}

func TestValidate_InboundDuplicateTag(t *testing.T) {
	cfg := validClientConfig()
	cfg.Inbounds = []InboundConfig{
		{Tag: "in-1", Type: "socks5"},
		{Tag: "in-1", Type: "http"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for duplicate inbound tag, got nil")
	}
}

func TestValidate_OutboundReservedTag(t *testing.T) {
	for _, reserved := range []string{"direct", "reject", "proxy"} {
		t.Run(reserved, func(t *testing.T) {
			cfg := validClientConfig()
			cfg.Outbounds = []OutboundConfig{{Tag: reserved, Type: "freedom"}}
			if err := cfg.Validate(); err == nil {
				t.Fatalf("expected error for reserved outbound tag %q, got nil", reserved)
			}
		})
	}
}

func TestValidate_OutboundDuplicateTag(t *testing.T) {
	cfg := validClientConfig()
	cfg.Outbounds = []OutboundConfig{
		{Tag: "out-1", Type: "freedom"},
		{Tag: "out-1", Type: "blackhole"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for duplicate outbound tag, got nil")
	}
}

func TestValidate_RuleChainCustomAction(t *testing.T) {
	// A rule with a custom outbound tag should pass validation.
	cfg := validClientConfig()
	cfg.Routing.RuleChain = []RuleChainEntry{
		{
			Match:  RuleMatch{GeoIP: []string{"JP"}},
			Action: "jp-server",
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error for custom action tag, got: %v", err)
	}

	// An empty action must still be rejected.
	cfg.Routing.RuleChain = []RuleChainEntry{
		{
			Match:  RuleMatch{GeoIP: []string{"JP"}},
			Action: "",
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty action, got nil")
	}

	// Built-in actions must still be accepted.
	for _, builtin := range []string{"proxy", "direct", "reject"} {
		cfg.Routing.RuleChain = []RuleChainEntry{
			{
				Match:  RuleMatch{GeoIP: []string{"JP"}},
				Action: builtin,
			},
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected no error for built-in action %q, got: %v", builtin, err)
		}
	}
}

func TestValidate_InboundValid(t *testing.T) {
	cfg := validClientConfig()
	cfg.Inbounds = []InboundConfig{
		{Tag: "in-socks", Type: "socks5"},
		{Tag: "in-http", Type: "http"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error for valid inbounds: %v", err)
	}
}
