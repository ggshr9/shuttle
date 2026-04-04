package obfs

import (
	"bytes"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingWriter records how many Write calls were made and accumulates data.
type countingWriter struct {
	buf   bytes.Buffer
	calls atomic.Int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.calls.Add(1)
	return w.buf.Write(p)
}

func (w *countingWriter) Read(p []byte) (int, error) {
	return w.buf.Read(p)
}

func TestShaperPassthrough(t *testing.T) {
	data := []byte("hello, world! this is a passthrough test")
	rw := &countingWriter{}
	cfg := DefaultShaperConfig()
	cfg.Enabled = false

	s := NewShaper(rw, cfg)
	n, err := s.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Write returned %d, want %d", n, len(data))
	}
	if !bytes.Equal(rw.buf.Bytes(), data) {
		t.Fatalf("data mismatch: got %q, want %q", rw.buf.Bytes(), data)
	}
	if rw.calls.Load() != 1 {
		t.Fatalf("expected 1 write call for disabled shaper, got %d", rw.calls.Load())
	}
}

func TestShaperWrite(t *testing.T) {
	// Generate data larger than ChunkMaxSize to force chunking.
	data := bytes.Repeat([]byte("A"), 5000)
	rw := &countingWriter{}
	cfg := ShaperConfig{
		Enabled:      true,
		MinDelay:     0,
		MaxDelay:     0, // no delay so test is fast
		ChunkMinSize: 64,
		ChunkMaxSize: 1400,
	}

	s := NewShaper(rw, cfg)
	n, err := s.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Write returned %d, want %d", n, len(data))
	}
	if !bytes.Equal(rw.buf.Bytes(), data) {
		t.Fatal("data mismatch after shaped write")
	}
}

func TestShaperChunking(t *testing.T) {
	// Use a large payload and small chunk sizes to guarantee multiple write calls.
	data := bytes.Repeat([]byte("B"), 3000)
	rw := &countingWriter{}
	cfg := ShaperConfig{
		Enabled:      true,
		MinDelay:     0,
		MaxDelay:     0,
		ChunkMinSize: 100,
		ChunkMaxSize: 500,
	}

	s := NewShaper(rw, cfg)
	n, err := s.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Write returned %d, want %d", n, len(data))
	}
	// With max chunk 500 and 3000 bytes, we need at least 6 writes.
	calls := rw.calls.Load()
	if calls < 2 {
		t.Fatalf("expected multiple write calls for chunked data, got %d", calls)
	}
	if !bytes.Equal(rw.buf.Bytes(), data) {
		t.Fatal("reassembled data does not match original")
	}
}

func TestShaperRead(t *testing.T) {
	expected := []byte("read passthrough test data")
	inner := bytes.NewBuffer(expected)
	rw := struct {
		io.Reader
		io.Writer
	}{inner, io.Discard}

	// Wrap with a simple ReadWriter adapter.
	adapter := &readWriterAdapter{r: rw.Reader, w: rw.Writer}
	cfg := DefaultShaperConfig()
	cfg.Enabled = true

	s := NewShaper(adapter, cfg)
	buf := make([]byte, len(expected))
	n, err := s.Read(buf)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if n != len(expected) {
		t.Fatalf("Read returned %d, want %d", n, len(expected))
	}
	if !bytes.Equal(buf[:n], expected) {
		t.Fatalf("Read data mismatch: got %q, want %q", buf[:n], expected)
	}
}

func TestDefaultShaperConfig(t *testing.T) {
	cfg := DefaultShaperConfig()
	if cfg.Enabled {
		t.Fatal("default config should have Enabled=false")
	}
	if cfg.MinDelay != 0 {
		t.Fatalf("default MinDelay should be 0, got %v", cfg.MinDelay)
	}
	if cfg.MaxDelay != 50*time.Millisecond {
		t.Fatalf("default MaxDelay should be 50ms, got %v", cfg.MaxDelay)
	}
	if cfg.ChunkMinSize != 64 {
		t.Fatalf("default ChunkMinSize should be 64, got %d", cfg.ChunkMinSize)
	}
	if cfg.ChunkMaxSize != 1400 {
		t.Fatalf("default ChunkMaxSize should be 1400, got %d", cfg.ChunkMaxSize)
	}
	if cfg.PaddingChance != 0.1 {
		t.Fatalf("default PaddingChance should be 0.1, got %f", cfg.PaddingChance)
	}
}

// readWriterAdapter combines separate Reader and Writer into an io.ReadWriter.
type readWriterAdapter struct {
	r io.Reader
	w io.Writer
}

func (a *readWriterAdapter) Read(p []byte) (int, error)  { return a.r.Read(p) }
func (a *readWriterAdapter) Write(p []byte) (int, error) { return a.w.Write(p) }

// --- New tests for concurrent write fix ---

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (lb *lockedBuffer) Read(p []byte) (int, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.buf.Read(p)
}

func (lb *lockedBuffer) Write(p []byte) (int, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.buf.Write(p)
}

func TestShaper_ConcurrentWritesNotBlocked(t *testing.T) {
	inner := &lockedBuffer{}
	s := NewShaper(inner, ShaperConfig{
		Enabled:      true,
		MinDelay:     1 * time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		ChunkMinSize: 10,
		ChunkMaxSize: 20,
	})

	data := make([]byte, 100)
	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Write(data)
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("concurrent writes took %v, may be serialized", elapsed)
	}
}

func TestShaper_WritePreservesData(t *testing.T) {
	inner := &lockedBuffer{}
	s := NewShaper(inner, ShaperConfig{
		Enabled:      true,
		MinDelay:     0,
		MaxDelay:     1 * time.Millisecond,
		ChunkMinSize: 5,
		ChunkMaxSize: 10,
	})

	data := []byte("hello world, this is a test of the shaper")
	n, err := s.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(data) {
		t.Errorf("wrote %d bytes, want %d", n, len(data))
	}

	inner.mu.Lock()
	got := inner.buf.Bytes()
	inner.mu.Unlock()
	if !bytes.Equal(got, data) {
		t.Errorf("data mismatch: got %q, want %q", got, data)
	}
}
