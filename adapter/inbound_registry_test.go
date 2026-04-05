package adapter_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/shuttleX/shuttle/adapter"
)

type mockInbound struct {
	tag string
}

func (m *mockInbound) Tag() string                                                       { return m.tag }
func (m *mockInbound) Type() string                                                      { return "mock" }
func (m *mockInbound) Start(_ context.Context, _ adapter.InboundRouter) error            { return nil }
func (m *mockInbound) Close() error                                                      { return nil }

type mockInboundFactory struct{ name string }

func (f *mockInboundFactory) Type() string { return f.name }
func (f *mockInboundFactory) Create(tag string, _ json.RawMessage, _ adapter.InboundDeps) (adapter.Inbound, error) {
	return &mockInbound{tag: tag}, nil
}

func TestInboundRegistryRoundTrip(t *testing.T) {
	adapter.ResetInboundRegistry()
	adapter.RegisterInbound(&mockInboundFactory{name: "test-inbound"})

	f := adapter.GetInbound("test-inbound")
	if f == nil {
		t.Fatal("expected factory, got nil")
	}
	if f.Type() != "test-inbound" {
		t.Errorf("Type() = %q, want %q", f.Type(), "test-inbound")
	}
}

func TestInboundRegistryAll(t *testing.T) {
	adapter.ResetInboundRegistry()
	adapter.RegisterInbound(&mockInboundFactory{name: "alpha"})
	adapter.RegisterInbound(&mockInboundFactory{name: "beta"})

	all := adapter.AllInbounds()
	if len(all) != 2 {
		t.Fatalf("AllInbounds() returned %d factories, want 2", len(all))
	}
	if all["alpha"] == nil || all["beta"] == nil {
		t.Fatal("missing expected factory in AllInbounds()")
	}
}

func TestInboundRegistryGetMissing(t *testing.T) {
	adapter.ResetInboundRegistry()
	if f := adapter.GetInbound("nonexistent"); f != nil {
		t.Errorf("GetInbound(nonexistent) = %v, want nil", f)
	}
}

func TestCreateInbound(t *testing.T) {
	adapter.ResetInboundRegistry()
	adapter.RegisterInbound(&mockInboundFactory{name: "test-inbound"})

	ib, err := adapter.CreateInbound("test-inbound", "my-tag", nil, adapter.InboundDeps{})
	if err != nil {
		t.Fatal(err)
	}
	if ib.Tag() != "my-tag" {
		t.Errorf("Tag() = %q, want %q", ib.Tag(), "my-tag")
	}
}

func TestCreateInboundUnknownType(t *testing.T) {
	adapter.ResetInboundRegistry()
	_, err := adapter.CreateInbound("unknown", "tag", nil, adapter.InboundDeps{})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}
