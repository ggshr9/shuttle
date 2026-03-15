package relay

import (
	"bytes"
	"io"
	"net"
	"runtime"
	"sync"
	"testing"
)

// TestTrySpliceNonSpliceable verifies trySplice returns false for non-spliceable
// connections (net.Pipe does not implement syscall.Conn).
func TestTrySpliceNonSpliceable(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	_, _, ok := trySplice(a, b)
	if ok {
		t.Fatal("trySplice should return false for net.Pipe connections")
	}
}

// TestTrySpliceCountedWrapper verifies that CountedReadWriter connections
// are never passed to splice, even if the inner connection supports it.
func TestTrySpliceCountedWrapper(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	acceptCh := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		acceptCh <- c
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server := <-acceptCh
	defer client.Close()
	defer server.Close()

	counted := NewCountedReadWriter(client)
	_, _, ok := trySplice(counted, server)
	if ok {
		t.Fatal("trySplice should return false when one side is CountedReadWriter")
	}

	_, _, ok = trySplice(server, counted)
	if ok {
		t.Fatal("trySplice should return false when one side is CountedReadWriter")
	}
}

// TestRelayFallback verifies Relay works correctly with non-spliceable connections
// (net.Pipe), confirming the userspace fallback path.
func TestRelayFallback(t *testing.T) {
	outerA, innerA := net.Pipe()
	innerB, outerB := net.Pipe()

	done := make(chan struct{})
	go func() {
		Relay(innerA, innerB)
		close(done)
	}()

	msg := []byte("fallback relay test data")
	go func() {
		outerA.Write(msg)
		outerA.Close()
	}()

	got, err := io.ReadAll(outerB)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("expected %q, got %q", msg, got)
	}
	outerB.Close()
	<-done
}

// TestRelayWithCountedBypassesSplice verifies that when Relay is called with
// CountedReadWriter wrappers, splice is bypassed and byte counting works.
func TestRelayWithCountedBypassesSplice(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln2.Close()

	acceptCh := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		acceptCh <- c
	}()
	acceptCh2 := make(chan net.Conn, 1)
	go func() {
		c, _ := ln2.Accept()
		acceptCh2 <- c
	}()

	outerA, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	innerA := <-acceptCh

	outerB, err := net.Dial("tcp", ln2.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	innerB := <-acceptCh2

	countedA := NewCountedReadWriter(innerA)
	countedB := NewCountedReadWriter(innerB)

	done := make(chan struct{})
	go func() {
		Relay(countedA, countedB)
		close(done)
	}()

	msg := []byte("counted relay test")

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		outerA.Write(msg)
		outerA.(*net.TCPConn).CloseWrite()
	}()

	var got []byte
	go func() {
		defer wg.Done()
		got, _ = io.ReadAll(outerB)
	}()

	wg.Wait()
	outerA.Close()
	outerB.Close()
	<-done

	if !bytes.Equal(got, msg) {
		t.Fatalf("expected %q, got %q", msg, got)
	}

	// Verify byte counting worked (proves splice was not used).
	if countedA.Received() != int64(len(msg)) {
		t.Errorf("countedA received: expected %d, got %d", len(msg), countedA.Received())
	}
	if countedB.Sent() != int64(len(msg)) {
		t.Errorf("countedB sent: expected %d, got %d", len(msg), countedB.Sent())
	}
}

// TestTrySpliceNonLinux verifies that on non-Linux platforms trySplice always
// returns false, even for TCP connections that implement syscall.Conn.
func TestTrySpliceNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("this test is for non-Linux platforms")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	acceptCh := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		acceptCh <- c
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server := <-acceptCh
	defer client.Close()
	defer server.Close()

	_, _, ok := trySplice(client, server)
	if ok {
		t.Fatal("trySplice should return false on non-Linux platforms")
	}
}
