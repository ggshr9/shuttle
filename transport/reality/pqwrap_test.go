package reality

import (
	"bytes"
	"crypto/rand"
	"io"
	"net"
	"sync"
	"testing"
)

// TestPQWrapRoundTrip verifies that two connections wrapped with the same
// PQ shared secret can exchange data correctly through the AEAD layer.
func TestPQWrapRoundTrip(t *testing.T) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}

	clientRaw, serverRaw := net.Pipe()
	defer clientRaw.Close()
	defer serverRaw.Close()

	clientConn, err := wrapConnWithPQ(clientRaw, secret)
	if err != nil {
		t.Fatalf("wrap client: %v", err)
	}
	serverConn, err := wrapConnWithPQ(serverRaw, secret)
	if err != nil {
		t.Fatalf("wrap server: %v", err)
	}

	testData := []byte("hello post-quantum world!")

	var wg sync.WaitGroup
	wg.Add(1)

	// Server side: read and echo back.
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		n, err := serverConn.Read(buf)
		if err != nil {
			t.Errorf("server read: %v", err)
			return
		}
		if _, err := serverConn.Write(buf[:n]); err != nil {
			t.Errorf("server write: %v", err)
		}
	}()

	// Client side: write and read echo.
	if _, err := clientConn.Write(testData); err != nil {
		t.Fatalf("client write: %v", err)
	}

	buf := make([]byte, 4096)
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("client read: %v", err)
	}

	if !bytes.Equal(buf[:n], testData) {
		t.Fatalf("round-trip mismatch: got %q, want %q", buf[:n], testData)
	}

	wg.Wait()
}

// TestPQWrapLargeData tests that data larger than pqFrameMaxPlaintext is
// correctly chunked and reassembled.
func TestPQWrapLargeData(t *testing.T) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}

	clientRaw, serverRaw := net.Pipe()
	defer clientRaw.Close()
	defer serverRaw.Close()

	clientConn, err := wrapConnWithPQ(clientRaw, secret)
	if err != nil {
		t.Fatalf("wrap client: %v", err)
	}
	serverConn, err := wrapConnWithPQ(serverRaw, secret)
	if err != nil {
		t.Fatalf("wrap server: %v", err)
	}

	// 50KB > pqFrameMaxPlaintext (16384), forces multiple frames.
	testData := make([]byte, 50*1024)
	if _, err := rand.Read(testData); err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)

	// Server side: read all data.
	go func() {
		var received bytes.Buffer
		buf := make([]byte, 4096)
		for received.Len() < len(testData) {
			n, err := serverConn.Read(buf)
			if err != nil {
				errCh <- err
				return
			}
			received.Write(buf[:n])
		}
		if !bytes.Equal(received.Bytes(), testData) {
			errCh <- io.ErrUnexpectedEOF
			return
		}
		errCh <- nil
	}()

	if _, err := clientConn.Write(testData); err != nil {
		t.Fatalf("client write: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("server error: %v", err)
	}
}

// TestPQWrapMismatchedSecrets verifies that connections wrapped with different
// secrets fail to communicate (AEAD decryption error).
func TestPQWrapMismatchedSecrets(t *testing.T) {
	secret1 := make([]byte, 32)
	secret2 := make([]byte, 32)
	if _, err := rand.Read(secret1); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(secret2); err != nil {
		t.Fatal(err)
	}

	clientRaw, serverRaw := net.Pipe()
	defer clientRaw.Close()
	defer serverRaw.Close()

	clientConn, err := wrapConnWithPQ(clientRaw, secret1)
	if err != nil {
		t.Fatalf("wrap client: %v", err)
	}
	serverConn, err := wrapConnWithPQ(serverRaw, secret2) // different secret
	if err != nil {
		t.Fatalf("wrap server: %v", err)
	}

	errCh := make(chan error, 1)

	go func() {
		buf := make([]byte, 4096)
		_, err := serverConn.Read(buf)
		errCh <- err
	}()

	if _, err := clientConn.Write([]byte("should fail")); err != nil {
		t.Fatalf("client write: %v", err)
	}

	readErr := <-errCh
	if readErr == nil {
		t.Fatal("expected decryption error with mismatched secrets, got nil")
	}
	t.Logf("expected error with mismatched secrets: %v", readErr)
}

// TestPQWrapEmptySecret verifies that an empty secret is rejected.
func TestPQWrapEmptySecret(t *testing.T) {
	conn, _ := net.Pipe()
	defer conn.Close()

	_, err := wrapConnWithPQ(conn, nil)
	if err == nil {
		t.Fatal("expected error for nil secret")
	}

	_, err = wrapConnWithPQ(conn, []byte{})
	if err == nil {
		t.Fatal("expected error for empty secret")
	}
}

// TestPQWrapPartialRead verifies that reading less than a full frame's worth
// of plaintext correctly buffers the remainder for subsequent reads.
func TestPQWrapPartialRead(t *testing.T) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}

	clientRaw, serverRaw := net.Pipe()
	defer clientRaw.Close()
	defer serverRaw.Close()

	clientConn, err := wrapConnWithPQ(clientRaw, secret)
	if err != nil {
		t.Fatalf("wrap client: %v", err)
	}
	serverConn, err := wrapConnWithPQ(serverRaw, secret)
	if err != nil {
		t.Fatalf("wrap server: %v", err)
	}

	testData := []byte("abcdefghijklmnopqrstuvwxyz")

	go func() {
		clientConn.Write(testData)
	}()

	// Read in small chunks (5 bytes at a time).
	var received bytes.Buffer
	buf := make([]byte, 5)
	for received.Len() < len(testData) {
		n, err := serverConn.Read(buf)
		if err != nil {
			t.Fatalf("partial read: %v", err)
		}
		received.Write(buf[:n])
	}

	if !bytes.Equal(received.Bytes(), testData) {
		t.Fatalf("partial read mismatch: got %q, want %q", received.Bytes(), testData)
	}
}
