package engine

import (
	"context"
	"testing"

	"github.com/ggshr9/shuttle/adapter"
)

func TestSelectState_SelectAndSelected(t *testing.T) {
	a := &mockOutbound{tag: "a", dialFunc: succeedDial}
	b := &mockOutbound{tag: "b", dialFunc: succeedDial}

	s := newSelectState([]adapter.Outbound{a, b})

	// Default: first outbound is selected.
	if got := s.Selected(); got != "a" {
		t.Errorf("initial Selected() = %q, want %q", got, "a")
	}

	// Switch to b.
	if err := s.Select("b"); err != nil {
		t.Fatalf("Select(\"b\"): %v", err)
	}
	if got := s.Selected(); got != "b" {
		t.Errorf("after Select(\"b\"), Selected() = %q, want %q", got, "b")
	}

	// Switch back to a.
	if err := s.Select("a"); err != nil {
		t.Fatalf("Select(\"a\"): %v", err)
	}
	if got := s.Selected(); got != "a" {
		t.Errorf("after Select(\"a\"), Selected() = %q, want %q", got, "a")
	}
}

func TestSelectState_Select_UnknownTag(t *testing.T) {
	a := &mockOutbound{tag: "a", dialFunc: succeedDial}
	s := newSelectState([]adapter.Outbound{a})

	err := s.Select("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown tag, got nil")
	}
}

func TestSelectState_Empty(t *testing.T) {
	s := newSelectState([]adapter.Outbound{})
	if got := s.Selected(); got != "" {
		t.Errorf("empty state Selected() = %q, want %q", got, "")
	}
}

func TestOutboundGroup_Select_DialUsesSelectedNode(t *testing.T) {
	a := &mockOutbound{tag: "a", dialFunc: succeedDial}
	b := &mockOutbound{tag: "b", dialFunc: succeedDial}

	g := NewOutboundGroup("sel", GroupSelect, []adapter.Outbound{a, b})
	g.SetSelect([]adapter.Outbound{a, b})

	// Default selection is "a".
	conn, err := g.DialContext(context.Background(), "tcp", "example.com:80")
	if err != nil {
		t.Fatalf("dial with default selection: %v", err)
	}
	conn.Close()

	if a.calls.Load() != 1 {
		t.Errorf("a calls = %d, want 1 (should be selected by default)", a.calls.Load())
	}
	if b.calls.Load() != 0 {
		t.Errorf("b calls = %d, want 0", b.calls.Load())
	}

	// Switch to "b" and dial again.
	if err := g.SelectOutbound("b"); err != nil {
		t.Fatalf("SelectOutbound(\"b\"): %v", err)
	}

	conn, err = g.DialContext(context.Background(), "tcp", "example.com:80")
	if err != nil {
		t.Fatalf("dial after selecting b: %v", err)
	}
	conn.Close()

	if a.calls.Load() != 1 {
		t.Errorf("a calls = %d, want 1 (should not have been called again)", a.calls.Load())
	}
	if b.calls.Load() != 1 {
		t.Errorf("b calls = %d, want 1", b.calls.Load())
	}
}

func TestOutboundGroup_SelectOutbound_UnknownTag(t *testing.T) {
	a := &mockOutbound{tag: "a", dialFunc: succeedDial}
	g := NewOutboundGroup("sel", GroupSelect, []adapter.Outbound{a})
	g.SetSelect([]adapter.Outbound{a})

	err := g.SelectOutbound("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown tag, got nil")
	}
}

func TestOutboundGroup_SelectedOutbound(t *testing.T) {
	a := &mockOutbound{tag: "a", dialFunc: succeedDial}
	b := &mockOutbound{tag: "b", dialFunc: succeedDial}

	g := NewOutboundGroup("sel", GroupSelect, []adapter.Outbound{a, b})
	g.SetSelect([]adapter.Outbound{a, b})

	if got := g.SelectedOutbound(); got != "a" {
		t.Errorf("SelectedOutbound() = %q, want %q", got, "a")
	}

	if err := g.SelectOutbound("b"); err != nil {
		t.Fatalf("SelectOutbound(\"b\"): %v", err)
	}

	if got := g.SelectedOutbound(); got != "b" {
		t.Errorf("after switch, SelectedOutbound() = %q, want %q", got, "b")
	}
}

func TestOutboundGroup_SelectedOutbound_NoSelectState(t *testing.T) {
	// A group without SetSelect should return "" from SelectedOutbound.
	a := &mockOutbound{tag: "a", dialFunc: succeedDial}
	g := NewOutboundGroup("fo", GroupFailover, []adapter.Outbound{a})

	if got := g.SelectedOutbound(); got != "" {
		t.Errorf("SelectedOutbound() without selectState = %q, want %q", got, "")
	}
}

func TestOutboundGroup_SelectOutbound_NoSelectState(t *testing.T) {
	// SelectOutbound on a non-select group should return an error.
	a := &mockOutbound{tag: "a", dialFunc: succeedDial}
	g := NewOutboundGroup("fo", GroupFailover, []adapter.Outbound{a})

	err := g.SelectOutbound("a")
	if err == nil {
		t.Fatal("expected error when calling SelectOutbound on non-select group")
	}
}

func TestOutboundGroup_Select_DialNoSelection(t *testing.T) {
	// An empty group with select strategy and nil selected outbound should error.
	g := NewOutboundGroup("sel-empty", GroupSelect, []adapter.Outbound{})
	g.SetSelect([]adapter.Outbound{})

	_, err := g.DialContext(context.Background(), "tcp", "example.com:80")
	if err == nil {
		t.Fatal("expected error when no outbound is selected")
	}
}

func TestOutboundGroup_Select_NoSelectState_FallsBackToFailover(t *testing.T) {
	// If GroupSelect but selectState is not set, falls back to failover.
	a := &mockOutbound{tag: "a", dialFunc: succeedDial}
	g := NewOutboundGroup("sel", GroupSelect, []adapter.Outbound{a})
	// Do NOT call g.SetSelect — selectState remains nil.

	conn, err := g.DialContext(context.Background(), "tcp", "example.com:80")
	if err != nil {
		t.Fatalf("expected fallover success, got %v", err)
	}
	conn.Close()

	if a.calls.Load() != 1 {
		t.Errorf("a calls = %d, want 1", a.calls.Load())
	}
}
