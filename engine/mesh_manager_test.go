package engine

import (
	"context"
	"testing"

	"github.com/shuttleX/shuttle/config"
)

func TestMeshManager_New(t *testing.T) {
	mm := NewMeshManager(nil)
	if mm == nil {
		t.Fatal("NewMeshManager returned nil")
	}
	if mm.Client() != nil {
		t.Error("client should be nil before Start")
	}
}

func TestMeshManager_StartDisabled(t *testing.T) {
	mm := NewMeshManager(nil)
	cfg := config.DefaultClientConfig()
	cfg.Mesh.Enabled = false
	err := mm.Start(context.Background(), cfg, nil, nil)
	if err != nil {
		t.Errorf("expected nil error for disabled mesh, got: %v", err)
	}
}

func TestMeshManager_StartNoTUN(t *testing.T) {
	mm := NewMeshManager(nil)
	cfg := config.DefaultClientConfig()
	cfg.Mesh.Enabled = true
	err := mm.Start(context.Background(), cfg, nil, nil)
	if err != nil {
		t.Errorf("expected nil error (warning only) for no TUN, got: %v", err)
	}
}

func TestMeshManager_CloseIdempotent(t *testing.T) {
	mm := NewMeshManager(nil)
	// Close without Start should be safe.
	if err := mm.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := mm.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
