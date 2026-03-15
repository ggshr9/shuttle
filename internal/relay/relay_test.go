package relay

import (
	"bytes"
	"crypto/rand"
	"io"
	"net"
	"sync"
	"testing"
)

// TestBasicRelay verifies unidirectional data transfer through Relay.
func TestBasicRelay(t *testing.T) {
	a, b := net.Pipe()

	done := make(chan struct{})
	go func() {
		Relay(a, b)
		close(done)
	}()

	a.Close()
	b.Close()
	<-done

	// Proper test: use two pipe pairs.
	outerA, innerA := net.Pipe()
	innerB, outerB := net.Pipe()
	_ = innerA // used by Relay internally
	_ = innerB

	go func() {
		Relay(innerA, innerB)
	}()

	msg := []byte("hello from a")
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
}

// TestBidirectionalRelay verifies data flows in both directions using TCP.
func TestBidirectionalRelay(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	acceptCh := make(chan net.Conn, 1)
	go func() {
		c, e := ln.Accept()
		if e == nil {
			acceptCh <- c
		}
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	serverConn := <-acceptCh

	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln2.Close()

	acceptCh2 := make(chan net.Conn, 1)
	go func() {
		c, e := ln2.Accept()
		if e == nil {
			acceptCh2 <- c
		}
	}()

	clientConn2, err := net.Dial("tcp", ln2.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	serverConn2 := <-acceptCh2

	done := make(chan struct{})
	go func() {
		Relay(serverConn, serverConn2)
		close(done)
	}()

	msgA := []byte("hello from a side")
	msgB := []byte("hello from b side")

	var wg sync.WaitGroup
	var gotA, gotB []byte

	wg.Add(4)

	go func() {
		defer wg.Done()
		clientConn.Write(msgA)
		clientConn.(*net.TCPConn).CloseWrite()
	}()

	go func() {
		defer wg.Done()
		clientConn2.Write(msgB)
		clientConn2.(*net.TCPConn).CloseWrite()
	}()

	go func() {
		defer wg.Done()
		gotB, _ = io.ReadAll(clientConn2)
	}()
	go func() {
		defer wg.Done()
		gotA, _ = io.ReadAll(clientConn)
	}()

	wg.Wait()
	<-done

	if !bytes.Equal(gotB, msgA) {
		t.Fatalf("client2: expected %q, got %q", msgA, gotB)
	}
	if !bytes.Equal(gotA, msgB) {
		t.Fatalf("client1: expected %q, got %q", msgB, gotA)
	}

	clientConn.Close()
	clientConn2.Close()
}

// TestOneSideClose verifies that closing one side propagates EOF.
func TestOneSideClose(t *testing.T) {
	outerA, innerA := net.Pipe()
	innerB, outerB := net.Pipe()
	_ = innerA
	_ = innerB

	done := make(chan struct{})
	go func() {
		Relay(innerA, innerB)
		close(done)
	}()

	outerA.Close()

	got, err := io.ReadAll(outerB)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty read, got %d bytes", len(got))
	}
	outerB.Close()
	<-done
}

// TestCountedReadWriter verifies byte counting accuracy for writes.
func TestCountedReadWriter(t *testing.T) {
	a, b := net.Pipe()

	counted := NewCountedReadWriter(a)

	msg := []byte("test message 1234567890")

	go func() {
		counted.Write(msg)
		counted.Close()
	}()

	got, err := io.ReadAll(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("expected %q, got %q", msg, got)
	}

	if counted.Sent() != int64(len(msg)) {
		t.Fatalf("sent: expected %d, got %d", len(msg), counted.Sent())
	}
}

// TestCountedReadWriterReceive verifies receive byte counting.
func TestCountedReadWriterReceive(t *testing.T) {
	a, b := net.Pipe()

	counted := NewCountedReadWriter(a)

	msg := []byte("incoming data 0123456789")
	go func() {
		b.Write(msg)
		b.Close()
	}()

	got, err := io.ReadAll(counted)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("expected %q, got %q", msg, got)
	}
	if counted.Received() != int64(len(msg)) {
		t.Fatalf("received: expected %d, got %d", len(msg), counted.Received())
	}
}

