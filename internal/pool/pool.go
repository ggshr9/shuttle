package pool

import (
	"sync"
)

// BufferPool manages reusable byte buffers to reduce GC pressure.
var (
	// Small buffers (1KB) for headers/control data
	SmallPool = &sync.Pool{
		New: func() any { return make([]byte, 1024) },
	}

	// Medium buffers (16KB) for typical data transfer
	MediumPool = &sync.Pool{
		New: func() any { return make([]byte, 16*1024) },
	}

	// Large buffers (64KB) for high-throughput transfer
	LargePool = &sync.Pool{
		New: func() any { return make([]byte, 64*1024) },
	}
)

// Get returns a buffer of at least the given size from the appropriate pool.
func Get(size int) []byte {
	switch {
	case size <= 1024:
		return SmallPool.Get().([]byte)[:size]
	case size <= 16*1024:
		return MediumPool.Get().([]byte)[:size]
	default:
		return LargePool.Get().([]byte)[:min(size, 64*1024)]
	}
}

// Put returns a buffer to the appropriate pool.
func Put(buf []byte) {
	c := cap(buf)
	switch {
	case c <= 1024:
		SmallPool.Put(buf[:c])
	case c <= 16*1024:
		MediumPool.Put(buf[:c])
	default:
		LargePool.Put(buf[:c])
	}
}
