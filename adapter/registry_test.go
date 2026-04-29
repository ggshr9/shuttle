package adapter_test

import (
	"testing"

	"github.com/ggshr9/shuttle/adapter"
	"github.com/ggshr9/shuttle/config"
)

type mockFactory struct{ name string }

func (f *mockFactory) Type() string { return f.name }
func (f *mockFactory) NewClient(cfg *config.ClientConfig, opts adapter.FactoryOptions) (adapter.ClientTransport, error) {
	return nil, nil
}
func (f *mockFactory) NewServer(cfg *config.ServerConfig, opts adapter.FactoryOptions) (adapter.ServerTransport, error) {
	return nil, nil
}

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
