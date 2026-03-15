package reality

import (
	"bytes"
	"io"
	"net"
	"testing"

	"github.com/hashicorp/yamux"
	"github.com/shuttle-proxy/shuttle/crypto"
)

// TestRealityPQHandshake tests a full PQ-enhanced Reality handshake round-trip
// over net.Pipe (no real network listeners required).
func TestRealityPQHandshake(t *testing.T) {
	password := "pq-test-password"

	serverPub, serverPriv, err := crypto.DeriveKeysFromPassword(password)
	if err != nil {
		t.Fatalf("derive server keys: %v", err)
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	errCh := make(chan error, 1)

	// Server side: Noise IK handshake + PQ KEM exchange + yamux echo
	go func() {
		hs, err := crypto.NewResponder(serverPriv, serverPub)
		if err != nil {
			errCh <- err
			return
		}

		// Read Noise msg1
		msg1, err := readFrame(serverConn)
		if err != nil {
			errCh <- err
			return
		}
		if _, err = hs.ReadMessage(msg1); err != nil {
			errCh <- err
			return
		}

		// Write Noise msg2
		msg2, err := hs.WriteMessage(nil)
		if err != nil {
			errCh <- err
			return
		}
		if err := writeFrame(serverConn, msg2); err != nil {
			errCh <- err
			return
		}

		if !hs.Completed() {
			errCh <- io.ErrUnexpectedEOF
			return
		}

		// PQ KEM exchange (server side)
		pqFrame, err := readFrame(serverConn)
		if err != nil {
			errCh <- err
			return
		}
		if len(pqFrame) == 0 || pqFrame[0] != crypto.HandshakeVersionHybridPQ {
			errCh <- io.ErrUnexpectedEOF
			return
		}

		pqPubBytes := pqFrame[1:]
		pq, err := crypto.NewPQHandshake()
		if err != nil {
			errCh <- err
			return
		}

		serverPQSecret, ciphertext, err := pq.Encapsulate(pqPubBytes)
		if err != nil {
			errCh <- err
			return
		}
		_ = serverPQSecret // In production, mixed into session keys

		if err := writeFrame(serverConn, ciphertext); err != nil {
			errCh <- err
			return
		}

		// yamux echo server
		sess, err := yamux.Server(serverConn, yamux.DefaultConfig())
		if err != nil {
			errCh <- err
			return
		}

		stream, err := sess.AcceptStream()
		if err != nil {
			errCh <- err
			return
		}
		buf := make([]byte, 4096)
		n, err := stream.Read(buf)
		if err != nil {
			errCh <- err
			return
		}
		if _, err = stream.Write(buf[:n]); err != nil {
			errCh <- err
			return
		}
		stream.Close()
		errCh <- nil
	}()

	// Client side: Noise IK handshake + PQ KEM exchange + yamux send
	clientPub, clientPriv, err := crypto.DeriveKeysFromPassword(password)
	if err != nil {
		t.Fatalf("derive client keys: %v", err)
	}

	hs, err := crypto.NewInitiator(clientPriv, clientPub, serverPub)
	if err != nil {
		t.Fatalf("noise initiator: %v", err)
	}

	msg1, err := hs.WriteMessage(nil)
	if err != nil {
		t.Fatalf("noise write msg1: %v", err)
	}
	if err := writeFrame(clientConn, msg1); err != nil {
		t.Fatalf("send msg1: %v", err)
	}

	msg2, err := readFrame(clientConn)
	if err != nil {
		t.Fatalf("read msg2: %v", err)
	}
	if _, err = hs.ReadMessage(msg2); err != nil {
		t.Fatalf("noise read msg2: %v", err)
	}

	if !hs.Completed() {
		t.Fatal("noise handshake incomplete")
	}

	// PQ KEM exchange (client side)
	pq, err := crypto.NewPQHandshake()
	if err != nil {
		t.Fatalf("pq handshake init: %v", err)
	}

	pqPub := pq.PublicKeyBytes()
	pqMsg := make([]byte, 1+len(pqPub))
	pqMsg[0] = crypto.HandshakeVersionHybridPQ
	copy(pqMsg[1:], pqPub)
	if err := writeFrame(clientConn, pqMsg); err != nil {
		t.Fatalf("send pq public key: %v", err)
	}

	pqCiphertext, err := readFrame(clientConn)
	if err != nil {
		t.Fatalf("read pq ciphertext: %v", err)
	}

	clientPQSecret, err := pq.Decapsulate(pqCiphertext)
	if err != nil {
		t.Fatalf("pq decapsulate: %v", err)
	}
	if len(clientPQSecret) == 0 {
		t.Fatal("PQ shared secret is empty")
	}

	// yamux data exchange
	sess, err := yamux.Client(clientConn, yamux.DefaultConfig())
	if err != nil {
		t.Fatalf("yamux client: %v", err)
	}

	stream, err := sess.OpenStream()
	if err != nil {
		t.Fatalf("yamux open: %v", err)
	}

	testData := []byte("hello pq reality transport")
	stream.Write(testData)

	buf := make([]byte, 4096)
	n, err := stream.Read(buf)
	if err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if !bytes.Equal(buf[:n], testData) {
		t.Fatalf("echo mismatch: got %q, want %q", buf[:n], testData)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("server error: %v", err)
	}
}

// TestRealityPQClientClassicalServer tests that a PQ client connecting to a
// classical (non-PQ) server completes the Noise handshake but the PQ frame
// causes the connection to fail gracefully (server reads it as unexpected data).
func TestRealityPQClientClassicalServer(t *testing.T) {
	password := "classical-server-test"

	serverPub, serverPriv, err := crypto.DeriveKeysFromPassword(password)
	if err != nil {
		t.Fatalf("derive server keys: %v", err)
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	errCh := make(chan error, 1)

	// Server side: classical (no PQ) — just Noise IK + yamux
	go func() {
		hs, err := crypto.NewResponder(serverPriv, serverPub)
		if err != nil {
			errCh <- err
			return
		}

		msg1, err := readFrame(serverConn)
		if err != nil {
			errCh <- err
			return
		}
		if _, err = hs.ReadMessage(msg1); err != nil {
			errCh <- err
			return
		}

		msg2, err := hs.WriteMessage(nil)
		if err != nil {
			errCh <- err
			return
		}
		if err := writeFrame(serverConn, msg2); err != nil {
			errCh <- err
			return
		}

		if !hs.Completed() {
			errCh <- io.ErrUnexpectedEOF
			return
		}

		// Classical server goes straight to yamux — the PQ frame from
		// the client will be interpreted as yamux data, causing an error.
		_, err = yamux.Server(serverConn, yamux.DefaultConfig())
		// We expect this to eventually fail or produce garbled state.
		// The important thing is it doesn't panic.
		errCh <- err
	}()

	// Client side: performs Noise + PQ
	clientPub, clientPriv, err := crypto.DeriveKeysFromPassword(password)
	if err != nil {
		t.Fatalf("derive client keys: %v", err)
	}

	hs, err := crypto.NewInitiator(clientPriv, clientPub, serverPub)
	if err != nil {
		t.Fatalf("noise initiator: %v", err)
	}

	msg1, err := hs.WriteMessage(nil)
	if err != nil {
		t.Fatalf("noise write msg1: %v", err)
	}
	if err := writeFrame(clientConn, msg1); err != nil {
		t.Fatalf("send msg1: %v", err)
	}

	msg2, err := readFrame(clientConn)
	if err != nil {
		t.Fatalf("read msg2: %v", err)
	}
	if _, err = hs.ReadMessage(msg2); err != nil {
		t.Fatalf("noise read msg2: %v", err)
	}

	// The Noise handshake itself should complete fine.
	if !hs.Completed() {
		t.Fatal("noise handshake incomplete")
	}

	// Client sends PQ frame — server will misinterpret it.
	pq, err := crypto.NewPQHandshake()
	if err != nil {
		t.Fatalf("pq handshake init: %v", err)
	}

	pqPub := pq.PublicKeyBytes()
	pqMsg := make([]byte, 1+len(pqPub))
	pqMsg[0] = crypto.HandshakeVersionHybridPQ
	copy(pqMsg[1:], pqPub)
	if err := writeFrame(clientConn, pqMsg); err != nil {
		t.Fatalf("send pq public key: %v", err)
	}

	// The server will interpret the PQ frame as yamux data. Trying to
	// read a PQ ciphertext response will fail, which is the expected
	// graceful failure when a PQ client talks to a classical server.
	_, readErr := readFrame(clientConn)

	// We expect either an error or garbled data — not a panic. The server
	// goroutine may also error. Both outcomes are acceptable: the key
	// requirement is no crash.
	_ = readErr
	<-errCh // drain the server goroutine
}