// TestLargeTransfer relays 10MB and checks for data corruption.
func TestLargeTransfer(t *testing.T) {
	const size = 10 * 1024 * 1024

	outerA, innerA := net.Pipe()
	innerB, outerB := net.Pipe()
	_ = innerA
	_ = innerB

	done := make(chan struct{})
	go func() {
		Relay(innerA, innerB)
		close(done)
	}()

	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var received []byte
	var readErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		outerA.Write(data)
		outerA.Close()
	}()
	go func() {
		defer wg.Done()
		received, readErr = io.ReadAll(outerB)
	}()

	wg.Wait()
	outerB.Close()
	<-done

	if readErr != nil {
		t.Fatal("read error:", readErr)
	}
	if len(received) != size {
		t.Fatalf("expected %d bytes, got %d", size, len(received))
	}
	if !bytes.Equal(received, data) {
		t.Fatal("data corruption detected")
	}
}

// TestCanSplice verifies CanSplice detection.
func TestCanSplice(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	if CanSplice(a, b) {
		t.Log("net.Pipe supports syscall.Conn on this platform (unexpected but OK)")
	}
}

// TestRelayWithCountedWrapper verifies Relay works with CountedReadWriter using TCP.
func TestRelayWithCountedWrapper(t *testing.T) {
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

	msgA := []byte("from-a-to-b")
	msgB := []byte("from-b-to-a-longer")

	var wg sync.WaitGroup
	wg.Add(4)

	var gotA, gotB []byte

	go func() {
		defer wg.Done()
		outerA.Write(msgA)
		outerA.(*net.TCPConn).CloseWrite()
	}()

	go func() {
		defer wg.Done()
		outerB.Write(msgB)
		outerB.(*net.TCPConn).CloseWrite()
	}()

	go func() {
		defer wg.Done()
		gotB, _ = io.ReadAll(outerB)
	}()
	go func() {
		defer wg.Done()
		gotA, _ = io.ReadAll(outerA)
	}()

	wg.Wait()
	<-done

	if !bytes.Equal(gotB, msgA) {
		t.Fatalf("outerB: expected %q, got %q", msgA, gotB)
	}
	if !bytes.Equal(gotA, msgB) {
		t.Fatalf("outerA: expected %q, got %q", msgB, gotA)
	}

	if countedA.Received() != int64(len(msgA)) {
		t.Errorf("countedA received: expected %d, got %d", len(msgA), countedA.Received())
	}
	if countedA.Sent() != int64(len(msgB)) {
		t.Errorf("countedA sent: expected %d, got %d", len(msgB), countedA.Sent())
	}
	if countedB.Received() != int64(len(msgB)) {
		t.Errorf("countedB received: expected %d, got %d", len(msgB), countedB.Received())
	}
	if countedB.Sent() != int64(len(msgA)) {
		t.Errorf("countedB sent: expected %d, got %d", len(msgA), countedB.Sent())
	}

	outerA.Close()
	outerB.Close()
}

// BenchmarkRelay benchmarks relay performance using net.Pipe.
func BenchmarkRelay(b *testing.B) {
	data := make([]byte, 32*1024)
	rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		outerA, innerA := net.Pipe()
		innerB, outerB := net.Pipe()

		done := make(chan struct{})
		go func() {
			Relay(innerA, innerB)
			close(done)
		}()

		go func() {
			outerA.Write(data)
			outerA.Close()
		}()

		buf := make([]byte, len(data))
		io.ReadFull(outerB, buf)
		outerB.Close()
		<-done
	}
}

// BenchmarkRelayWithCounted benchmarks relay with CountedReadWriter wrappers.
func BenchmarkRelayWithCounted(b *testing.B) {
	data := make([]byte, 32*1024)
	rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		outerA, innerA := net.Pipe()
		innerB, outerB := net.Pipe()

		countedA := NewCountedReadWriter(innerA)
		countedB := NewCountedReadWriter(innerB)

		done := make(chan struct{})
		go func() {
			Relay(countedA, countedB)
			close(done)
		}()

		go func() {
			outerA.Write(data)
			outerA.Close()
		}()

		buf := make([]byte, len(data))
		io.ReadFull(outerB, buf)
		outerB.Close()
		<-done
	}
}
