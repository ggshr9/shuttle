package reality

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/ggshr9/shuttle/crypto"
	"golang.org/x/crypto/curve25519"
)

func TestNewClient(t *testing.T) {
	c := NewClient(&ClientConfig{
		ServerAddr: "127.0.0.1:443",
		ServerName: "example.com",
		Password:   "secret",
	})
	if c.Type() != "reality" {
		t.Fatalf("expected type reality, got %s", c.Type())
	}
}

func TestClientClosedDial(t *testing.T) {
	c := NewClient(&ClientConfig{
		ServerAddr: "127.0.0.1:443",
		Password:   "secret",
	})
	c.Close()
	_, err := c.Dial(context.Background(), "")
	if err == nil {
		t.Fatal("expected error dialing closed client")
	}
}

func TestNewServer(t *testing.T) {
	// Valid 32-byte hex key required
	s, err := NewServer(&ServerConfig{
		ListenAddr: ":0",
		PrivateKey: "0102030405060708091011121314151617181920212223242526272829303132",
	}, nil)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	if s.Type() != "reality" {
		t.Fatalf("expected type reality, got %s", s.Type())
	}
}

func TestNewServerMissingKey(t *testing.T) {
	_, err := NewServer(&ServerConfig{
		ListenAddr: ":0",
	}, nil)
	if err == nil {
		t.Fatal("expected error for missing private key")
	}
}

func TestNewServerWithPrivateKey(t *testing.T) {
	privKey := make([]byte, 32)
	for i := range privKey {
		privKey[i] = byte(i + 1)
	}
	privHex := hex.EncodeToString(privKey)

	s, err := NewServer(&ServerConfig{
		PrivateKey: privHex,
	}, nil)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	clamped := make([]byte, 32)
	copy(clamped, privKey)
	clamped[0] &= 248
	clamped[31] &= 127
	clamped[31] |= 64
	expectedPub, _ := curve25519.X25519(clamped, curve25519.Basepoint)

	if !bytes.Equal(s.pubKey[:], expectedPub) {
		t.Fatal("public key derivation mismatch")
	}
}

func TestNewServerInvalidKey(t *testing.T) {
	_, err := NewServer(&ServerConfig{
		PrivateKey: "not-hex",
	}, nil)
	if err == nil {
		t.Fatal("expected error for invalid hex key")
	}
}

func TestServerCloseBeforeListen(t *testing.T) {
	s, err := NewServer(&ServerConfig{
		PrivateKey: "0102030405060708091011121314151617181920212223242526272829303132",
	}, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	err = s.Close()
	if err != nil {
		t.Fatalf("Close before Listen: %v", err)
	}
}

func TestServerAcceptTimeout(t *testing.T) {
	s, err := NewServer(&ServerConfig{
		PrivateKey: "0102030405060708091011121314151617181920212223242526272829303132",
	}, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, acceptErr := s.Accept(ctx)
	if acceptErr == nil {
		t.Fatal("expected timeout error")
	}
}

func TestWriteFrame(t *testing.T) {
	var buf bytes.Buffer
	data := []byte("hello reality")
	if err := writeFrame(&buf, data); err != nil {
		t.Fatalf("writeFrame: %v", err)
	}
	result := buf.Bytes()
	length := binary.BigEndian.Uint16(result[:2])
	if int(length) != len(data) {
		t.Fatalf("frame length = %d, want %d", length, len(data))
	}
	if !bytes.Equal(result[2:], data) {
		t.Fatalf("frame payload mismatch")
	}
}

func TestReadFrame(t *testing.T) {
	data := []byte("test frame data")
	var buf bytes.Buffer
	writeFrame(&buf, data)

	result, err := readFrame(&buf)
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if !bytes.Equal(result, data) {
		t.Fatalf("read frame mismatch: got %q, want %q", result, data)
	}
}

func TestReadFrameTooLarge(t *testing.T) {
	header := make([]byte, 2)
	binary.BigEndian.PutUint16(header, 65535)
	buf := bytes.NewReader(header)
	_, err := readFrame(buf)
	if err == nil {
		t.Fatal("expected error for truncated large frame")
	}
}

func TestReadFrameEmpty(t *testing.T) {
	buf := bytes.NewReader(nil)
	_, err := readFrame(buf)
	if err == nil {
		t.Fatal("expected error for empty reader")
	}
}

func TestWriteReadFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	frames := [][]byte{
		[]byte("frame one"),
		[]byte("frame two longer data"),
		[]byte("f3"),
	}

	for _, f := range frames {
		if err := writeFrame(&buf, f); err != nil {
			t.Fatalf("writeFrame: %v", err)
		}
	}

	reader := bytes.NewReader(buf.Bytes())
	for i, expected := range frames {
		got, err := readFrame(reader)
		if err != nil {
			t.Fatalf("readFrame %d: %v", i, err)
		}
		if !bytes.Equal(got, expected) {
			t.Fatalf("frame %d mismatch: got %q, want %q", i, got, expected)
		}
	}
}

// TestRealityEndToEnd tests the full Reality handshake with Noise IK + yamux over net.Pipe.
func TestRealityEndToEnd(t *testing.T) {
	password := "e2e-password"

	serverPub, serverPriv, err := crypto.DeriveKeysFromPassword(password)
	if err != nil {
		t.Fatalf("derive server keys: %v", err)
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	errCh := make(chan error, 1)

	// Server side
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

	// Client side
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

	sess, err := yamux.Client(clientConn, yamux.DefaultConfig())
	if err != nil {
		t.Fatalf("yamux client: %v", err)
	}

	stream, err := sess.OpenStream()
	if err != nil {
		t.Fatalf("yamux open: %v", err)
	}

	testData := []byte("hello reality transport")
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

// FuzzReadFrame tests frame parsing with random inputs.
func FuzzReadFrame(f *testing.F) {
	f.Add([]byte{0x00, 0x05, 'h', 'e', 'l', 'l', 'o'})
	f.Add([]byte{0x00, 0x00})
	f.Add([]byte{0xFF, 0xFF})
	f.Add([]byte{0x00})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		reader := bytes.NewReader(data)
		// Should not panic
		readFrame(reader)
	})
}
