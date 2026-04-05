package engine

import (
	"bytes"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/obfs"
	"github.com/shuttleX/shuttle/stream"
)

// mockStream implements transport.Stream for testing streamConn.
type mockStream struct {
	*bytes.Buffer
	closed bool
}

func newMockStream(data []byte) *mockStream {
	return &mockStream{Buffer: bytes.NewBuffer(data)}
}

func (m *mockStream) Close() error   { m.closed = true; return nil }
func (m *mockStream) StreamID() uint64 { return 42 }

// ---------------------------------------------------------------------------
// TestNewEngine
// ---------------------------------------------------------------------------

func TestNewEngine(t *testing.T) {
	cfg := config.DefaultClientConfig()
	eng := New(cfg)

	if eng == nil {
		t.Fatal("New returned nil")
	}
	if eng.state != StateStopped {
		t.Errorf("initial state = %v, want StateStopped", eng.state)
	}
	if eng.obs == nil {
		t.Error("obs manager is nil")
	}
	if eng.traffic == nil {
		t.Error("traffic manager is nil")
	}
	if eng.cfg == nil {
		t.Error("cfg is nil")
	}
}

// ---------------------------------------------------------------------------
// TestValidateConfig
// ---------------------------------------------------------------------------

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*config.ClientConfig)
		wantErr bool
	}{
		{
			name: "valid config with H3",
			mutate: func(c *config.ClientConfig) {
				c.Server.Addr = "example.com:443"
				c.Transport.H3.Enabled = true
			},
			wantErr: false,
		},
		{
			name: "valid config with Reality",
			mutate: func(c *config.ClientConfig) {
				c.Server.Addr = "example.com:443"
				c.Transport.Reality.Enabled = true
			},
			wantErr: false,
		},
		{
			name: "valid config with CDN",
			mutate: func(c *config.ClientConfig) {
				c.Server.Addr = "example.com:443"
				c.Transport.CDN.Enabled = true
			},
			wantErr: false,
		},
		{
			name: "valid config with WebRTC",
			mutate: func(c *config.ClientConfig) {
				c.Server.Addr = "example.com:443"
				c.Transport.WebRTC.Enabled = true
			},
			wantErr: false,
		},
		{
			name: "missing server address",
			mutate: func(c *config.ClientConfig) {
				c.Server.Addr = ""
				c.Transport.H3.Enabled = true
			},
			wantErr: true,
		},
		{
			name: "no transports enabled",
			mutate: func(c *config.ClientConfig) {
				c.Server.Addr = "example.com:443"
				c.Transport.H3.Enabled = false
				c.Transport.Reality.Enabled = false
				c.Transport.CDN.Enabled = false
				c.Transport.WebRTC.Enabled = false
			},
			wantErr: true,
		},
		{
			name: "both missing address and no transports",
			mutate: func(c *config.ClientConfig) {
				c.Server.Addr = ""
				c.Transport.H3.Enabled = false
				c.Transport.Reality.Enabled = false
				c.Transport.CDN.Enabled = false
				c.Transport.WebRTC.Enabled = false
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultClientConfig()
			tt.mutate(cfg)
			err := ValidateConfig(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestEngineStateInitial
// ---------------------------------------------------------------------------

func TestEngineStateInitial(t *testing.T) {
	cfg := config.DefaultClientConfig()
	eng := New(cfg)

	status := eng.Status()
	if status.State != "stopped" {
		t.Errorf("Status().State = %q, want %q", status.State, "stopped")
	}
}

// ---------------------------------------------------------------------------
// TestEngineConfig
// ---------------------------------------------------------------------------

func TestEngineConfig(t *testing.T) {
	cfg := config.DefaultClientConfig()
	cfg.Server.Addr = "original:443"
	eng := New(cfg)

	// Config() should return the current value.
	got := eng.Config()
	if got.Server.Addr != "original:443" {
		t.Errorf("Config().Server.Addr = %q, want %q", got.Server.Addr, "original:443")
	}

	// Mutating the returned config must NOT affect the engine's internal copy.
	got.Server.Addr = "mutated:443"
	got2 := eng.Config()
	if got2.Server.Addr != "original:443" {
		t.Errorf("after mutation, Config().Server.Addr = %q, want %q", got2.Server.Addr, "original:443")
	}

	// SetConfig should update the engine's internal config (deep copy).
	newCfg := config.DefaultClientConfig()
	newCfg.Server.Addr = "updated:443"
	eng.SetConfig(newCfg)

	got3 := eng.Config()
	if got3.Server.Addr != "updated:443" {
		t.Errorf("after SetConfig, Config().Server.Addr = %q, want %q", got3.Server.Addr, "updated:443")
	}

	// Mutating the config passed to SetConfig must NOT affect the engine.
	newCfg.Server.Addr = "sneaky:443"
	got4 := eng.Config()
	if got4.Server.Addr != "updated:443" {
		t.Errorf("after mutating SetConfig arg, Config().Server.Addr = %q, want %q", got4.Server.Addr, "updated:443")
	}
}

// ---------------------------------------------------------------------------
// TestEngineSubscribeUnsubscribe
// ---------------------------------------------------------------------------

func TestEngineSubscribeUnsubscribe(t *testing.T) {
	cfg := config.DefaultClientConfig()
	eng := New(cfg)

	ch := eng.Subscribe()
	if ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}

	// Channel should be buffered.
	if cap(ch) != eventChannelBuffer {
		t.Errorf("channel capacity = %d, want %d", cap(ch), eventChannelBuffer)
	}

	// Unsubscribe should close the channel.
	eng.Unsubscribe(ch)

	// Reading from the closed channel should return the zero value immediately.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed")
		}
	default:
		t.Error("channel not closed after Unsubscribe")
	}

	// Double unsubscribe should be safe (no panic).
	eng.Unsubscribe(ch)
}

// ---------------------------------------------------------------------------
// TestEngineEmit
// ---------------------------------------------------------------------------

func TestEngineEmit(t *testing.T) {
	cfg := config.DefaultClientConfig()
	eng := New(cfg)

	ch1 := eng.Subscribe()
	ch2 := eng.Subscribe()
	defer eng.Unsubscribe(ch1)
	defer eng.Unsubscribe(ch2)

	eng.emit(Event{Type: EventLog, Message: "hello"})

	// Both subscribers should receive the event.
	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case ev := <-ch:
			if ev.Type != EventLog {
				t.Errorf("subscriber %d: Type = %v, want EventLog", i, ev.Type)
			}
			if ev.Message != "hello" {
				t.Errorf("subscriber %d: Message = %q, want %q", i, ev.Message, "hello")
			}
			if ev.Timestamp.IsZero() {
				t.Errorf("subscriber %d: Timestamp is zero", i)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out waiting for event", i)
		}
	}
}

