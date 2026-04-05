package engine

import (
	"context"
	"testing"

	"github.com/shuttleX/shuttle/adapter"
)

// Compile-time interface check.
var _ adapter.Outbound = (*MeshOutbound)(nil)

func TestMeshOutbound_TagType(t *testing.T) {
	mm := NewMeshManager(nil)
	ob := NewMeshOutbound("mesh", mm)

	if ob.Tag() != "mesh" {
		t.Errorf("Tag() = %q, want %q", ob.Tag(), "mesh")
	}
	if ob.Type() != "mesh" {
		t.Errorf("Type() = %q, want %q", ob.Type(), "mesh")
	}
	if err := ob.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

func TestMeshOutbound_DialNotConnected(t *testing.T) {
	// MeshManager with no client (not started) — dial must fail.
	mm := NewMeshManager(nil)
	ob := NewMeshOutbound("mesh", mm)

	conn, err := ob.DialContext(context.Background(), "tcp", "10.0.0.1:80")
	if err == nil {
		conn.Close()
		t.Fatal("expected error when mesh is not connected")
	}
	if conn != nil {
		t.Error("expected nil conn when mesh is not connected")
	}
}

func TestMeshOutbound_DialNilManager(t *testing.T) {
	// nil meshManager — dial must fail gracefully.
	ob := NewMeshOutbound("mesh", nil)

	conn, err := ob.DialContext(context.Background(), "tcp", "10.0.0.1:80")
	if err == nil {
		conn.Close()
		t.Fatal("expected error when meshManager is nil")
	}
	if conn != nil {
		t.Error("expected nil conn when meshManager is nil")
	}
}
