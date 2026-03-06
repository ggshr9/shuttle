package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// mockDialer returns a Dialer that hands back the client end of a net.Pipe.
// The server end is sent on the returned channel so the test can read/write it.
func mockDialer() (Dialer, <-chan net.Conn) {
	ch := make(chan net.Conn, 1)
	d := func(_ context.Context, _, _ string) (net.Conn, error) {
		server, client := net.Pipe()
		ch <- server
		return client, nil
	}
	return d, ch
}

// socks5Greet performs the SOCKS5 greeting (no-auth) on conn and returns the
// server's chosen auth method.
func socks5Greet(conn net.Conn) (byte, error) {
	// Send version + 1 method (no auth)
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return 0, err
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return 0, err
	}
	if resp[0] != 0x05 {
		return 0, fmt.Errorf("unexpected version %d", resp[0])
	}
	return resp[1], nil
}

// socks5GreetWithAuth performs the greeting offering password auth and then
// sends the username/password sub-negotiation.
func socks5GreetWithAuth(conn net.Conn, user, pass string) (authOK bool, err error) {
	// Greeting: offer password auth
	if _, err := conn.Write([]byte{0x05, 0x01, 0x02}); err != nil {
		return false, err
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return false, err
	}
	if resp[1] != 0x02 {
		return false, fmt.Errorf("server did not select password auth: %d", resp[1])
	}

	// Sub-negotiation (RFC 1929)
	buf := []byte{0x01, byte(len(user))}
	buf = append(buf, []byte(user)...)
	buf = append(buf, byte(len(pass)))
	buf = append(buf, []byte(pass)...)
	if _, err := conn.Write(buf); err != nil {
		return false, err
	}

	status := make([]byte, 2)
	if _, err := io.ReadFull(conn, status); err != nil {
		return false, err
	}
	return status[1] == 0x00, nil
}

// socks5ConnectIPv4 sends a CONNECT request for an IPv4 address.
func socks5ConnectIPv4(conn net.Conn, ip net.IP, port uint16) ([]byte, error) {
	req := []byte{0x05, 0x01, 0x00, 0x01}
	req = append(req, ip.To4()...)
	req = append(req, 0, 0)
	binary.BigEndian.PutUint16(req[len(req)-2:], port)
	if _, err := conn.Write(req); err != nil {
		return nil, err
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return nil, err
	}
	return reply, nil
}

// socks5ConnectDomain sends a CONNECT request for a domain name.
func socks5ConnectDomain(conn net.Conn, domain string, port uint16) ([]byte, error) {
	req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(domain))}
	req = append(req, []byte(domain)...)
	req = append(req, 0, 0)
	binary.BigEndian.PutUint16(req[len(req)-2:], port)
	if _, err := conn.Write(req); err != nil {
		return nil, err
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return nil, err
	}
	return reply, nil
}

// socks5ConnectIPv6 sends a CONNECT request for an IPv6 address.
func socks5ConnectIPv6(conn net.Conn, ip net.IP, port uint16) ([]byte, error) {
	req := []byte{0x05, 0x01, 0x00, 0x04}
	req = append(req, ip.To16()...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, port)
	req = append(req, portBytes...)
	if _, err := conn.Write(req); err != nil {
		return nil, err
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return nil, err
	}
	return reply, nil
}

// ---------------------------------------------------------------------------
// 1. SOCKS5 Handshake tests
// ---------------------------------------------------------------------------

func TestSOCKS5Handshake_NoAuth(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	dialer, _ := mockDialer()
	srv := NewSOCKS5Server(&SOCKS5Config{}, dialer, nil)

	go srv.handleConn(context.Background(), server)

	method, err := socks5Greet(client)
	if err != nil {
		t.Fatalf("greeting failed: %v", err)
	}
	if method != authNone {
		t.Fatalf("expected authNone (0x00), got 0x%02x", method)
	}
}

func TestSOCKS5Handshake_ConnectIPv4(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	var dialedAddr string
	dialer := func(_ context.Context, _, addr string) (net.Conn, error) {
		dialedAddr = addr
		s, c := net.Pipe()
		go func() { io.Copy(io.Discard, s) }()
		_ = c
		return s, nil
	}
	srv := NewSOCKS5Server(&SOCKS5Config{}, dialer, nil)

	go srv.handleConn(context.Background(), server)

	if _, err := socks5Greet(client); err != nil {
		t.Fatal(err)
	}

	reply, err := socks5ConnectIPv4(client, net.IPv4(93, 184, 216, 34), 443)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	if reply[1] != repSuccess {
		t.Fatalf("expected repSuccess, got 0x%02x", reply[1])
	}
	if dialedAddr != "93.184.216.34:443" {
		t.Fatalf("dialer received %q, want %q", dialedAddr, "93.184.216.34:443")
	}
}