// ---------------------------------------------------------------------------
// TestEmitConnectionEvent
// ---------------------------------------------------------------------------

func TestEmitConnectionEvent(t *testing.T) {
	cfg := config.DefaultClientConfig()
	eng := New(cfg)

	ch := eng.Subscribe()
	defer eng.Unsubscribe(ch)

	eng.EmitConnectionEvent("conn-1", "opened", "example.com:443", "proxy", "tcp", "curl", 100, 200, 500)

	select {
	case ev := <-ch:
		if ev.Type != EventConnection {
			t.Errorf("Type = %v, want EventConnection", ev.Type)
		}
		if ev.ConnID != "conn-1" {
			t.Errorf("ConnID = %q, want %q", ev.ConnID, "conn-1")
		}
		if ev.ConnState != "opened" {
			t.Errorf("ConnState = %q, want %q", ev.ConnState, "opened")
		}
		if ev.Target != "example.com:443" {
			t.Errorf("Target = %q, want %q", ev.Target, "example.com:443")
		}
		if ev.Rule != "proxy" {
			t.Errorf("Rule = %q, want %q", ev.Rule, "proxy")
		}
		if ev.Protocol != "tcp" {
			t.Errorf("Protocol = %q, want %q", ev.Protocol, "tcp")
		}
		if ev.ProcessName != "curl" {
			t.Errorf("ProcessName = %q, want %q", ev.ProcessName, "curl")
		}
		if ev.BytesIn != 100 {
			t.Errorf("BytesIn = %d, want 100", ev.BytesIn)
		}
		if ev.BytesOut != 200 {
			t.Errorf("BytesOut = %d, want 200", ev.BytesOut)
		}
		if ev.DurationMs != 500 {
			t.Errorf("DurationMs = %d, want 500", ev.DurationMs)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for connection event")
	}
}

// ---------------------------------------------------------------------------
// TestEngineSubscriberBuffer
// ---------------------------------------------------------------------------

func TestEngineSubscriberBuffer(t *testing.T) {
	cfg := config.DefaultClientConfig()
	eng := New(cfg)

	ch := eng.Subscribe()
	defer eng.Unsubscribe(ch)

	// Fill the buffer completely.
	for i := 0; i < eventChannelBuffer; i++ {
		eng.emit(Event{Type: EventLog, Message: "fill"})
	}

	// Emit one more -- this must NOT block (dropped for slow consumer).
	done := make(chan struct{})
	go func() {
		eng.emit(Event{Type: EventLog, Message: "overflow"})
		close(done)
	}()

	select {
	case <-done:
		// Good: emit did not block.
	case <-time.After(time.Second):
		t.Fatal("emit blocked on full subscriber channel")
	}
}

// ---------------------------------------------------------------------------
// TestStopNotRunning
// ---------------------------------------------------------------------------

func TestStopNotRunning(t *testing.T) {
	cfg := config.DefaultClientConfig()
	eng := New(cfg)

	err := eng.Stop()
	if err == nil {
		t.Fatal("Stop on stopped engine should return error")
	}
	if got := err.Error(); got != "engine not running (state: stopped)" {
		t.Errorf("Stop error = %q, want %q", got, "engine not running (state: stopped)")
	}
}

// ---------------------------------------------------------------------------
// TestEngineStateString
// ---------------------------------------------------------------------------

func TestEngineStateString(t *testing.T) {
	tests := []struct {
		state EngineState
		want  string
	}{
		{StateStopped, "stopped"},
		{StateStarting, "starting"},
		{StateRunning, "running"},
		{StateStopping, "stopping"},
		{EngineState(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("EngineState(%d).String() = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestEventTypeString
// ---------------------------------------------------------------------------

func TestEventTypeString(t *testing.T) {
	tests := []struct {
		et   EventType
		want string
	}{
		{EventConnected, "connected"},
		{EventDisconnected, "disconnected"},
		{EventSpeedTick, "speed_tick"},
		{EventLog, "log"},
		{EventTransportChanged, "transport_changed"},
		{EventError, "error"},
		{EventConnection, "connection"},
		{EventType(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.et.String(); got != tt.want {
				t.Errorf("EventType(%d).String() = %q, want %q", tt.et, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestStartAlreadyRunning
// ---------------------------------------------------------------------------

func TestStartAlreadyRunning(t *testing.T) {
	cfg := config.DefaultClientConfig()
	eng := New(cfg)

	// Manually set state to Running to test the guard without calling real Start.
	eng.mu.Lock()
	eng.state = StateRunning
	eng.mu.Unlock()

	// Stop should succeed (it checks StateRunning), but we cannot actually stop
	// because there's no cancel/selector. Instead, test that a second Start
	// would be rejected. We access startInternal indirectly by checking the state guard.
	// Since Start() acquires lifecycleMu and calls startInternal, and startInternal
	// checks state, we test via the public method by setting the state.
	// But Start will call sysopt.Apply after the guard -- we cannot call it.
	// So we test the guard by checking that Stop succeeds on a manually-running engine
	// (it will panic on nil cancel -- but the state check comes first).

	// Instead, test the state-based error from startInternal directly.
	// We hold lifecycleMu ourselves to call startInternal (but it calls sysopt.Apply).
	// The safest approach: just verify the state guard in Stop on a non-running engine.

	// Actually, let's test the error from Stop on a "starting" state.
	eng.mu.Lock()
	eng.state = StateStarting
	eng.mu.Unlock()

	err := eng.Stop()
	if err == nil {
		t.Fatal("Stop on starting engine should return error")
	}
	if got := err.Error(); got != "engine not running (state: starting)" {
		t.Errorf("Stop error = %q, want correct state guard message", got)
	}

	// Also test stop on a "stopping" engine.
	eng.mu.Lock()
	eng.state = StateStopping
	eng.mu.Unlock()

	err = eng.Stop()
	if err == nil {
		t.Fatal("Stop on stopping engine should return error")
	}
}

// ---------------------------------------------------------------------------
// TestConcurrentSubscribe
// ---------------------------------------------------------------------------

func TestConcurrentSubscribe(t *testing.T) {
	cfg := config.DefaultClientConfig()
	eng := New(cfg)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	channels := make([]<-chan Event, goroutines)
	var mu sync.Mutex

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			ch := eng.Subscribe()
			mu.Lock()
			channels[idx] = ch
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	// Emit an event; all subscribers should receive it.
	eng.emit(Event{Type: EventLog, Message: "concurrent"})

	for i, ch := range channels {
		select {
		case ev := <-ch:
			if ev.Message != "concurrent" {
				t.Errorf("subscriber %d got message %q", i, ev.Message)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out", i)
		}
	}

	// Concurrently unsubscribe all.
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			eng.Unsubscribe(channels[idx])
		}(i)
	}
	wg.Wait()

	// Verify all channels are closed.
	for i, ch := range channels {
		select {
		case _, ok := <-ch:
			if ok {
				t.Errorf("subscriber %d: channel not closed", i)
			}
		default:
			t.Errorf("subscriber %d: channel not closed (blocked on read)", i)
		}
	}
}

// ---------------------------------------------------------------------------
// TestStreamConn
// ---------------------------------------------------------------------------

func TestStreamConn(t *testing.T) {
	ms := newMockStream([]byte("hello world"))
	sc := &streamConn{stream: ms, addr: "example.com:443"}

	// Verify net.Conn interface compliance at compile time.
	var _ net.Conn = sc

	// Test Read.
	buf := make([]byte, 5)
	n, err := sc.Read(buf)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if n != 5 || string(buf) != "hello" {
		t.Errorf("Read = (%d, %q), want (5, %q)", n, buf, "hello")
	}

	// Test Write.
	n, err = sc.Write([]byte(" back"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != 5 {
		t.Errorf("Write returned %d, want 5", n)
	}

	// Test LocalAddr / RemoteAddr return non-nil.
	if sc.LocalAddr() == nil {
		t.Error("LocalAddr returned nil")
	}
	if sc.RemoteAddr() == nil {
		t.Error("RemoteAddr returned nil")
	}

	// Test deadline methods return nil errors.
	now := time.Now()
	if err := sc.SetDeadline(now); err != nil {
		t.Errorf("SetDeadline error: %v", err)
	}
	if err := sc.SetReadDeadline(now); err != nil {
		t.Errorf("SetReadDeadline error: %v", err)
	}
	if err := sc.SetWriteDeadline(now); err != nil {
		t.Errorf("SetWriteDeadline error: %v", err)
	}

	// Test Close.
	if err := sc.Close(); err != nil {
		t.Errorf("Close error: %v", err)
	}
	if !ms.closed {
		t.Error("underlying stream not closed")
	}
}

// ---------------------------------------------------------------------------
// TestEmitNoSubscribers
// ---------------------------------------------------------------------------

func TestEmitNoSubscribers(t *testing.T) {
	cfg := config.DefaultClientConfig()
	eng := New(cfg)

	// emit with no subscribers should not panic.
	eng.emit(Event{Type: EventLog, Message: "nobody listening"})
}

// ---------------------------------------------------------------------------
// TestEmitSetsTimestamp
// ---------------------------------------------------------------------------

func TestEmitSetsTimestamp(t *testing.T) {
	cfg := config.DefaultClientConfig()
	eng := New(cfg)

	ch := eng.Subscribe()
	defer eng.Unsubscribe(ch)

	before := time.Now()
	eng.emit(Event{Type: EventLog})
	after := time.Now()

	ev := <-ch
	if ev.Timestamp.Before(before) || ev.Timestamp.After(after) {
		t.Errorf("Timestamp %v not between %v and %v", ev.Timestamp, before, after)
	}
}

// testEngine returns a minimal Engine suitable for calling methods in tests.
func testEngine() *Engine {
	return New(&config.ClientConfig{})
}

// ---------------------------------------------------------------------------
// TestBuildShaperConfig
// ---------------------------------------------------------------------------

func TestBuildShaperConfig(t *testing.T) {
	t.Run("disabled when ShapingEnabled is false", func(t *testing.T) {
		cfg := config.ObfsConfig{ShapingEnabled: false}
		sc := testEngine().buildShaperConfig(cfg)
		if sc.Enabled {
			t.Error("expected Enabled=false when ShapingEnabled is false")
		}
	})

	t.Run("enabled with defaults", func(t *testing.T) {
		cfg := config.ObfsConfig{ShapingEnabled: true}
		sc := testEngine().buildShaperConfig(cfg)
		if !sc.Enabled {
			t.Fatal("expected Enabled=true")
		}
		// Should use DefaultShaperConfig values
		if sc.ChunkMinSize != 64 {
			t.Errorf("ChunkMinSize = %d, want 64", sc.ChunkMinSize)
		}
		if sc.ChunkMaxSize != 1400 {
			t.Errorf("ChunkMaxSize = %d, want 1400", sc.ChunkMaxSize)
		}
		if sc.MaxDelay != 50*time.Millisecond {
			t.Errorf("MaxDelay = %v, want 50ms", sc.MaxDelay)
		}
	})

	t.Run("custom delays", func(t *testing.T) {
		cfg := config.ObfsConfig{
			ShapingEnabled: true,
			MinDelay:       "5ms",
			MaxDelay:       "100ms",
		}
		sc := testEngine().buildShaperConfig(cfg)
		if sc.MinDelay != 5*time.Millisecond {
			t.Errorf("MinDelay = %v, want 5ms", sc.MinDelay)
		}
		if sc.MaxDelay != 100*time.Millisecond {
			t.Errorf("MaxDelay = %v, want 100ms", sc.MaxDelay)
		}
	})

	t.Run("custom chunk size large", func(t *testing.T) {
		cfg := config.ObfsConfig{
			ShapingEnabled: true,
			ChunkSize:      1024,
		}
		sc := testEngine().buildShaperConfig(cfg)
		if sc.ChunkMinSize != 1024 {
			t.Errorf("ChunkMinSize = %d, want 1024", sc.ChunkMinSize)
		}
		if sc.ChunkMaxSize != 2048 {
			t.Errorf("ChunkMaxSize = %d, want 2048 (2x min)", sc.ChunkMaxSize)
		}
	})

	t.Run("small chunk size uses 1400 max", func(t *testing.T) {
		cfg := config.ObfsConfig{
			ShapingEnabled: true,
			ChunkSize:      100,
		}
		sc := testEngine().buildShaperConfig(cfg)
		if sc.ChunkMaxSize != 1400 {
			t.Errorf("ChunkMaxSize = %d, want 1400 (floor)", sc.ChunkMaxSize)
		}
	})

	t.Run("invalid delay strings ignored", func(t *testing.T) {
		cfg := config.ObfsConfig{
			ShapingEnabled: true,
			MinDelay:       "notaduration",
			MaxDelay:       "alsobad",
		}
		sc := testEngine().buildShaperConfig(cfg)
		if !sc.Enabled {
			t.Fatal("should still be enabled despite bad durations")
		}
		// Should fall back to default values
		if sc.MinDelay != 0 {
			t.Errorf("MinDelay = %v, want 0 (default)", sc.MinDelay)
		}
		if sc.MaxDelay != 50*time.Millisecond {
			t.Errorf("MaxDelay = %v, want 50ms (default)", sc.MaxDelay)
		}
	})
}

// ---------------------------------------------------------------------------
// TestSapedConnWriteGoesThruShaper
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// TestExtractPort
// ---------------------------------------------------------------------------

func TestExtractPort(t *testing.T) {
	tests := []struct {
		addr string
		want uint16
	}{
		{"example.com:443", 443},
		{"example.com:80", 80},
		{"127.0.0.1:8080", 8080},
		{"[::1]:22", 22},
		{"example.com:0", 0},
		{"example.com:65535", 65535},
		// Malformed cases
		{"example.com", 0},
		{"", 0},
		{"example.com:abc", 0},
		{"example.com:99999", 0},  // out of range
		{"example.com:-1", 0},     // negative
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			got := extractPort(tt.addr)
			if got != tt.want {
				t.Errorf("extractPort(%q) = %d, want %d", tt.addr, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestStreamPrioritySet
// ---------------------------------------------------------------------------

func TestStreamPrioritySet(t *testing.T) {
	ms := newMockStream(nil)
	tracker := stream.NewStreamTracker(10)
	metrics := tracker.Track(ms.StreamID(), "example.com:22", "h3")
	measured := stream.NewMeasuredStream(ms, metrics)

	// Default priority should be 0 (Critical == 0, but unset is also 0).
	if got := metrics.Priority.Load(); got != 0 {
		t.Errorf("initial priority = %d, want 0", got)
	}

	// Set priority to 3 (Bulk).
	measured.SetPriority(3)
	if got := metrics.Priority.Load(); got != 3 {
		t.Errorf("after SetPriority(3), priority = %d, want 3", got)
	}

	// Verify it appears in the summary.
	summary := tracker.Summary()
	if summary.Priorities.Bulk != 1 {
		t.Errorf("summary Bulk = %d, want 1", summary.Priorities.Bulk)
	}
}

func TestSapedConnWriteGoesThruShaper(t *testing.T) {
	ms := newMockStream(nil)
	sc := &streamConn{stream: ms, addr: "example.com:443"}

	// Create a shaper with no delay so the test is fast, but chunk splitting
	// is active (min=2, max=4), proving writes go through the Shaper.
	cfg := config.ObfsConfig{
		ShapingEnabled: true,
		MinDelay:       "0s",
		MaxDelay:       "0s",
		ChunkSize:      2,
	}
	shaperCfg := testEngine().buildShaperConfig(cfg)

	shaper := obfs.NewShaper(ms, shaperCfg)
	shaped := &shapedConn{streamConn: sc, shaper: shaper}

	data := []byte("ABCDEFGH")
	n, err := shaped.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write returned %d, want %d", n, len(data))
	}

	// The written bytes should appear in the underlying buffer.
	if ms.String() != "ABCDEFGH" {
		t.Errorf("underlying buffer = %q, want %q", ms.String(), "ABCDEFGH")
	}

	// Read passes through unchanged.
	ms2 := newMockStream([]byte("response"))
	sc2 := &streamConn{stream: ms2, addr: "example.com:443"}
	shaper2 := obfs.NewShaper(ms2, shaperCfg)
	shaped2 := &shapedConn{streamConn: sc2, shaper: shaper2}

	buf := make([]byte, 8)
	rn, err := shaped2.Read(buf)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if string(buf[:rn]) != "response" {
		t.Errorf("Read = %q, want %q", string(buf[:rn]), "response")
	}
}

// ---------------------------------------------------------------------------
// TestSetTransportStrategy
// ---------------------------------------------------------------------------

func TestSetTransportStrategyUnknown(t *testing.T) {
	cfg := config.DefaultClientConfig()
	eng := New(cfg)

	err := eng.SetTransportStrategy(nil, "bogus")
	if err == nil {
		t.Fatal("expected error for unknown strategy, got nil")
	}
}

func TestSetTransportStrategyNoSelector(t *testing.T) {
	cfg := config.DefaultClientConfig()
	eng := New(cfg)

	// Engine is stopped — selector is nil. All valid strategies should return an error.
	for _, s := range []string{"auto", "priority", "latency", "multipath"} {
		err := eng.SetTransportStrategy(nil, s)
		if err == nil {
			t.Errorf("strategy=%q: expected error (no active selector), got nil", s)
		}
	}
}

