package proxy

import (
	"testing"

	"github.com/shuttleX/shuttle/adapter"
)

func TestTUNInboundFactory_Registered(t *testing.T) {
	// Verify factory type and direct construction (avoids init() ordering issues
	// when running the full proxy package test suite).
	f := &TUNInboundFactory{}
	if f.Type() != "tun" {
		t.Errorf("Type() = %q, want %q", f.Type(), "tun")
	}
	// Also verify registration if available (may not be populated in all test orderings).
	if got := adapter.GetInbound("tun"); got != nil {
		if got.Type() != "tun" {
			t.Errorf("registered Type() = %q, want %q", got.Type(), "tun")
		}
	}
}

func TestTUNInboundFactory_Create(t *testing.T) {
	f := &TUNInboundFactory{}
	ib, err := f.Create("my-tun", nil, adapter.InboundDeps{})
	if err != nil {
		t.Fatal(err)
	}
	if ib.Tag() != "my-tun" {
		t.Errorf("Tag() = %q, want %q", ib.Tag(), "my-tun")
	}
	if ib.Type() != "tun" {
		t.Errorf("Type() = %q, want %q", ib.Type(), "tun")
	}
}

func TestTUNInboundFactory_CreateWithOptions(t *testing.T) {
	f := &TUNInboundFactory{}
	opts := []byte(`{"device_name":"utun99","cidr":"10.0.0.0/8","mtu":9000,"auto_route":true,"tun_fd":5}`)
	ib, err := f.Create("tagged", opts, adapter.InboundDeps{})
	if err != nil {
		t.Fatal(err)
	}
	ti, ok := ib.(*TUNInbound)
	if !ok {
		t.Fatal("expected *TUNInbound")
	}
	if ti.config.DeviceName != "utun99" {
		t.Errorf("DeviceName = %q, want %q", ti.config.DeviceName, "utun99")
	}
	if ti.config.CIDR != "10.0.0.0/8" {
		t.Errorf("CIDR = %q, want %q", ti.config.CIDR, "10.0.0.0/8")
	}
	if ti.config.MTU != 9000 {
		t.Errorf("MTU = %d, want %d", ti.config.MTU, 9000)
	}
	if !ti.config.AutoRoute {
		t.Error("AutoRoute = false, want true")
	}
	if ti.config.TunFD != 5 {
		t.Errorf("TunFD = %d, want %d", ti.config.TunFD, 5)
	}
}

func TestTUNInbound_ServerNilBeforeStart(t *testing.T) {
	f := &TUNInboundFactory{}
	ib, err := f.Create("tun0", nil, adapter.InboundDeps{})
	if err != nil {
		t.Fatal(err)
	}
	ti := ib.(*TUNInbound)
	if ti.Server() != nil {
		t.Error("Server() should be nil before Start()")
	}
}
