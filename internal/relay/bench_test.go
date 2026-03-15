package relay

import (
	"io"
	"net"
	"testing"

	"github.com/shuttle-proxy/shuttle/internal/pool"
)

// BenchmarkRelayCopyBuffer benchmarks just the copyBuffer path with pooled buffers.
func BenchmarkRelayCopyBuffer(b *testing.B) {
	data := make([]byte, 32*1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, w := net.Pipe()

		done := make(chan struct{})
		go func() {
			defer close(done)
			buf := pool.GetMedLarge()
			io.CopyBuffer(io.Discard, r, buf)
			pool.PutMedLarge(buf)
		}()

		w.Write(data)
		w.Close()
		<-done
		r.Close()
	}
}

// BenchmarkRelayCopyBufferParallel benchmarks concurrent copyBuffer operations.
func BenchmarkRelayCopyBufferParallel(b *testing.B) {
	data := make([]byte, 32*1024)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r, w := net.Pipe()

			done := make(chan struct{})
			go func() {
				defer close(done)
				buf := pool.GetMedLarge()
				io.CopyBuffer(io.Discard, r, buf)
				pool.PutMedLarge(buf)
			}()

			w.Write(data)
			w.Close()
			<-done
			r.Close()
		}
	})
}
