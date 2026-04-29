package healthcheck

import (
	"testing"
	"time"
)

func TestHeartbeat_IsAliveWhenFresh(t *testing.T) {
	h := NewHeartbeat()
	h.Tick()
	if !h.IsAlive(time.Second) {
		t.Fatal("freshly ticked heartbeat should be alive")
	}
}

func TestHeartbeat_IsAliveBecomesFalseWhenStale(t *testing.T) {
	h := NewHeartbeat()
	h.Tick()
	time.Sleep(20 * time.Millisecond)
	if h.IsAlive(10 * time.Millisecond) {
		t.Fatal("heartbeat should be stale after 20ms with 10ms threshold")
	}
}

func TestHeartbeat_ZeroValueIsNotAlive(t *testing.T) {
	h := NewHeartbeat()
	if h.IsAlive(time.Hour) {
		t.Fatal("never-ticked heartbeat should not be alive")
	}
}
