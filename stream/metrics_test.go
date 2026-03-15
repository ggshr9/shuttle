package stream

import (
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// fakeStream implements transport.Stream using a net.Conn from net.Pipe.
type fakeStream struct {
	conn net.Conn
	id   uint64
}

func (f *fakeStream) Read(p []byte) (int, error)  { return f.conn.Read(p) }
func (f *fakeStream) Write(p []byte) (int, error) { return f.conn.Write(p) }
func (f *fakeStream) Close() error                { return f.conn.Close() }
func (f *fakeStream) StreamID() uint64             { return f.id }

// --- StreamMetrics tests ---

func TestStreamMetrics_Atomics(t *testing.T) {
	m := &StreamMetrics{StreamID: 1, StartTime: time.Now()}
	m.BytesSent.Add(100)
	m.BytesSent.Add(200)
	if got := m.BytesSent.Load(); got != 300 {
		t.Fatalf("BytesSent = %d, want 300", got)
	}
	m.BytesReceived.Add(50)
	if got := m.BytesReceived.Load(); got != 50 {
		t.Fatalf("BytesReceived = %d, want 50", got)
	}
	m.Errors.Add(1)
	if got := m.Errors.Load(); got != 1 {
		t.Fatalf("Errors = %d, want 1", got)
	}
}

func TestStreamMetrics_FirstByte(t *testing.T) {
	m := &StreamMetrics{}
	if fb := m.GetFirstByteTime(); !fb.IsZero() {
		t.Fatal("expected zero first-byte time before set")
	}
	now := time.Now()
	m.SetFirstByte(now)
	if fb := m.GetFirstByteTime(); !fb.Equal(now) {
		t.Fatalf("first byte time mismatch: got %v, want %v", fb, now)
	}
	// Second call should be ignored (CAS).
	later := now.Add(time.Second)
	m.SetFirstByte(later)
	if fb := m.GetFirstByteTime(); !fb.Equal(now) {
		t.Fatal("SetFirstByte should be idempotent")
	}
}

// --- StreamTracker tests ---

func TestStreamTracker_TrackAndGet(t *testing.T) {
	tr := NewStreamTracker(10)
	m := tr.Track(42, "example.com:443", "h3")
	if m.StreamID != 42 {
		t.Fatalf("StreamID = %d, want 42", m.StreamID)
	}
	got := tr.Get(42)
	if got != m {
		t.Fatal("Get returned different pointer")
	}
	if tr.Get(999) != nil {
		t.Fatal("Get should return nil for unknown ID")
	}
}

func TestStreamTracker_Active(t *testing.T) {
	tr := NewStreamTracker(10)
	m1 := tr.Track(1, "a:80", "h3")
	tr.Track(2, "b:80", "reality")
	m1.Closed.Store(true)

	active := tr.Active()
	if len(active) != 1 {
		t.Fatalf("Active() len = %d, want 1", len(active))
	}
	if active[0].StreamID != 2 {
		t.Fatalf("Active stream ID = %d, want 2", active[0].StreamID)
	}
}

func TestStreamTracker_Recent(t *testing.T) {
	tr := NewStreamTracker(5)
	for i := uint64(1); i <= 7; i++ {
		tr.Track(i, "t", "h3")
	}
	// Ring has capacity 5, so streams 1 and 2 are evicted.
	recent := tr.Recent(3)
	if len(recent) != 3 {
		t.Fatalf("Recent(3) len = %d, want 3", len(recent))
	}
	// Should be newest first: 7, 6, 5
	if recent[0].StreamID != 7 || recent[1].StreamID != 6 || recent[2].StreamID != 5 {
		t.Fatalf("Recent order: got %d,%d,%d want 7,6,5",
			recent[0].StreamID, recent[1].StreamID, recent[2].StreamID)
	}
}

func TestStreamTracker_RingEviction(t *testing.T) {
	tr := NewStreamTracker(3)
	tr.Track(1, "a", "h3")
	tr.Track(2, "b", "h3")
	tr.Track(3, "c", "h3")
	// Stream 1 should still be in index.
	if tr.Get(1) == nil {
		t.Fatal("stream 1 should exist before eviction")
	}
	// Adding a 4th should evict stream 1.
	tr.Track(4, "d", "h3")
	if tr.Get(1) != nil {
		t.Fatal("stream 1 should be evicted after ring wrap")
	}
	if tr.Get(4) == nil {
		t.Fatal("stream 4 should exist")
	}
}

func TestStreamTracker_Summary(t *testing.T) {
	tr := NewStreamTracker(10)
	m1 := tr.Track(1, "a", "h3")
	m1.BytesSent.Store(100)
	m1.BytesReceived.Store(200)
	m1.Duration.Store(int64(time.Second))
	m1.Closed.Store(true)

	m2 := tr.Track(2, "b", "reality")
	m2.BytesSent.Store(50)
	m2.BytesReceived.Store(75)

	s := tr.Summary()
	if s.TotalStreams != 2 {
		t.Fatalf("TotalStreams = %d, want 2", s.TotalStreams)
	}
	if s.ActiveStreams != 1 {
		t.Fatalf("ActiveStreams = %d, want 1", s.ActiveStreams)
	}
	if s.TotalBytesSent != 150 {
		t.Fatalf("TotalBytesSent = %d, want 150", s.TotalBytesSent)
	}
	if s.TotalBytesRecv != 275 {
		t.Fatalf("TotalBytesRecv = %d, want 275", s.TotalBytesRecv)
	}
	if s.AvgDuration != time.Second {
		t.Fatalf("AvgDuration = %v, want 1s", s.AvgDuration)
	}
}

func TestStreamTracker_DefaultSize(t *testing.T) {
	tr := NewStreamTracker(0)
	if tr.size != defaultRingSize {
		t.Fatalf("default size = %d, want %d", tr.size, defaultRingSize)
	}
}

func TestStreamTracker_ConcurrentAccess(t *testing.T) {
	tr := NewStreamTracker(100)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id uint64) {
			defer wg.Done()
			m := tr.Track(id, "target", "h3")
			m.BytesSent.Add(10)
			m.BytesReceived.Add(20)
			_ = tr.Get(id)
			_ = tr.Active()
			_ = tr.Recent(5)
			_ = tr.Summary()
		}(uint64(i))
	}
	wg.Wait()
}

