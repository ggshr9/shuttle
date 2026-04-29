package adapter_test

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"github.com/ggshr9/shuttle/adapter"
)

type mockOutbound struct {
	tag string
}

func (m *mockOutbound) Tag() string                                                              { return m.tag }
func (m *mockOutbound) Type() string                                                             { return "mock" }
func (m *mockOutbound) DialContext(_ context.Context, _, _ string) (net.Conn, error)              { return nil, nil }
func (m *mockOutbound) Close() error                                                             { return nil }

type mockOutboundFactory struct{ name string }

func (f *mockOutboundFactory) Type() string { return f.name }
func (f *mockOutboundFactory) Create(tag string, _ json.RawMessage, _ adapter.OutboundDeps) (adapter.Outbound, error) {
	return &mockOutbound{tag: tag}, nil
}

func TestOutboundRegistryRoundTrip(t *testing.T) {
	adapter.ResetOutboundRegistry()
	adapter.RegisterOutbound(&mockOutboundFactory{name: "test-outbound"})

	f := adapter.GetOutbound("test-outbound")
	if f == nil {
		t.Fatal("expected factory, got nil")
	}
	if f.Type() != "test-outbound" {
		t.Errorf("Type() = %q, want %q", f.Type(), "test-outbound")
	}
}

func TestOutboundRegistryAll(t *testing.T) {
	adapter.ResetOutboundRegistry()
	adapter.RegisterOutbound(&mockOutboundFactory{name: "alpha"})
	adapter.RegisterOutbound(&mockOutboundFactory{name: "beta"})

	all := adapter.AllOutbounds()
	if len(all) != 2 {
		t.Fatalf("AllOutbounds() returned %d factories, want 2", len(all))
	}
	if all["alpha"] == nil || all["beta"] == nil {
		t.Fatal("missing expected factory in AllOutbounds()")
	}
}

func TestOutboundRegistryGetMissing(t *testing.T) {
	adapter.ResetOutboundRegistry()
	if f := adapter.GetOutbound("nonexistent"); f != nil {
		t.Errorf("GetOutbound(nonexistent) = %v, want nil", f)
	}
}

func TestCreateOutbound(t *testing.T) {
	adapter.ResetOutboundRegistry()
	adapter.RegisterOutbound(&mockOutboundFactory{name: "test-outbound"})

	ob, err := adapter.CreateOutbound("test-outbound", "my-tag", nil, adapter.OutboundDeps{})
	if err != nil {
		t.Fatal(err)
	}
	if ob.Tag() != "my-tag" {
		t.Errorf("Tag() = %q, want %q", ob.Tag(), "my-tag")
	}
}

func TestCreateOutboundUnknownType(t *testing.T) {
	adapter.ResetOutboundRegistry()
	_, err := adapter.CreateOutbound("unknown", "tag", nil, adapter.OutboundDeps{})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}
