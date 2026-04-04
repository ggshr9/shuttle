package adapter_test

import (
	"testing"

	"github.com/shuttleX/shuttle/adapter"
)

type mockFactory struct{ name string }

func (f *mockFactory) Type() string { return f.name }

func TestRegistryRoundTrip(t *testing.T) {
	adapter.ResetRegistry()
	adapter.Register(&mockFactory{name: "test-transport"})

	f := adapter.Get("test-transport")
	if f == nil {
		t.Fatal("expected factory, got nil")
	}
	if f.Type() != "test-transport" {
		t.Errorf("Type() = %q, want %q", f.Type(), "test-transport")
	}
}

func TestRegistryAll(t *testing.T) {
	adapter.ResetRegistry()
	adapter.Register(&mockFactory{name: "alpha"})
	adapter.Register(&mockFactory{name: "beta"})

	all := adapter.All()
	if len(all) != 2 {
		t.Fatalf("All() returned %d factories, want 2", len(all))
	}
	if all["alpha"] == nil || all["beta"] == nil {
		t.Fatal("missing expected factory in All()")
	}
}

func TestRegistryGetMissing(t *testing.T) {
	adapter.ResetRegistry()
	if f := adapter.Get("nonexistent"); f != nil {
		t.Errorf("Get(nonexistent) = %v, want nil", f)
	}
}