// --- MeasuredStream tests ---

func TestMeasuredStream_ReadWrite(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	m := &StreamMetrics{StreamID: 1, StartTime: time.Now()}
	ms := NewMeasuredStream(&fakeStream{conn: client, id: 1}, m)

	payload := []byte("hello world")

	// Write through measured stream (net.Pipe is synchronous, so we need
	// a goroutine on the reader side).
	errCh := make(chan error, 1)
	buf := make([]byte, 64)
	go func() {
		n, err := server.Read(buf)
		if err != nil {
			errCh <- err
			return
		}
		if string(buf[:n]) != "hello world" {
			errCh <- fmt.Errorf("got %q, want %q", string(buf[:n]), "hello world")
			return
		}
		errCh <- nil
	}()

	n, err := ms.Write(payload)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("wrote %d bytes, want %d", n, len(payload))
	}
	if readErr := <-errCh; readErr != nil {
		t.Fatalf("server read: %v", readErr)
	}
	if got := m.BytesSent.Load(); got != int64(len(payload)) {
		t.Fatalf("BytesSent = %d, want %d", got, len(payload))
	}

	// Read through measured stream.
	response := []byte("hi back")
	go func() {
		server.Write(response)
	}()

	n, err = ms.Read(buf)
	if err != nil {
		t.Fatalf("measured read: %v", err)
	}
	if string(buf[:n]) != "hi back" {
		t.Fatalf("got %q, want %q", string(buf[:n]), "hi back")
	}
	if got := m.BytesReceived.Load(); got != int64(len(response)) {
		t.Fatalf("BytesReceived = %d, want %d", got, len(response))
	}
	// FirstByteTime should be set after first read.
	if fb := m.GetFirstByteTime(); fb.IsZero() {
		t.Fatal("FirstByteTime should be set after Read")
	}
}

