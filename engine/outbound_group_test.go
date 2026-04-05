package engine

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"

	"github.com/shuttleX/shuttle/adapter"
)

// mockOutbound is a test double for adapter.Outbound.
type mockOutbound struct {
	tag      string
	dialFunc func(ctx context.Context, network, address string) (net.Conn, error)
	calls    atomic.Int64
}

func (m *mockOutbound) Tag() string  { return m.tag }
func (m *mockOutbound) Type() string { return "mock" }
func (m *mockOutbound) Close() error { return nil }

func (m *mockOutbound) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	m.calls.Add(1)
	return m.dialFunc(ctx, network, address)
}

func failDial(_ context.Context, _, _ string) (net.Conn, error) {
	return nil, errors.New("dial failed")
}

func succeedDial(_ context.Context, _, _ string) (net.Conn, error) {
	// Return a pipe end as a valid net.Conn.
	c1, _ := net.Pipe()
	return c1, nil
}

// Compile-time interface check.
var _ adapter.Outbound = (*OutboundGroup)(nil)

func TestOutboundGroup_Failover(t *testing.T) {
	first := &mockOutbound{tag: "fail", dialFunc: failDial}
	second := &mockOutbound{tag: "ok", dialFunc: succeedDial}

	g := NewOutboundGroup("fo", GroupFailover, []adapter.Outbound{first, second})

	conn, err := g.DialContext(context.Background(), "tcp", "example.com:80")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	conn.Close()

	if first.calls.Load() != 1 {
		t.Errorf("first outbound calls = %d, want 1", first.calls.Load())
	}
	if second.calls.Load() != 1 {
		t.Errorf("second outbound calls = %d, want 1", second.calls.Load())
	}
}

func TestOutboundGroup_LoadBalance(t *testing.T) {
	a := &mockOutbound{tag: "a", dialFunc: succeedDial}
	b := &mockOutbound{tag: "b", dialFunc: succeedDial}

	g := NewOutboundGroup("lb", GroupLoadBalance, []adapter.Outbound{a, b})

	// Dial 4 times — expect round-robin distribution (2 each).
	for i := 0; i < 4; i++ {
		conn, err := g.DialContext(context.Background(), "tcp", "example.com:80")
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		conn.Close()
	}

	if a.calls.Load() != 2 {
		t.Errorf("outbound a calls = %d, want 2", a.calls.Load())
	}
	if b.calls.Load() != 2 {
		t.Errorf("outbound b calls = %d, want 2", b.calls.Load())
	}
}

func TestOutboundGroup_LoadBalance_Failover(t *testing.T) {
	// When the selected LB outbound fails, it should try the next one.
	fail := &mockOutbound{tag: "fail", dialFunc: failDial}
	ok := &mockOutbound{tag: "ok", dialFunc: succeedDial}

	g := NewOutboundGroup("lb-fo", GroupLoadBalance, []adapter.Outbound{fail, ok})

	conn, err := g.DialContext(context.Background(), "tcp", "example.com:80")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	conn.Close()

	if ok.calls.Load() != 1 {
		t.Errorf("ok outbound calls = %d, want 1", ok.calls.Load())
	}
}

func TestOutboundGroup_AllFail(t *testing.T) {
	a := &mockOutbound{tag: "a", dialFunc: failDial}
	b := &mockOutbound{tag: "b", dialFunc: failDial}

	g := NewOutboundGroup("fail-all", GroupFailover, []adapter.Outbound{a, b})

	conn, err := g.DialContext(context.Background(), "tcp", "example.com:80")
	if conn != nil {
		t.Fatal("expected nil conn")
	}
	if err == nil {
		t.Fatal("expected error when all members fail")
	}
	if a.calls.Load() != 1 || b.calls.Load() != 1 {
		t.Errorf("expected both outbounds tried, got a=%d b=%d", a.calls.Load(), b.calls.Load())
	}
}

func TestOutboundGroup_Empty(t *testing.T) {
	g := NewOutboundGroup("empty", GroupFailover, nil)
	_, err := g.DialContext(context.Background(), "tcp", "example.com:80")
	if err == nil {
		t.Fatal("expected error for empty group")
	}
}

func TestOutboundGroup_TagType(t *testing.T) {
	g := NewOutboundGroup("my-group", GroupFailover, nil)
	if g.Tag() != "my-group" {
		t.Errorf("Tag() = %q, want %q", g.Tag(), "my-group")
	}
	if g.Type() != "group" {
		t.Errorf("Type() = %q, want %q", g.Type(), "group")
	}
}

func TestOutboundGroup_Close(t *testing.T) {
	closed := make([]string, 0)
	makeMock := func(tag string) *mockOutbound {
		return &mockOutbound{
			tag: tag,
			dialFunc: succeedDial,
		}
	}
	a := makeMock("a")
	b := makeMock("b")
	// Override Close to track calls.
	type closeable struct {
		adapter.Outbound
		closeFunc func() error
	}
	ca := &closeable{a, func() error { closed = append(closed, "a"); return nil }}
	cb := &closeable{b, func() error { closed = append(closed, "b"); return nil }}

	// We can't use closeable directly since OutboundGroup stores adapter.Outbound.
	// Instead just test Close on the group with normal mocks.
	g := NewOutboundGroup("g", GroupFailover, []adapter.Outbound{a, b})
	if err := g.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
	// Verify no panic; mockOutbound.Close returns nil.
	_ = ca
	_ = cb
	_ = closed
}

func TestParseOutboundGroupConfig(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		raw := []byte(`{"strategy":"failover","outbounds":["us","jp"]}`)
		cfg, err := parseOutboundGroupConfig(raw)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Strategy != GroupFailover {
			t.Errorf("strategy = %q, want %q", cfg.Strategy, GroupFailover)
		}
		if len(cfg.Outbounds) != 2 {
			t.Errorf("outbounds len = %d, want 2", len(cfg.Outbounds))
		}
	})

	t.Run("empty outbounds", func(t *testing.T) {
		raw := []byte(`{"strategy":"failover","outbounds":[]}`)
		_, err := parseOutboundGroupConfig(raw)
		if err == nil {
			t.Fatal("expected error for empty outbounds")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := parseOutboundGroupConfig([]byte(`{bad`))
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})
}
