package subscription

import (
	"encoding/json"
	"testing"

	"github.com/shuttleX/shuttle/config"
)

func TestToOutboundConfigs_Basic(t *testing.T) {
	servers := []config.ServerEndpoint{
		{Name: "Server One", Addr: "1.2.3.4:443"},
		{Name: "Server Two", Addr: "5.6.7.8:443"},
	}

	out := ToOutboundConfigs(servers)
	if len(out) != 2 {
		t.Fatalf("expected 2 outbounds, got %d", len(out))
	}

	// Check first outbound.
	if out[0].Tag != "server-one" {
		t.Errorf("expected tag %q, got %q", "server-one", out[0].Tag)
	}
	if out[0].Type != "proxy" {
		t.Errorf("expected type %q, got %q", "proxy", out[0].Type)
	}
	var opts0 map[string]string
	if err := json.Unmarshal(out[0].Options, &opts0); err != nil {
		t.Fatalf("unmarshal options[0]: %v", err)
	}
	if opts0["server"] != "1.2.3.4:443" {
		t.Errorf("expected server %q, got %q", "1.2.3.4:443", opts0["server"])
	}

	// Check second outbound.
	if out[1].Tag != "server-two" {
		t.Errorf("expected tag %q, got %q", "server-two", out[1].Tag)
	}
	var opts1 map[string]string
	if err := json.Unmarshal(out[1].Options, &opts1); err != nil {
		t.Fatalf("unmarshal options[1]: %v", err)
	}
	if opts1["server"] != "5.6.7.8:443" {
		t.Errorf("expected server %q, got %q", "5.6.7.8:443", opts1["server"])
	}
}

func TestToOutboundConfigs_EmptyName(t *testing.T) {
	servers := []config.ServerEndpoint{
		{Name: "", Addr: "10.0.0.1:8080"},
	}

	out := ToOutboundConfigs(servers)
	if len(out) != 1 {
		t.Fatalf("expected 1 outbound, got %d", len(out))
	}

	// Tag should be derived from addr (colons → dashes).
	expected := "10.0.0.1-8080"
	if out[0].Tag != expected {
		t.Errorf("expected tag %q, got %q", expected, out[0].Tag)
	}
	if out[0].Type != "proxy" {
		t.Errorf("expected type %q, got %q", "proxy", out[0].Type)
	}
	var opts map[string]string
	if err := json.Unmarshal(out[0].Options, &opts); err != nil {
		t.Fatalf("unmarshal options: %v", err)
	}
	if opts["server"] != "10.0.0.1:8080" {
		t.Errorf("expected server %q, got %q", "10.0.0.1:8080", opts["server"])
	}
}

func TestToOutboundConfigs_DuplicateTags(t *testing.T) {
	servers := []config.ServerEndpoint{
		{Name: "dup", Addr: "1.1.1.1:443"},
		{Name: "dup", Addr: "2.2.2.2:443"},
		{Name: "dup", Addr: "3.3.3.3:443"},
	}

	out := ToOutboundConfigs(servers)
	if len(out) != 3 {
		t.Fatalf("expected 3 outbounds, got %d", len(out))
	}

	if out[0].Tag != "dup" {
		t.Errorf("expected first tag %q, got %q", "dup", out[0].Tag)
	}
	if out[1].Tag != "dup-2" {
		t.Errorf("expected second tag %q, got %q", "dup-2", out[1].Tag)
	}
	if out[2].Tag != "dup-3" {
		t.Errorf("expected third tag %q, got %q", "dup-3", out[2].Tag)
	}
}

func TestToGroupConfig(t *testing.T) {
	outbounds := []config.OutboundConfig{
		{Tag: "server-one", Type: "proxy"},
		{Tag: "server-two", Type: "proxy"},
	}

	group := ToGroupConfig("my-group", outbounds)

	if group.Tag != "my-group" {
		t.Errorf("expected tag %q, got %q", "my-group", group.Tag)
	}
	if group.Type != "group" {
		t.Errorf("expected type %q, got %q", "group", group.Type)
	}

	var opts struct {
		Strategy    string   `json:"strategy"`
		Outbounds   []string `json:"outbounds"`
		MaxLatency  string   `json:"max_latency"`
		MaxLossRate float64  `json:"max_loss_rate"`
	}
	if err := json.Unmarshal(group.Options, &opts); err != nil {
		t.Fatalf("unmarshal group options: %v", err)
	}

	if opts.Strategy != "quality" {
		t.Errorf("expected strategy %q, got %q", "quality", opts.Strategy)
	}
	if len(opts.Outbounds) != 2 {
		t.Fatalf("expected 2 outbounds in group, got %d", len(opts.Outbounds))
	}
	if opts.Outbounds[0] != "server-one" || opts.Outbounds[1] != "server-two" {
		t.Errorf("unexpected outbound tags: %v", opts.Outbounds)
	}
	if opts.MaxLatency != "500ms" {
		t.Errorf("expected max_latency %q, got %q", "500ms", opts.MaxLatency)
	}
	if opts.MaxLossRate != 0.05 {
		t.Errorf("expected max_loss_rate 0.05, got %v", opts.MaxLossRate)
	}
}
