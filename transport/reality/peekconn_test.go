package reality

import (
	"io"
	"net"
	"testing"
	"time"
)

func TestPeekConn_ReplaysPrefix(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	// Write "world" from the far side.
	go func() {
		c2.Write([]byte("world"))
	}()

	pc := &peekConn{Conn: c1, prefix: []byte("hello")}
	buf := make([]byte, 10)
	n, err := io.ReadFull(pc, buf)
	if err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if n != 10 {
		t.Fatalf("expected 10 bytes, got %d", n)
	}
	if string(buf) != "helloworld" {
		t.Fatalf("expected helloworld, got %q", string(buf))
	}
}

func TestPeekConn_PartialReads(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		c2.Write([]byte("XY"))
	}()

	pc := &peekConn{Conn: c1, prefix: []byte("AB")}
	for _, want := range []byte{'A', 'B', 'X', 'Y'} {
		b := make([]byte, 1)
		if _, err := io.ReadFull(pc, b); err != nil {
			t.Fatalf("read: %v", err)
		}
		if b[0] != want {
			t.Fatalf("expected %c, got %c", want, b[0])
		}
	}
}

func TestPeekConn_NilPrefix(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		c2.Write([]byte("direct"))
	}()

	pc := &peekConn{Conn: c1}
	buf := make([]byte, 6)
	if _, err := io.ReadFull(pc, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "direct" {
		t.Fatalf("expected direct, got %q", string(buf))
	}
}

func TestPeekConn_ForwardsDeadlines(t *testing.T) {
	c1, _ := net.Pipe()
	defer c1.Close()
	pc := &peekConn{Conn: c1, prefix: []byte{1, 2, 3}}
	if err := pc.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
}
