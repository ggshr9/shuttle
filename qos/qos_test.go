package qos

import (
	"testing"

	"github.com/shuttle-proxy/shuttle/config"
)

func TestClassifier_Disabled(t *testing.T) {
	c := NewClassifier(nil)
	if c.Enabled() {
		t.Error("classifier should be disabled when config is nil")
	}

	c = NewClassifier(&config.QoSConfig{Enabled: false})
	if c.Enabled() {
		t.Error("classifier should be disabled when Enabled=false")
	}

	// Should return normal priority when disabled
	if p := c.ClassifyPort(22); p != PriorityNormal {
		t.Errorf("expected PriorityNormal for disabled classifier, got %d", p)
	}
}

func TestClassifier_DefaultPorts(t *testing.T) {
	c := NewClassifier(&config.QoSConfig{Enabled: true})

	tests := []struct {
		port     uint16
		expected Priority
	}{
		{22, PriorityCritical},   // SSH
		{3389, PriorityCritical}, // RDP
		{5060, PriorityHigh},     // SIP
		{6881, PriorityLow},      // BitTorrent
		{80, PriorityNormal},     // HTTP (no default rule)
		{443, PriorityNormal},    // HTTPS (no default rule)
	}

	for _, tt := range tests {
		got := c.ClassifyPort(tt.port)
		if got != tt.expected {
			t.Errorf("port %d: expected priority %d, got %d", tt.port, tt.expected, got)
		}
	}
}

func TestClassifier_CustomRules(t *testing.T) {
	cfg := &config.QoSConfig{
		Enabled: true,
		Rules: []config.QoSRule{
			{
				Ports:    []uint16{443, 8443},
				Priority: "high",
			},
			{
				Ports:    []uint16{21},
				Protocol: "tcp",
				Priority: "bulk",
			},
			{
				Process:  []string{"chrome", "firefox"},
				Priority: "normal",
			},
			{
				Domains:  []string{"youtube.com", "netflix.com"},
				Priority: "high",
			},
		},
	}

	c := NewClassifier(cfg)

	// Port-based rules
	if p := c.ClassifyPort(443); p != PriorityHigh {
		t.Errorf("port 443: expected PriorityHigh, got %d", p)
	}
	if p := c.ClassifyPort(8443); p != PriorityHigh {
		t.Errorf("port 8443: expected PriorityHigh, got %d", p)
	}

	// Full classification with protocol
	if p := c.Classify(21, "tcp", "", ""); p != PriorityBulk {
		t.Errorf("port 21 tcp: expected PriorityBulk, got %d", p)
	}

	// Process-based classification
	if p := c.Classify(80, "tcp", "", "chrome"); p != PriorityNormal {
		t.Errorf("chrome: expected PriorityNormal, got %d", p)
	}
	if p := c.Classify(80, "tcp", "", "Firefox"); p != PriorityNormal {
		t.Errorf("Firefox (case insensitive): expected PriorityNormal, got %d", p)
	}

	// Domain-based classification
	if p := c.Classify(443, "tcp", "www.youtube.com", ""); p != PriorityHigh {
		t.Errorf("youtube.com: expected PriorityHigh, got %d", p)
	}
	if p := c.Classify(443, "tcp", "netflix.com", ""); p != PriorityHigh {
		t.Errorf("netflix.com: expected PriorityHigh, got %d", p)
	}
}

func TestDSCPToTOS(t *testing.T) {
	tests := []struct {
		dscp     uint8
		expected uint8
	}{
		{DSCPExpedited, 184},  // 46 << 2 = 184
		{DSCPAF41, 136},       // 34 << 2 = 136
		{DSCPAF21, 72},        // 18 << 2 = 72
		{DSCPAF11, 40},        // 10 << 2 = 40
		{DSCPBestEffort, 0},   // 0 << 2 = 0
	}

	for _, tt := range tests {
		got := DSCPToTOS(tt.dscp)
		if got != tt.expected {
			t.Errorf("DSCPToTOS(%d): expected %d, got %d", tt.dscp, tt.expected, got)
		}
	}
}

func TestClassifier_GetTOS(t *testing.T) {
	c := NewClassifier(&config.QoSConfig{Enabled: true})

	tests := []struct {
		priority Priority
		expected uint8
	}{
		{PriorityCritical, 184}, // EF
		{PriorityHigh, 136},     // AF41
		{PriorityNormal, 72},    // AF21
		{PriorityBulk, 40},      // AF11
		{PriorityLow, 0},        // BE
	}

	for _, tt := range tests {
		got := c.GetTOS(tt.priority)
		if got != tt.expected {
			t.Errorf("GetTOS(%d): expected %d, got %d", tt.priority, tt.expected, got)
		}
	}
}

func TestClassifier_Update(t *testing.T) {
	c := NewClassifier(nil)
	if c.Enabled() {
		t.Error("initially should be disabled")
	}

	// Update with enabled config
	c.Update(&config.QoSConfig{
		Enabled: true,
		Rules: []config.QoSRule{
			{Ports: []uint16{8080}, Priority: "critical"},
		},
	})

	if !c.Enabled() {
		t.Error("should be enabled after update")
	}
	if p := c.ClassifyPort(8080); p != PriorityCritical {
		t.Errorf("port 8080: expected PriorityCritical, got %d", p)
	}

	// Update with disabled config
	c.Update(&config.QoSConfig{Enabled: false})
	if c.Enabled() {
		t.Error("should be disabled after update")
	}
}

func TestParsePriority(t *testing.T) {
	tests := []struct {
		input    string
		expected Priority
	}{
		{"critical", PriorityCritical},
		{"Critical", PriorityCritical},
		{"CRITICAL", PriorityCritical},
		{"high", PriorityHigh},
		{"normal", PriorityNormal},
		{"bulk", PriorityBulk},
		{"low", PriorityLow},
		{"unknown", PriorityNormal}, // default
		{"", PriorityNormal},        // default
	}

	for _, tt := range tests {
		got := parsePriority(tt.input)
		if got != tt.expected {
			t.Errorf("parsePriority(%q): expected %d, got %d", tt.input, tt.expected, got)
		}
	}
}
