package pool

import (
	"testing"
)

func TestGetPut(t *testing.T) {
	buf := Get(100)
	if len(buf) != 100 {
		t.Fatalf("Get(100): got len %d, want 100", len(buf))
	}
	// Put should not panic
	Put(buf)
}

func TestPoolSizes(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantCap int
	}{
		{"Small", 512, 1024},
		{"Medium", 8 * 1024, 16 * 1024},
		{"Large", 32 * 1024, 64 * 1024},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := Get(tc.size)
			if len(buf) != tc.size {
				t.Fatalf("len = %d, want %d", len(buf), tc.size)
			}
			if cap(buf) != tc.wantCap {
				t.Fatalf("cap = %d, want %d", cap(buf), tc.wantCap)
			}
			Put(buf)
		})
	}
}

func TestGetReuse(t *testing.T) {
	// Get a buffer, put it back, get again — should work without error.
	buf1 := Get(1024)
	Put(buf1)
	buf2 := Get(1024)
	// We can't guarantee buf2 == buf1 (GC may collect pooled items),
	// but it should have the correct size.
	if len(buf2) != 1024 {
		t.Fatalf("reuse: got len %d, want 1024", len(buf2))
	}
	Put(buf2)
}
