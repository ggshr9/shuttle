package pool

import (
	"sync"
)

// BufferPool manages reusable byte buffers to reduce GC pressure.
var (
	// Small buffers (1KB) for headers/control data
	SmallPool = &sync.Pool{
		New: func() any {
			b := make([]byte, 1024)
			return &b
		},
	}

	// Medium buffers (16KB) for typical data transfer
	MediumPool = &sync.Pool{
		New: func() any {
			b := make([]byte, 16*1024)
			return &b
		},
	}

	// Large buffers (64KB) for high-throughput transfer
	LargePool = &sync.Pool{
		New: func() any {
			b := make([]byte, 64*1024)
			return &b
		},
	}
)

// Get returns a buffer of at least the given size from the appropriate pool.
func Get(size int) []byte {
	switch {
	case size <= 1024:
		bp := SmallPool.Get().(*[]byte)
		return (*bp)[:size]
	case size <= 16*1024:
		bp := MediumPool.Get().(*[]byte)
		return (*bp)[:size]
	default:
		bp := LargePool.Get().(*[]byte)
		return (*bp)[:min(size, 64*1024)]
	}
}

// Put zeroes a buffer and returns it to the appropriate pool.
// Zeroing prevents sensitive data (keys, plaintext) from leaking to
// subsequent pool users.
func Put(buf []byte) {
	c := cap(buf)
	b := buf[:c]
	clear(b)
	switch {
	case c <= 1024:
		SmallPool.Put(&b)
	case c <= 16*1024:
		MediumPool.Put(&b)
	default:
		LargePool.Put(&b)
	}
}
