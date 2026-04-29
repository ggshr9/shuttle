package engine

import (
	"context"
	"testing"

	"github.com/ggshr9/shuttle/adapter"
)

// Compile-time interface checks for all outbound types.
var _ adapter.Outbound = (*DirectOutbound)(nil)
var _ adapter.Outbound = (*RejectOutbound)(nil)
var _ adapter.Outbound = (*ProxyOutbound)(nil)
var _ adapter.Outbound = (*OutboundGroup)(nil)

func TestDirectOutboundTagType(t *testing.T) {
	d := &DirectOutbound{tag: "direct"}
	if d.Tag() != "direct" {
		t.Errorf("Tag() = %q, want %q", d.Tag(), "direct")
	}
	if d.Type() != "direct" {
		t.Errorf("Type() = %q, want %q", d.Type(), "direct")
	}
	if err := d.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

func TestRejectOutboundDial(t *testing.T) {
	r := &RejectOutbound{tag: "reject"}
	if r.Tag() != "reject" {
		t.Errorf("Tag() = %q, want %q", r.Tag(), "reject")
	}
	if r.Type() != "reject" {
		t.Errorf("Type() = %q, want %q", r.Type(), "reject")
	}
	conn, err := r.DialContext(context.Background(), "tcp", "example.com:80")
	if err == nil {
		t.Fatal("expected error from RejectOutbound.DialContext")
	}
	if conn != nil {
		t.Fatal("expected nil conn from RejectOutbound.DialContext")
	}
}
