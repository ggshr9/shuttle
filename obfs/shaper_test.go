package obfs

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

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