func TestSOCKS5Handshake_ConnectDomain(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	var dialedAddr string
	dialer := func(_ context.Context, _, addr string) (net.Conn, error) {
		dialedAddr = addr
		s, c := net.Pipe()
		go func() { io.Copy(io.Discard, s) }()
		_ = c
		return s, nil
	}
	srv := NewSOCKS5Server(&SOCKS5Config{}, dialer, nil)

	go srv.handleConn(context.Background(), server)

	if _, err := socks5Greet(client); err != nil {
		t.Fatal(err)
	}

	reply, err := socks5ConnectDomain(client, "example.com", 80)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	if reply[1] != repSuccess {
		t.Fatalf("expected repSuccess, got 0x%02x", reply[1])
	}
	if dialedAddr != "example.com:80" {
		t.Fatalf("dialer received %q, want %q", dialedAddr, "example.com:80")
	}
}

func TestSOCKS5Handshake_ConnectIPv6(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	var dialedAddr string
	dialer := func(_ context.Context, _, addr string) (net.Conn, error) {
		dialedAddr = addr
		s, c := net.Pipe()
		go func() { io.Copy(io.Discard, s) }()
		_ = c
		return s, nil
	}
	srv := NewSOCKS5Server(&SOCKS5Config{}, dialer, nil)

	go srv.handleConn(context.Background(), server)

	if _, err := socks5Greet(client); err != nil {
		t.Fatal(err)
	}

	ipv6 := net.ParseIP("2001:db8::1")
	reply, err := socks5ConnectIPv6(client, ipv6, 8080)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	if reply[1] != repSuccess {
		t.Fatalf("expected repSuccess, got 0x%02x", reply[1])
	}
	if dialedAddr != "2001:db8::1:8080" {
		t.Fatalf("dialer received %q, want %q", dialedAddr, "2001:db8::1:8080")
	}
}

// ---------------------------------------------------------------------------
// 2. SOCKS5 Password Auth tests
// ---------------------------------------------------------------------------

func TestSOCKS5PasswordAuth_Success(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	dialer, _ := mockDialer()
	srv := NewSOCKS5Server(&SOCKS5Config{
		Username: "alice",
		Password: "s3cret",
	}, dialer, nil)

	go srv.handleConn(context.Background(), server)

	ok, err := socks5GreetWithAuth(client, "alice", "s3cret")
	if err != nil {
		t.Fatalf("auth handshake error: %v", err)
	}
	if !ok {
		t.Fatal("expected auth success, got failure")
	}
}

func TestSOCKS5PasswordAuth_WrongPassword(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	dialer, _ := mockDialer()
	srv := NewSOCKS5Server(&SOCKS5Config{
		Username: "alice",
		Password: "s3cret",
	}, dialer, nil)

	go srv.handleConn(context.Background(), server)

	ok, err := socks5GreetWithAuth(client, "alice", "wrong")
	if err != nil {
		t.Fatalf("auth handshake error: %v", err)
	}
	if ok {
		t.Fatal("expected auth failure, got success")
	}
}

// ---------------------------------------------------------------------------
// 3. SOCKS5 Full Integration test
// ---------------------------------------------------------------------------

func TestSOCKS5Integration_DataRelay(t *testing.T) {
	dialer, remoteCh := mockDialer()
	srv := NewSOCKS5Server(&SOCKS5Config{
		ListenAddr: "127.0.0.1:0",
	}, dialer, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer srv.Close()

	addr := srv.listener.Addr().String()

	// Connect via net.Dial
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial socks5 server: %v", err)
	}
	defer conn.Close()

	// SOCKS5 greeting
	if _, err := socks5Greet(conn); err != nil {
		t.Fatal(err)
	}

	// CONNECT to a fake target
	reply, err := socks5ConnectDomain(conn, "target.example.com", 9999)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if reply[1] != repSuccess {
		t.Fatalf("expected repSuccess, got 0x%02x", reply[1])
	}

	// Get the "remote" end that the dialer handed out
	var remote net.Conn
	select {
	case remote = <-remoteCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dialer to be called")
	}
	defer remote.Close()

	// Send data client -> remote
	payload := []byte("hello from client")
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, 256)
	n, err := remote.Read(buf)
	if err != nil {
		t.Fatalf("read from remote: %v", err)
	}
	if !bytes.Equal(buf[:n], payload) {
		t.Fatalf("got %q, want %q", buf[:n], payload)
	}

	// Send data remote -> client
	response := []byte("hello from remote")
	if _, err := remote.Write(response); err != nil {
		t.Fatalf("write: %v", err)
	}

	n, err = conn.Read(buf)
	if err != nil {
		t.Fatalf("read from client: %v", err)
	}
	if !bytes.Equal(buf[:n], response) {
		t.Fatalf("got %q, want %q", buf[:n], response)
	}
}

