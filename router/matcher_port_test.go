package router

import "testing"

func TestPortMatcher_Single(t *testing.T) {
	m, err := newPortMatcher([]string{"80"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		port uint16
		want bool
	}{
		{80, true},
		{443, false},
		{0, false},
	}
	for _, tt := range tests {
		ctx := &MatchContext{Port: tt.port}
		if got := m.Match(ctx); got != tt.want {
			t.Errorf("port %d: got %v, want %v", tt.port, got, tt.want)
		}
	}
}

func TestPortMatcher_Range(t *testing.T) {
	m, err := newPortMatcher([]string{"8080-8090"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		port uint16
		want bool
	}{
		{8079, false},
		{8080, true},
		{8085, true},
		{8090, true},
		{8091, false},
	}
	for _, tt := range tests {
		ctx := &MatchContext{Port: tt.port}
		if got := m.Match(ctx); got != tt.want {
			t.Errorf("port %d: got %v, want %v", tt.port, got, tt.want)
		}
	}
}

func TestPortMatcher_Mixed(t *testing.T) {
	m, err := newPortMatcher([]string{"80", "443", "8080-8090"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		port uint16
		want bool
	}{
		{80, true},
		{443, true},
		{8085, true},
		{22, false},
	}
	for _, tt := range tests {
		ctx := &MatchContext{Port: tt.port}
		if got := m.Match(ctx); got != tt.want {
			t.Errorf("port %d: got %v, want %v", tt.port, got, tt.want)
		}
	}
}

func TestPortMatcher_Invalid(t *testing.T) {
	invalid := [][]string{
		{"abc"},
		{"0"},
		{"99999"},
		{"100-50"}, // min > max
		{},
	}
	for _, specs := range invalid {
		_, err := newPortMatcher(specs)
		if err == nil {
			t.Errorf("expected error for specs %v", specs)
		}
	}
}
