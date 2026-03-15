package pool

import (
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
)

func BenchmarkPoolGet(b *testing.B) {
	for i := 0; i < b.N; i++ {
		buf := Get(32 * 1024)
		Put(buf)
	}
}

func BenchmarkPoolGetParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := Get(32 * 1024)
			Put(buf)
		}
	})
}

func BenchmarkPoolGetSmall(b *testing.B) {
	for i := 0; i < b.N; i++ {
		buf := Get(512)
		Put(buf)
	}
}

func BenchmarkPoolGetMedium(b *testing.B) {
	for i := 0; i < b.N; i++ {
		buf := Get(16 * 1024)
		Put(buf)
	}
}

func BenchmarkPoolGetMedLarge(b *testing.B) {
	for i := 0; i < b.N; i++ {
		buf := GetMedLarge()
		PutMedLarge(buf)
	}
}

func BenchmarkPoolGetLarge(b *testing.B) {
	for i := 0; i < b.N; i++ {
		buf := Get(64 * 1024)
		Put(buf)
	}
}

var connIDSink string

func BenchmarkConnIDSprintf(b *testing.B) {
	var seq uint64
	for i := 0; i < b.N; i++ {
		connIDSink = fmt.Sprintf("%08x", atomic.AddUint64(&seq, 1))
	}
}

func BenchmarkConnIDFormatUint(b *testing.B) {
	var seq uint64
	for i := 0; i < b.N; i++ {
		s := atomic.AddUint64(&seq, 1)
		connIDSink = strconv.FormatUint(s, 16)
	}
}
