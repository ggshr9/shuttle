package vmess_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/transport/vmess"
)

// testUUID is a fixed UUID for testing: "01234567-89ab-cdef-0123-456789abcdef"
var testUUID = func() [16]byte {
	var u [16]byte
	b, _ := hex.DecodeString("0123456789abcdef0123456789abcdef")
	copy(u[:], b)
	return u
}()

// echoServer accepts connections and echoes all data back.
func echoServer(t *testing.T, ln net.Listener) {
	t.Helper()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			io.Copy(conn, conn) //nolint:errcheck
		}()
	}
}

func TestVMess_EchoThroughServer(t *testing.T) {
	// 1. Start echo backend.
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoLn.Close()
	go echoServer(t, echoLn)

	echoAddr := echoLn.Addr().String()

	// 2. Start VMess server (no TLS).
	serverLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer serverLn.Close()

	srv := vmess.NewServer(vmess.ServerConfig{
		Users: map[[16]byte]string{testUUID: "testuser"},
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		srv.Serve(ctx, serverLn, func(_ context.Context, cmd byte, address string, conn net.Conn) {
			defer conn.Close()

			if cmd != vmess.CmdTCP {
				t.Errorf("expected CmdTCP, got 0x%02x", cmd)
				return
			}

			// Dial echo backend and relay.
			backend, err := net.Dial("tcp", address)
			if err != nil {
				t.Errorf("handler dial: %v", err)
				return
			}
			defer backend.Close()

			var relayWg sync.WaitGroup
			relayWg.Add(2)
			go func() {
				defer relayWg.Done()
				io.Copy(backend, conn) //nolint:errcheck
			}()
			go func() {
				defer relayWg.Done()
				io.Copy(conn, backend) //nolint:errcheck
			}()
			relayWg.Wait()
		})
	}()

	// 3. Create VMess client dialer (no TLS).
	dialer, err := vmess.NewDialer(&vmess.DialerConfig{
		Server:   serverLn.Addr().String(),
		UUID:     testUUID,
		Security: vmess.SecurityNone,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 4. Dial through VMess to the echo backend.
	conn, err := dialer.DialContext(ctx, "tcp", echoAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// 5. Echo test.
	msg := "hello vmess aead"
	if _, err := fmt.Fprint(conn, msg); err != nil {
		t.Fatal(err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}

	if got := string(buf); got != msg {
		t.Fatalf("echo mismatch: got %q, want %q", got, msg)
	}
}

func TestVMess_BadUUIDRejected(t *testing.T) {
	serverLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer serverLn.Close()

	srv := vmess.NewServer(vmess.ServerConfig{
		Users: map[[16]byte]string{testUUID: "testuser"},
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		srv.Serve(ctx, serverLn, func(_ context.Context, _ byte, _ string, conn net.Conn) {
			defer conn.Close()
			t.Error("handler should not be called for bad UUID")
		})
	}()

	// Use a different UUID.
	var badUUID [16]byte
	badUUID[0] = 0xFF

	dialer, err := vmess.NewDialer(&vmess.DialerConfig{
		Server:   serverLn.Addr().String(),
		UUID:     badUUID,
		Security: vmess.SecurityNone,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = dialer.DialContext(ctx, "tcp", "127.0.0.1:9999")
	if err == nil {
		t.Fatal("expected error for bad UUID, got nil")
	}
}

func TestVMess_LargePayload(t *testing.T) {
	// 1. Start echo backend.
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoLn.Close()
	go echoServer(t, echoLn)

	echoAddr := echoLn.Addr().String()

	// 2. Start VMess server.
	serverLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer serverLn.Close()

	srv := vmess.NewServer(vmess.ServerConfig{
		Users: map[[16]byte]string{testUUID: "testuser"},
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		srv.Serve(ctx, serverLn, func(_ context.Context, _ byte, address string, conn net.Conn) {
			defer conn.Close()
			backend, err := net.Dial("tcp", address)
			if err != nil {
				return
			}
			defer backend.Close()

			var relayWg sync.WaitGroup
			relayWg.Add(2)
			go func() {
				defer relayWg.Done()
				io.Copy(backend, conn) //nolint:errcheck
			}()
			go func() {
				defer relayWg.Done()
				io.Copy(conn, backend) //nolint:errcheck
			}()
			relayWg.Wait()
		})
	}()

	// 3. Dial through VMess.
	dialer, err := vmess.NewDialer(&vmess.DialerConfig{
		Server:   serverLn.Addr().String(),
		UUID:     testUUID,
		Security: vmess.SecurityNone,
	})
	if err != nil {
		t.Fatal(err)
	}

	conn, err := dialer.DialContext(ctx, "tcp", echoAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Send 1 MB payload.
	payload := make([]byte, 1<<20)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	go func() {
		conn.Write(payload) //nolint:errcheck
	}()

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	received := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, received); err != nil {
		t.Fatalf("read full: %v", err)
	}

	if !bytes.Equal(payload, received) {
		t.Fatal("large payload mismatch")
	}
}

func TestVMess_ProtocolRoundtrip(t *testing.T) {
	// Test protocol encoding/decoding without a network connection.
	hdr := &vmess.RequestHeader{
		Version:  vmess.Version,
		Command:  vmess.CmdTCP,
		Security: vmess.SecurityAES128GCM,
		Address:  "example.com:443",
	}
	copy(hdr.DataIV[:], []byte{1, 2, 3, 4, 5, 6, 7, 8})
	copy(hdr.DataKey[:], []byte{9, 10, 11, 12, 13, 14, 15, 16})

	var buf bytes.Buffer
	timestamp := time.Now().Unix()

	if err := vmess.EncodeRequest(&buf, testUUID, timestamp, hdr); err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := vmess.DecodeRequest(&buf, func(authID [vmess.AuthIDLen]byte) ([16]byte, bool) {
		// Accept any auth_id and return our test UUID.
		candidate := vmess.ComputeAuthIDForTest(testUUID, timestamp)
		if authID == candidate {
			return testUUID, true
		}
		return [16]byte{}, false
	})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.Version != hdr.Version {
		t.Errorf("version: got %d, want %d", decoded.Version, hdr.Version)
	}
	if decoded.Command != hdr.Command {
		t.Errorf("command: got %d, want %d", decoded.Command, hdr.Command)
	}
	if decoded.Security != hdr.Security {
		t.Errorf("security: got %d, want %d", decoded.Security, hdr.Security)
	}
	if decoded.Address != hdr.Address {
		t.Errorf("address: got %q, want %q", decoded.Address, hdr.Address)
	}
	if decoded.DataIV != hdr.DataIV {
		t.Errorf("dataIV mismatch")
	}
	if decoded.DataKey != hdr.DataKey {
		t.Errorf("dataKey mismatch")
	}
}

func TestVMess_ParseUUID(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"01234567-89ab-cdef-0123-456789abcdef", true},
		{"0123456789abcdef0123456789abcdef", true},
		{"invalid", false},
		{"", false},
		{"01234567-89ab-cdef-0123-456789abcdeg", false}, // 'g' is invalid hex
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := vmess.ParseUUIDForTest(tt.input)
			if (err == nil) != tt.valid {
				t.Errorf("parseUUID(%q): err=%v, valid=%v", tt.input, err, tt.valid)
			}
		})
	}
}
