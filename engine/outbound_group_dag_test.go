package engine

import (
	"strings"
	"testing"
)

func TestValidateGroupDAG_NoCycle(t *testing.T) {
	groups := map[string][]string{
		"A": {"proxy-1", "proxy-2"},
		"B": {"A", "proxy-3"},
		"C": {"B", "direct"},
	}
	if err := validateGroupDAG(groups); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateGroupDAG_DirectCycle(t *testing.T) {
	groups := map[string][]string{"A": {"B"}, "B": {"A"}}
	err := validateGroupDAG(groups)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected error to contain 'cycle', got: %v", err)
	}
}

func TestValidateGroupDAG_IndirectCycle(t *testing.T) {
	groups := map[string][]string{"A": {"B"}, "B": {"C"}, "C": {"A"}}
	if err := validateGroupDAG(groups); err == nil {
		t.Fatal("expected cycle error, got nil")
	}
}

func TestValidateGroupDAG_SelfCycle(t *testing.T) {
	groups := map[string][]string{"A": {"A"}}
	if err := validateGroupDAG(groups); err == nil {
		t.Fatal("expected self-cycle error, got nil")
	}
}

func TestValidateGroupDAG_Empty(t *testing.T) {
	if err := validateGroupDAG(map[string][]string{}); err != nil {
		t.Fatalf("expected no error for empty groups, got: %v", err)
	}
}
