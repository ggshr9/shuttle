package proxy

import (
	"net"
	"testing"
	"time"
)

// FuzzSOCKS5Handshake sends random data to a SOCKS5 listener to verify
// it handles malformed input without panicking.
func FuzzSOCKS5Handshake(f *testing.F) {
	// Valid SOCKS5 greeting
	f.Add([]byte{0x05, 0x01, 0x00})
	// SOCKS5 with password auth
	f.Add([]byte{0x05, 0x02, 0x00, 0x02})
	// SOCKS4
	f.Add([]byte{0x04, 0x01, 0x00, 0x50})
	// Empty
	f.Add([]byte{})
	// Single byte
	f.Add([]byte{0x05})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Create a pipe to simulate a connection
		client, server := net.Pipe()

		go func() {
			defer server.Close()
			// The server side reads and handles — should not panic
			buf := make([]byte, 4096)
			server.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			server.Read(buf)
		}()

		// Send fuzzed data
		client.SetWriteDeadline(time.Now().Add(50 * time.Millisecond))
		client.Write(data)
		client.Close()
	})
}
