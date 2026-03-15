//go:build linux

package relay

import (
	"bytes"
	"io"
	"net"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

// TestSplicePairCreateClose verifies pipe pair creation and cleanup.
func TestSplicePairCreateClose(t *testing.T) {
	pair, err := newSplicePair()
	if err != nil {
		t.Fatal("failed to create splice pair:", err)
	}

	// Verify the fds are valid by writing and reading through the pipe.
	msg := []byte("pipe test")
	n, err := unix.Write(pair.w, msg)
	if err != nil {
		t.Fatal("write to pipe:", err)
	}
	if n != len(msg) {
		t.Fatalf("wrote %d, expected %d", n, len(msg))
	}

	buf := make([]byte, len(msg))
	n, err = unix.Read(pair.r, buf)
	if err != nil {
		t.Fatal("read from pipe:", err)
	}
	if !bytes.Equal(buf[:n], msg) {
		t.Fatalf("expected %q, got %q", msg, buf[:n])
	}

	pair.Close()

	// After close, operations should fail.
	_, err = unix.Write(pair.w, msg)
	if err == nil {
		t.Fatal("expected error writing to closed pipe")
	}
}

// TestSpliceRelayTCP verifies splice-based relay between two TCP connection pairs.
func TestSpliceRelayTCP(t *testing.T) {
	ln1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln1.Close()

	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln2.Close()

	acceptCh1 := make(chan net.Conn, 1)
	go func() {
		c, _ := ln1.Accept()
		acceptCh1 <- c
	}()
	acceptCh2 := make(chan net.Conn, 1)
	go func() {
		c, _ := ln2.Accept()
		acceptCh2 <- c
	}()

	outerA, err := net.Dial("tcp", ln1.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	innerA := <-acceptCh1

	outerB, err := net.Dial("tcp", ln2.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	innerB := <-acceptCh2

	// Use trySplice on the inner connections (raw TCP).
	done := make(chan struct{})
	var n1, n2 int64
	var spliceOK bool
	go func() {
		n1, n2, spliceOK = trySplice(innerA, innerB)
		close(done)
	}()

	msgA := []byte("splice a-to-b data")
	msgB := []byte("splice b-to-a response")

	var wg sync.WaitGroup
	var gotA, gotB []byte
	wg.Add(4)

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

	if !spliceOK {
		t.Fatal("trySplice returned false for raw TCP connections on Linux")
	}
	if !bytes.Equal(gotB, msgA) {
		t.Fatalf("outerB: expected %q, got %q", msgA, gotB)
	}
	if !bytes.Equal(gotA, msgB) {
		t.Fatalf("outerA: expected %q, got %q", msgB, gotA)
	}
	if n1 != int64(len(msgA)) {
		t.Errorf("aToB: expected %d, got %d", len(msgA), n1)
	}
	if n2 != int64(len(msgB)) {
		t.Errorf("bToA: expected %d, got %d", len(msgB), n2)
	}

	outerA.Close()
	outerB.Close()
}

// TestSpliceRelayLargeData verifies splice handles larger data correctly.
func TestSpliceRelayLargeData(t *testing.T) {
	const size = 1024 * 1024 // 1MB

	ln1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln1.Close()

	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln2.Close()

	acceptCh1 := make(chan net.Conn, 1)
	go func() {
		c, _ := ln1.Accept()
		acceptCh1 <- c
	}()
	acceptCh2 := make(chan net.Conn, 1)
	go func() {
		c, _ := ln2.Accept()
		acceptCh2 <- c
	}()

	outerA, err := net.Dial("tcp", ln1.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	innerA := <-acceptCh1

	outerB, err := net.Dial("tcp", ln2.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	innerB := <-acceptCh2

	done := make(chan struct{})
	go func() {
		Relay(innerA, innerB)
		close(done)
	}()

	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}

	var wg sync.WaitGroup
	var received []byte
	wg.Add(2)

	go func() {
		defer wg.Done()
		outerA.Write(data)
		outerA.(*net.TCPConn).CloseWrite()
	}()
	go func() {
		defer wg.Done()
		received, _ = io.ReadAll(outerB)
	}()

	wg.Wait()
	outerA.Close()
	outerB.Close()
	<-done

	if len(received) != size {
		t.Fatalf("expected %d bytes, got %d", size, len(received))
	}
	if !bytes.Equal(received, data) {
		t.Fatal("data corruption detected in splice relay")
	}
}