// ---------------------------------------------------------------------------
// 4. HTTP CONNECT proxy test
// ---------------------------------------------------------------------------

func TestHTTPConnect_DataRelay(t *testing.T) {
	dialer, remoteCh := mockDialer()
	srv := NewHTTPServer(&HTTPConfig{
		ListenAddr: "127.0.0.1:0",
	}, dialer, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer srv.Close()

	addr := srv.listener.Addr().String()

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial http proxy: %v", err)
	}
	defer conn.Close()

	// Send CONNECT request
	connectReq := "CONNECT target.example.com:443 HTTP/1.1\r\nHost: target.example.com:443\r\n\r\n"
	if _, err := conn.Write([]byte(connectReq)); err != nil {
		t.Fatal(err)
	}

	// Read response line
	br := bufio.NewReader(conn)
	line, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if line != "HTTP/1.1 200 Connection Established\r\n" {
		t.Fatalf("unexpected status line: %q", line)
	}
	// Read blank line after headers
	blank, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read blank line: %v", err)
	}
	if blank != "\r\n" {
		t.Fatalf("expected blank line, got %q", blank)
	}

	// Get the remote side
	var remote net.Conn
	select {
	case remote = <-remoteCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dialer")
	}
	defer remote.Close()

	// client -> remote
	payload := []byte("TLS client hello fake")
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 256)
	n, err := remote.Read(buf)
	if err != nil {
		t.Fatalf("remote read: %v", err)
	}
	if !bytes.Equal(buf[:n], payload) {
		t.Fatalf("got %q, want %q", buf[:n], payload)
	}

	// remote -> client
	response := []byte("TLS server hello fake")
	if _, err := remote.Write(response); err != nil {
		t.Fatal(err)
	}

	n, err = br.Read(buf)
	if err != nil {
		t.Fatalf("client read: %v", err)
	}
	if !bytes.Equal(buf[:n], response) {
		t.Fatalf("got %q, want %q", buf[:n], response)
	}
}

// ---------------------------------------------------------------------------
// 5. sendReply encoding test
// ---------------------------------------------------------------------------

func TestSendReply_Encoding(t *testing.T) {
	tests := []struct {
		name     string
		rep      byte
		addr     net.Addr
		expected []byte
	}{
		{
			name:     "success with nil addr",
			rep:      repSuccess,
			addr:     nil,
			expected: []byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0},
		},
		{
			name: "success with IPv4 addr",
			rep:  repSuccess,
			addr: &net.TCPAddr{
				IP:   net.IPv4(127, 0, 0, 1),
				Port: 12345,
			},
			expected: []byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0x30, 0x39},
		},
		{
			name:     "host unreachable with nil addr",
			rep:      repHostUnreach,
			addr:     nil,
			expected: []byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0},
		},
		{
			name: "general failure with addr",
			rep:  repGeneralFailure,
			addr: &net.TCPAddr{
				IP:   net.IPv4(10, 0, 0, 1),
				Port: 80,
			},
			expected: []byte{0x05, 0x01, 0x00, 0x01, 10, 0, 0, 1, 0x00, 0x50},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			defer server.Close()

			dialer, _ := mockDialer()
			srv := NewSOCKS5Server(&SOCKS5Config{}, dialer, nil)

			go func() {
				srv.sendReply(server, tc.rep, tc.addr)
				server.Close()
			}()

			var buf bytes.Buffer
			io.Copy(&buf, client)
			got := buf.Bytes()

			if !bytes.Equal(got, tc.expected) {
				t.Fatalf("sendReply bytes:\n  got  %v\n  want %v", got, tc.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 6. ProcessContext round-trip
// ---------------------------------------------------------------------------

func TestProcessContext_RoundTrip(t *testing.T) {
	ctx := context.Background()

	// Empty context returns empty string.
	if got := ProcessFromContext(ctx); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}

	// Set and retrieve.
	ctx = WithProcess(ctx, "firefox")
	if got := ProcessFromContext(ctx); got != "firefox" {
		t.Fatalf("expected %q, got %q", "firefox", got)
	}

	// Overwrite.
	ctx = WithProcess(ctx, "curl")
	if got := ProcessFromContext(ctx); got != "curl" {
		t.Fatalf("expected %q, got %q", "curl", got)
	}
}