func TestMeasuredStream_Close(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	m := &StreamMetrics{StreamID: 1, StartTime: time.Now().Add(-100 * time.Millisecond)}
	ms := NewMeasuredStream(&fakeStream{conn: client, id: 1}, m)

	if err := ms.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !m.Closed.Load() {
		t.Fatal("Closed should be true after Close")
	}
	if m.GetDuration() <= 0 {
		t.Fatal("Duration should be positive after Close")
	}

	// Double close should not panic.
	if err := ms.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestMeasuredStream_StreamID(t *testing.T) {
	_, client := net.Pipe()
	defer client.Close()

	m := &StreamMetrics{StreamID: 77, StartTime: time.Now()}
	ms := NewMeasuredStream(&fakeStream{conn: client, id: 77}, m)
	if ms.StreamID() != 77 {
		t.Fatalf("StreamID = %d, want 77", ms.StreamID())
	}
}

func TestMeasuredStream_Metrics(t *testing.T) {
	_, client := net.Pipe()
	defer client.Close()

	m := &StreamMetrics{StreamID: 1, StartTime: time.Now()}
	ms := NewMeasuredStream(&fakeStream{conn: client, id: 1}, m)
	if ms.Metrics() != m {
		t.Fatal("Metrics() should return the same pointer")
	}
}

func TestMeasuredStream_ReadError(t *testing.T) {
	server, client := net.Pipe()
	m := &StreamMetrics{StreamID: 1, StartTime: time.Now()}
	ms := NewMeasuredStream(&fakeStream{conn: client, id: 1}, m)

	// Close server side to cause read error (io.EOF or io.ErrClosedPipe).
	server.Close()

	buf := make([]byte, 64)
	_, err := ms.Read(buf)
	if err == nil {
		t.Fatal("expected error on read from closed pipe")
	}
	// io.EOF is NOT counted as an error; only real errors are.
	if err == io.EOF {
		if got := m.Errors.Load(); got != 0 {
			t.Fatalf("Errors = %d, want 0 (EOF is not an error)", got)
		}
	} else {
		if got := m.Errors.Load(); got != 1 {
			t.Fatalf("Errors = %d, want 1 for non-EOF error", got)
		}
	}
}

func TestMeasuredStream_WriteError(t *testing.T) {
	server, client := net.Pipe()
	m := &StreamMetrics{StreamID: 1, StartTime: time.Now()}
	ms := NewMeasuredStream(&fakeStream{conn: client, id: 1}, m)

	// Close server side to cause write error.
	server.Close()

	_, err := ms.Write([]byte("data"))
	if err == nil {
		// net.Pipe may buffer, so read to trigger the error.
		_, err = ms.Write([]byte("more data"))
	}
	if err != nil && m.Errors.Load() == 0 {
		t.Fatal("Errors should be > 0 after write error")
	}
}

func TestMeasuredStream_ReadEOFNotCountedAsError(t *testing.T) {
	server, client := net.Pipe()
	m := &StreamMetrics{StreamID: 1, StartTime: time.Now()}
	ms := NewMeasuredStream(&fakeStream{conn: client, id: 1}, m)

	// Write then close to produce EOF.
	go func() {
		server.Write([]byte("data"))
		server.Close()
	}()

	// Drain all data.
	buf := make([]byte, 64)
	for {
		_, err := ms.Read(buf)
		if err != nil {
			break
		}
	}
	// io.EOF is a normal stream termination, not counted as an error.
	if got := m.Errors.Load(); got != 0 {
		t.Fatalf("Errors = %d, want 0 (EOF should not count)", got)
	}
	// But bytes should still be counted.
	if got := m.BytesReceived.Load(); got == 0 {
		t.Fatal("BytesReceived should be > 0")
	}
}
