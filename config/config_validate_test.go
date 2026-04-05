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
