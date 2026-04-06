# Phase 2: Shadowsocks + VLESS + Trojan Protocol Support

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Shadowsocks, VLESS, and Trojan protocol support — both client (Dialer) and server (InboundHandler) — making Shuttle compatible with 80% of existing proxy server deployments.

**Architecture:** Each protocol implements `adapter.Dialer` (client) and `adapter.InboundHandler` (server), registers via `adapter.Register()` in `init()`. URI parsers register in `subscription.ParseURI()` for subscription compatibility. All three protocols share a common TLS infrastructure for external TLS/Reality wrapping.

**Tech Stack:** Go 1.24+, `github.com/shadowsocks/go-shadowsocks2` (SS AEAD), `github.com/sagernet/sing-shadowsocks2` (SS 2022)

**Spec:** `docs/superpowers/specs/2026-04-05-ecosystem-compatibility-design.md` — Section 5.1-5.3, 5.7-5.8

**Depends on:** Phase 1 (adapter/dialer.go, adapter/bridge.go)

---

## File Structure

### New Files
| File | Responsibility |
|------|---------------|
| `transport/shadowsocks/factory.go` | SS factory registration |
| `transport/shadowsocks/dialer.go` | SS client Dialer (TCP) |
| `transport/shadowsocks/server.go` | SS server InboundHandler |
| `transport/shadowsocks/cipher.go` | Cipher initialization (AEAD + 2022) |
| `transport/shadowsocks/udp.go` | SS UDP relay |
| `transport/shadowsocks/dialer_test.go` | SS client tests |
| `transport/shadowsocks/server_test.go` | SS server tests |
| `transport/vless/factory.go` | VLESS factory registration |
| `transport/vless/dialer.go` | VLESS client Dialer |
| `transport/vless/server.go` | VLESS server InboundHandler |
| `transport/vless/protocol.go` | VLESS header encoding/decoding |
| `transport/vless/dialer_test.go` | VLESS client tests |
| `transport/vless/server_test.go` | VLESS server tests |
| `transport/trojan/factory.go` | Trojan factory registration |
| `transport/trojan/dialer.go` | Trojan client Dialer |
| `transport/trojan/server.go` | Trojan server InboundHandler |
| `transport/trojan/protocol.go` | Trojan header encoding/decoding |
| `transport/trojan/dialer_test.go` | Trojan client tests |
| `transport/trojan/server_test.go` | Trojan server tests |
| `transport/shared/tls_config.go` | Shared TLS configuration builder |
| `transport/shared/addr.go` | SOCKS5-style address encoding/decoding |
| `transport/shared/addr_test.go` | Address codec tests |
| `subscription/parser_uri.go` | Unified URI parser (ss://, vless://, trojan://) |
| `subscription/parser_uri_test.go` | URI parser tests |
| `test/e2e/protocol_test.go` | E2E tests: SS/VLESS/Trojan through full proxy pipeline (`//go:build sandbox`) |

### Modified Files
| File | Change |
|------|--------|
| `config/config.go` | Add SS/VLESS/Trojan outbound and inbound config types |
| `subscription/subscription.go` | Register URI parsers |
| `subscription/parser_clash.go` | Map Clash ss/vmess/vless/trojan types to new OutboundOptions |
| `server/server.go` | Start InboundHandler listeners alongside existing transports |
| `go.mod` | Add shadowsocks dependencies |

---

### Task 1: Shared Address Codec

**Files:**
- Create: `transport/shared/addr.go`
- Create: `transport/shared/addr_test.go`

SS, VLESS, and Trojan all use SOCKS5-style address encoding. Build once, reuse three times.

- [ ] **Step 1: Write test for address codec**

```go
// transport/shared/addr_test.go
package shared_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"shuttle/transport/shared"
)

func TestEncodeDecodeAddr_Domain(t *testing.T) {
	var buf bytes.Buffer
	err := shared.EncodeAddr(&buf, "tcp", "example.com:443")
	require.NoError(t, err)

	network, addr, err := shared.DecodeAddr(&buf)
	require.NoError(t, err)
	assert.Equal(t, "tcp", network)
	assert.Equal(t, "example.com:443", addr)
}

func TestEncodeDecodeAddr_IPv4(t *testing.T) {
	var buf bytes.Buffer
	err := shared.EncodeAddr(&buf, "tcp", "1.2.3.4:80")
	require.NoError(t, err)

	network, addr, err := shared.DecodeAddr(&buf)
	require.NoError(t, err)
	assert.Equal(t, "tcp", network)
	assert.Equal(t, "1.2.3.4:80", addr)
}

func TestEncodeDecodeAddr_IPv6(t *testing.T) {
	var buf bytes.Buffer
	err := shared.EncodeAddr(&buf, "tcp", "[::1]:8080")
	require.NoError(t, err)

	network, addr, err := shared.DecodeAddr(&buf)
	require.NoError(t, err)
	assert.Equal(t, "tcp", network)
	assert.Equal(t, "[::1]:8080", addr)
}

func TestEncodeDecodeAddr_UDP(t *testing.T) {
	var buf bytes.Buffer
	err := shared.EncodeAddr(&buf, "udp", "example.com:53")
	require.NoError(t, err)

	network, addr, err := shared.DecodeAddr(&buf)
	require.NoError(t, err)
	assert.Equal(t, "udp", network)
	assert.Equal(t, "example.com:53", addr)
}
```

- [ ] **Step 2: Implement address codec**

```go
// transport/shared/addr.go
package shared

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
)

// SOCKS5 address types
const (
	AddrTypeIPv4   = 0x01
	AddrTypeDomain = 0x03
	AddrTypeIPv6   = 0x04
)

// Network command types (used by Trojan)
const (
	CmdConnect      = 0x01
	CmdUDPAssociate = 0x03
)

// EncodeAddr writes a SOCKS5-style address to w.
// Format: [atype(1)][addr][port(2)]
func EncodeAddr(w io.Writer, network, address string) error {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid address %q: %w", address, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port %q: %w", portStr, err)
	}

	ip := net.ParseIP(host)
	var buf []byte

	switch {
	case ip == nil:
		// Domain
		if len(host) > 255 {
			return fmt.Errorf("domain too long: %d", len(host))
		}
		buf = make([]byte, 1+1+len(host)+2)
		buf[0] = AddrTypeDomain
		buf[1] = byte(len(host))
		copy(buf[2:], host)
		binary.BigEndian.PutUint16(buf[2+len(host):], uint16(port))

	case ip.To4() != nil:
		buf = make([]byte, 1+4+2)
		buf[0] = AddrTypeIPv4
		copy(buf[1:], ip.To4())
		binary.BigEndian.PutUint16(buf[5:], uint16(port))

	default:
		buf = make([]byte, 1+16+2)
		buf[0] = AddrTypeIPv6
		copy(buf[1:], ip.To16())
		binary.BigEndian.PutUint16(buf[17:], uint16(port))
	}

	_, err = w.Write(buf)
	return err
}

// DecodeAddr reads a SOCKS5-style address from r.
// Returns network ("tcp"/"udp") and "host:port".
func DecodeAddr(r io.Reader) (network string, address string, err error) {
	network = "tcp" // default, caller overrides for UDP

	var atype [1]byte
	if _, err = io.ReadFull(r, atype[:]); err != nil {
		return
	}

	var host string
	switch atype[0] {
	case AddrTypeIPv4:
		var ip [4]byte
		if _, err = io.ReadFull(r, ip[:]); err != nil {
			return
		}
		host = net.IP(ip[:]).String()

	case AddrTypeDomain:
		var length [1]byte
		if _, err = io.ReadFull(r, length[:]); err != nil {
			return
		}
		domain := make([]byte, length[0])
		if _, err = io.ReadFull(r, domain); err != nil {
			return
		}
		host = string(domain)

	case AddrTypeIPv6:
		var ip [16]byte
		if _, err = io.ReadFull(r, ip[:]); err != nil {
			return
		}
		host = "[" + net.IP(ip[:]).String() + "]"

	default:
		err = fmt.Errorf("unknown address type: 0x%02x", atype[0])
		return
	}

	var portBuf [2]byte
	if _, err = io.ReadFull(r, portBuf[:]); err != nil {
		return
	}
	port := binary.BigEndian.Uint16(portBuf[:])
	address = net.JoinHostPort(host, strconv.Itoa(int(port)))
	return
}
```

- [ ] **Step 3: Run tests**

Run: `./scripts/test.sh --pkg ./transport/shared/`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add transport/shared/
git commit -m "feat(transport): add shared SOCKS5-style address codec"
```

---

### Task 2: Shared TLS Configuration

**Files:**
- Create: `transport/shared/tls_config.go`

- [ ] **Step 1: Implement shared TLS config builder**

VLESS and Trojan both need TLS wrapping. Build a shared helper.

```go
// transport/shared/tls_config.go
package shared

import (
	"crypto/tls"
	"fmt"
)

// TLSOptions configures client-side TLS.
type TLSOptions struct {
	Enabled            bool   `json:"enabled" yaml:"enabled"`
	ServerName         string `json:"server_name" yaml:"server_name"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify,omitempty" yaml:"insecure_skip_verify,omitempty"`
	ALPN               []string `json:"alpn,omitempty" yaml:"alpn,omitempty"`
}

// ServerTLSOptions configures server-side TLS.
type ServerTLSOptions struct {
	CertFile string   `json:"cert_file" yaml:"cert_file"`
	KeyFile  string   `json:"key_file" yaml:"key_file"`
	ALPN     []string `json:"alpn,omitempty" yaml:"alpn,omitempty"`
}

// BuildClientTLS creates a *tls.Config from TLSOptions.
func BuildClientTLS(opts TLSOptions) (*tls.Config, error) {
	if !opts.Enabled {
		return nil, nil
	}
	cfg := &tls.Config{
		ServerName:         opts.ServerName,
		InsecureSkipVerify: opts.InsecureSkipVerify,
	}
	if len(opts.ALPN) > 0 {
		cfg.NextProtos = opts.ALPN
	}
	return cfg, nil
}

// BuildServerTLS creates a *tls.Config from ServerTLSOptions.
func BuildServerTLS(opts ServerTLSOptions) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(opts.CertFile, opts.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load TLS cert: %w", err)
	}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	if len(opts.ALPN) > 0 {
		cfg.NextProtos = opts.ALPN
	}
	return cfg, nil
}
```

- [ ] **Step 2: Commit**

```bash
git add transport/shared/tls_config.go
git commit -m "feat(transport): add shared TLS configuration builder"
```

---

### Task 3: Shadowsocks Cipher Layer

**Files:**
- Create: `transport/shadowsocks/cipher.go`
- Modify: `go.mod`

- [ ] **Step 1: Add dependencies**

```bash
go get github.com/shadowsocks/go-shadowsocks2@latest
go get github.com/sagernet/sing-shadowsocks2@latest
```

- [ ] **Step 2: Implement cipher initialization**

```go
// transport/shadowsocks/cipher.go
package shadowsocks

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"io"
	"net"

	shadowaead "github.com/shadowsocks/go-shadowsocks2/shadowaead"
	core "github.com/shadowsocks/go-shadowsocks2/core"
)

// SupportedMethods lists all supported encryption methods.
var SupportedMethods = []string{
	"aes-128-gcm",
	"aes-256-gcm",
	"chacha20-ietf-poly1305",
	// SS 2022 methods handled separately
}

// NewCipher creates a shadowsocks cipher from method and password.
func NewCipher(method, password string) (core.Cipher, error) {
	switch method {
	case "aes-128-gcm", "aes-256-gcm", "chacha20-ietf-poly1305":
		return core.PickCipher(method, nil, password)
	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}
}

// WrapConn wraps a net.Conn with shadowsocks encryption.
func WrapConn(conn net.Conn, ciph core.Cipher) net.Conn {
	return ciph.StreamConn(conn)
}
```

Note: SS 2022 support will be added as a follow-up since sing-shadowsocks2 has a different API. The classic AEAD methods cover the immediate need.

- [ ] **Step 3: Commit**

```bash
git add transport/shadowsocks/cipher.go go.mod go.sum
git commit -m "feat(shadowsocks): add cipher initialization for AEAD methods"
```

---

### Task 4: Shadowsocks Client Dialer

**Files:**
- Create: `transport/shadowsocks/dialer.go`
- Create: `transport/shadowsocks/dialer_test.go`

- [ ] **Step 1: Write test — SS client dials through SS server**

```go
// transport/shadowsocks/dialer_test.go
package shadowsocks_test

import (
	"context"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"shuttle/transport/shadowsocks"
)

func TestDialer_EchoThroughServer(t *testing.T) {
	// 1. Start a TCP echo backend
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer echoLn.Close()
	go func() {
		for {
			c, err := echoLn.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(c, c); c.Close() }()
		}
	}()

	// 2. Start SS server
	ssLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ssLn.Close()

	srv, err := shadowsocks.NewServer(shadowsocks.ServerConfig{
		Method:   "aes-256-gcm",
		Password: "test-password",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Serve(ctx, ssLn, func(ctx context.Context, conn net.Conn, meta shadowsocks.ConnMeta) {
		// Relay to echo backend
		backend, err := net.Dial("tcp", meta.Destination)
		if err != nil {
			conn.Close()
			return
		}
		go func() { io.Copy(backend, conn); backend.Close() }()
		io.Copy(conn, backend)
		conn.Close()
	})

	// 3. SS client dials through SS server to echo backend
	dialer, err := shadowsocks.NewDialer(shadowsocks.DialerConfig{
		Server:   ssLn.Addr().String(),
		Method:   "aes-256-gcm",
		Password: "test-password",
	})
	require.NoError(t, err)

	conn, err := dialer.DialContext(context.Background(), "tcp", echoLn.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte("hello shadowsocks"))
	require.NoError(t, err)

	buf := make([]byte, 17)
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	assert.Equal(t, "hello shadowsocks", string(buf))
}
```

- [ ] **Step 2: Implement SS Dialer**

```go
// transport/shadowsocks/dialer.go
package shadowsocks

import (
	"bytes"
	"context"
	"net"

	core "github.com/shadowsocks/go-shadowsocks2/core"

	"shuttle/transport/shared"
)

// DialerConfig configures the SS client.
type DialerConfig struct {
	Server   string // host:port
	Method   string
	Password string
}

// Dialer implements adapter.Dialer for Shadowsocks.
type Dialer struct {
	server string
	cipher core.Cipher
}

// NewDialer creates a new SS Dialer.
func NewDialer(cfg DialerConfig) (*Dialer, error) {
	ciph, err := NewCipher(cfg.Method, cfg.Password)
	if err != nil {
		return nil, err
	}
	return &Dialer{
		server: cfg.Server,
		cipher: ciph,
	}, nil
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// 1. TCP connect to SS server
	var dialer net.Dialer
	rawConn, err := dialer.DialContext(ctx, "tcp", d.server)
	if err != nil {
		return nil, err
	}

	// 2. Wrap with AEAD encryption
	ssConn := WrapConn(rawConn, d.cipher)

	// 3. Write target address header
	var addrBuf bytes.Buffer
	if err := shared.EncodeAddr(&addrBuf, network, address); err != nil {
		ssConn.Close()
		return nil, err
	}
	if _, err := ssConn.Write(addrBuf.Bytes()); err != nil {
		ssConn.Close()
		return nil, err
	}

	return ssConn, nil
}

func (d *Dialer) Type() string { return "shadowsocks" }
func (d *Dialer) Close() error { return nil }
```

- [ ] **Step 3: Run test**

Run: `./scripts/test.sh --pkg ./transport/shadowsocks/ --run TestDialer`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add transport/shadowsocks/dialer.go transport/shadowsocks/dialer_test.go
git commit -m "feat(shadowsocks): implement client Dialer with AEAD encryption"
```

---

### Task 5: Shadowsocks Server

**Files:**
- Create: `transport/shadowsocks/server.go`
- Create: `transport/shadowsocks/server_test.go`

- [ ] **Step 1: Implement SS server**

```go
// transport/shadowsocks/server.go
package shadowsocks

import (
	"context"
	"net"

	core "github.com/shadowsocks/go-shadowsocks2/core"

	"shuttle/transport/shared"
)

// ConnMeta contains metadata about an incoming SS connection.
type ConnMeta struct {
	Network     string
	Destination string
	Source      string
}

// ConnHandler is called when a new proxied connection arrives.
type ConnHandler func(ctx context.Context, conn net.Conn, meta ConnMeta)

// ServerConfig configures the SS server.
type ServerConfig struct {
	Method   string
	Password string
}

// Server implements an SS inbound handler.
type Server struct {
	cipher core.Cipher
}

// NewServer creates a new SS server.
func NewServer(cfg ServerConfig) (*Server, error) {
	ciph, err := NewCipher(cfg.Method, cfg.Password)
	if err != nil {
		return nil, err
	}
	return &Server{cipher: ciph}, nil
}

// Serve accepts connections from the listener and processes them.
func (s *Server) Serve(ctx context.Context, ln net.Listener, handler ConnHandler) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				continue
			}
		}
		go s.handleConn(ctx, conn, handler)
	}
}

func (s *Server) handleConn(ctx context.Context, rawConn net.Conn, handler ConnHandler) {
	// Wrap with AEAD decryption
	ssConn := WrapConn(rawConn, s.cipher)

	// Read target address
	_, addr, err := shared.DecodeAddr(ssConn)
	if err != nil {
		ssConn.Close()
		return
	}

	meta := ConnMeta{
		Network:     "tcp",
		Destination: addr,
		Source:      rawConn.RemoteAddr().String(),
	}

	handler(ctx, ssConn, meta)
}

func (s *Server) Type() string { return "shadowsocks" }
func (s *Server) Close() error { return nil }
```

- [ ] **Step 2: Run full SS test**

Run: `./scripts/test.sh --pkg ./transport/shadowsocks/`
Expected: PASS (TestDialer_EchoThroughServer exercises both client and server)

- [ ] **Step 3: Commit**

```bash
git add transport/shadowsocks/server.go
git commit -m "feat(shadowsocks): implement server InboundHandler"
```

---

### Task 6: Shadowsocks Factory Registration

**Files:**
- Create: `transport/shadowsocks/factory.go`

- [ ] **Step 1: Implement factory**

```go
// transport/shadowsocks/factory.go
package shadowsocks

import (
	"shuttle/adapter"
)

func init() {
	adapter.Register(&factory{})
}

type factory struct{}

func (f *factory) Type() string { return "shadowsocks" }

// Shadowsocks doesn't use multiplexed ClientTransport/ServerTransport.
func (f *factory) NewClient(cfg *config.ClientConfig, opts adapter.FactoryOptions) (adapter.ClientTransport, error) {
	return nil, nil
}
func (f *factory) NewServer(cfg *config.ServerConfig, opts adapter.FactoryOptions) (adapter.ServerTransport, error) {
	return nil, nil
}

// Per-request protocol — uses Dialer/InboundHandler
func (f *factory) NewDialer(cfg map[string]any, opts adapter.FactoryOptions) (adapter.Dialer, error) {
	method, _ := cfg["method"].(string)
	password, _ := cfg["password"].(string)
	server, _ := cfg["server"].(string)
	return NewDialer(DialerConfig{
		Server:   server,
		Method:   method,
		Password: password,
	})
}

func (f *factory) NewInboundHandler(cfg map[string]any, opts adapter.FactoryOptions) (adapter.InboundHandler, error) {
	method, _ := cfg["method"].(string)
	password, _ := cfg["password"].(string)
	srv, err := NewServer(ServerConfig{
		Method:   method,
		Password: password,
	})
	if err != nil {
		return nil, err
	}
	return &inboundAdapter{srv: srv}, nil
}

// inboundAdapter wraps Server to satisfy adapter.InboundHandler.
type inboundAdapter struct {
	srv *Server
}

func (a *inboundAdapter) Type() string { return "shadowsocks" }
func (a *inboundAdapter) Serve(ctx context.Context, ln net.Listener, handler adapter.ConnHandler) error {
	return a.srv.Serve(ctx, ln, func(ctx context.Context, conn net.Conn, meta ConnMeta) {
		handler(ctx, conn, adapter.ConnMetadata{
			Network:     meta.Network,
			Destination: meta.Destination,
			Source:      meta.Source,
		})
	})
}
func (a *inboundAdapter) Close() error { return a.srv.Close() }
```

Note: Add necessary imports (`context`, `net`, `shuttle/config`). The exact import of `config` depends on the final TransportFactory interface — if it uses `map[string]any` for the Dialer methods (as designed in Phase 1), the config import isn't needed for the Dialer factory methods.

- [ ] **Step 2: Commit**

```bash
git add transport/shadowsocks/factory.go
git commit -m "feat(shadowsocks): register factory for auto-discovery"
```

---

### Task 7: VLESS Protocol Implementation

**Files:**
- Create: `transport/vless/protocol.go`
- Create: `transport/vless/dialer.go`
- Create: `transport/vless/server.go`
- Create: `transport/vless/factory.go`
- Create: `transport/vless/dialer_test.go`

- [ ] **Step 1: Implement VLESS protocol codec**

```go
// transport/vless/protocol.go
package vless

import (
	"encoding/binary"
	"fmt"
	"io"

	"shuttle/transport/shared"
)

const (
	Version = 0

	CmdTCP = 0x01
	CmdUDP = 0x02
)

// RequestHeader is the VLESS client request header.
type RequestHeader struct {
	Version byte
	UUID    [16]byte
	Command byte
	Address string // host:port
}

// EncodeRequest writes the VLESS request header.
// Format: [version(1)][uuid(16)][addon_len(1)][addon(0)][cmd(1)][addr]
func EncodeRequest(w io.Writer, h *RequestHeader) error {
	buf := make([]byte, 1+16+1+1) // version + uuid + addon_len(0) + cmd
	buf[0] = h.Version
	copy(buf[1:17], h.UUID[:])
	buf[17] = 0 // no addons
	buf[18] = h.Command
	if _, err := w.Write(buf); err != nil {
		return err
	}

	network := "tcp"
	if h.Command == CmdUDP {
		network = "udp"
	}
	return shared.EncodeAddr(w, network, h.Address)
}

// DecodeRequest reads the VLESS request header.
func DecodeRequest(r io.Reader) (*RequestHeader, error) {
	var header [1 + 16]byte // version + uuid
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	h := &RequestHeader{
		Version: header[0],
	}
	copy(h.UUID[:], header[1:17])

	// Read addon length
	var addonLen [1]byte
	if _, err := io.ReadFull(r, addonLen[:]); err != nil {
		return nil, err
	}
	if addonLen[0] > 0 {
		addon := make([]byte, addonLen[0])
		if _, err := io.ReadFull(r, addon); err != nil {
			return nil, err
		}
		// Addons ignored for now
	}

	// Read command
	var cmd [1]byte
	if _, err := io.ReadFull(r, cmd[:]); err != nil {
		return nil, err
	}
	h.Command = cmd[0]

	// Read address
	_, addr, err := shared.DecodeAddr(r)
	if err != nil {
		return nil, err
	}
	h.Address = addr

	return h, nil
}

// ResponseHeader is the VLESS server response.
type ResponseHeader struct {
	Version  byte
	AddonLen byte
}

// EncodeResponse writes the VLESS response header.
func EncodeResponse(w io.Writer) error {
	_, err := w.Write([]byte{Version, 0}) // version + no addons
	return err
}

// DecodeResponse reads the VLESS response header.
func DecodeResponse(r io.Reader) error {
	var header [2]byte
	_, err := io.ReadFull(r, header[:])
	return err
}
```

- [ ] **Step 2: Implement VLESS Dialer**

```go
// transport/vless/dialer.go
package vless

import (
	"context"
	"crypto/tls"
	"net"

	"shuttle/transport/shared"
)

// DialerConfig configures the VLESS client.
type DialerConfig struct {
	Server string
	UUID   [16]byte
	TLS    shared.TLSOptions
}

// Dialer implements adapter.Dialer for VLESS.
type Dialer struct {
	server string
	uuid   [16]byte
	tls    *tls.Config
}

// NewDialer creates a new VLESS Dialer.
func NewDialer(cfg DialerConfig) (*Dialer, error) {
	tlsCfg, err := shared.BuildClientTLS(cfg.TLS)
	if err != nil {
		return nil, err
	}
	return &Dialer{
		server: cfg.Server,
		uuid:   cfg.UUID,
		tls:    tlsCfg,
	}, nil
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// 1. TCP connect
	var dialer net.Dialer
	rawConn, err := dialer.DialContext(ctx, "tcp", d.server)
	if err != nil {
		return nil, err
	}

	// 2. TLS handshake
	var conn net.Conn = rawConn
	if d.tls != nil {
		tlsConn := tls.Client(rawConn, d.tls)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			rawConn.Close()
			return nil, err
		}
		conn = tlsConn
	}

	// 3. Send VLESS request
	cmd := CmdTCP
	if network == "udp" {
		cmd = CmdUDP
	}
	if err := EncodeRequest(conn, &RequestHeader{
		Version: Version,
		UUID:    d.uuid,
		Command: byte(cmd),
		Address: address,
	}); err != nil {
		conn.Close()
		return nil, err
	}

	// 4. Read response header
	if err := DecodeResponse(conn); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

func (d *Dialer) Type() string { return "vless" }
func (d *Dialer) Close() error { return nil }
```

- [ ] **Step 3: Implement VLESS Server**

```go
// transport/vless/server.go
package vless

import (
	"context"
	"fmt"
	"net"

	"shuttle/adapter"
)

// ServerConfig configures the VLESS server.
type ServerConfig struct {
	Users map[[16]byte]string // uuid → user tag
}

// Server implements VLESS inbound handling.
type Server struct {
	users map[[16]byte]string
}

// NewServer creates a new VLESS server.
func NewServer(cfg ServerConfig) (*Server, error) {
	if len(cfg.Users) == 0 {
		return nil, fmt.Errorf("at least one user is required")
	}
	return &Server{users: cfg.Users}, nil
}

// Serve accepts and handles VLESS connections.
// The listener should already be TLS-wrapped if TLS is desired.
func (s *Server) Serve(ctx context.Context, ln net.Listener, handler adapter.ConnHandler) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				continue
			}
		}
		go s.handleConn(ctx, conn, handler)
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn, handler adapter.ConnHandler) {
	req, err := DecodeRequest(conn)
	if err != nil {
		conn.Close()
		return
	}

	// Validate UUID
	if _, ok := s.users[req.UUID]; !ok {
		conn.Close()
		return
	}

	// Send response
	if err := EncodeResponse(conn); err != nil {
		conn.Close()
		return
	}

	network := "tcp"
	if req.Command == CmdUDP {
		network = "udp"
	}

	handler(ctx, conn, adapter.ConnMetadata{
		Network:     network,
		Destination: req.Address,
		Source:      conn.RemoteAddr().String(),
	})
}

func (s *Server) Type() string { return "vless" }
func (s *Server) Close() error { return nil }
```

- [ ] **Step 4: Write VLESS test (client through server, no TLS for unit test)**

```go
// transport/vless/dialer_test.go
package vless_test

import (
	"context"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"shuttle/adapter"
	"shuttle/transport/shared"
	"shuttle/transport/vless"
)

func TestVLESS_EchoThroughServer(t *testing.T) {
	testUUID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	// Echo backend
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer echoLn.Close()
	go func() {
		for {
			c, err := echoLn.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(c, c); c.Close() }()
		}
	}()

	// VLESS server (no TLS in unit test)
	vlessLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer vlessLn.Close()

	srv, err := vless.NewServer(vless.ServerConfig{
		Users: map[[16]byte]string{testUUID: "test-user"},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Serve(ctx, vlessLn, func(ctx context.Context, conn net.Conn, meta adapter.ConnMetadata) {
		backend, err := net.Dial("tcp", meta.Destination)
		if err != nil {
			conn.Close()
			return
		}
		go func() { io.Copy(backend, conn); backend.Close() }()
		io.Copy(conn, backend)
		conn.Close()
	})

	// VLESS client (no TLS)
	dialer, err := vless.NewDialer(vless.DialerConfig{
		Server: vlessLn.Addr().String(),
		UUID:   testUUID,
		TLS:    shared.TLSOptions{Enabled: false},
	})
	require.NoError(t, err)

	conn, err := dialer.DialContext(context.Background(), "tcp", echoLn.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte("hello vless"))
	require.NoError(t, err)

	buf := make([]byte, 11)
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	assert.Equal(t, "hello vless", string(buf))
}
```

- [ ] **Step 5: Run tests**

Run: `./scripts/test.sh --pkg ./transport/vless/`
Expected: PASS

- [ ] **Step 6: Implement VLESS factory and commit**

Create `transport/vless/factory.go` following the same pattern as Task 6 (SS factory).

```bash
git add transport/vless/
git commit -m "feat(vless): implement VLESS client Dialer and server with UUID auth"
```

---

### Task 8: Trojan Protocol Implementation

**Files:**
- Create: `transport/trojan/protocol.go`
- Create: `transport/trojan/dialer.go`
- Create: `transport/trojan/server.go`
- Create: `transport/trojan/factory.go`
- Create: `transport/trojan/dialer_test.go`

- [ ] **Step 1: Implement Trojan protocol codec**

```go
// transport/trojan/protocol.go
package trojan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"shuttle/transport/shared"
)

// HashPassword returns the SHA224 hex digest of a password (Trojan spec).
func HashPassword(password string) string {
	h := sha256.New224()
	h.Write([]byte(password))
	return hex.EncodeToString(h.Sum(nil))
}

// EncodeRequest writes the Trojan client request.
// Format: SHA224(password) + CRLF + cmd(1) + addr + CRLF
func EncodeRequest(w io.Writer, passwordHash string, cmd byte, address string) error {
	// Password hash (56 hex chars) + CRLF
	header := passwordHash + "\r\n"
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}

	// Command
	if _, err := w.Write([]byte{cmd}); err != nil {
		return err
	}

	// Address (SOCKS5 format)
	if err := shared.EncodeAddr(w, "tcp", address); err != nil {
		return err
	}

	// Trailing CRLF
	_, err := io.WriteString(w, "\r\n")
	return err
}

// DecodeRequest reads the Trojan request from a connection.
// Returns the password hash, command, and target address.
func DecodeRequest(r io.Reader) (hash string, cmd byte, address string, err error) {
	// Read 56 hex chars + CRLF = 58 bytes
	var hashBuf [58]byte
	if _, err = io.ReadFull(r, hashBuf[:]); err != nil {
		return
	}
	hash = string(hashBuf[:56])
	// Verify CRLF
	if hashBuf[56] != '\r' || hashBuf[57] != '\n' {
		err = fmt.Errorf("invalid Trojan header: missing CRLF after hash")
		return
	}

	// Command
	var cmdBuf [1]byte
	if _, err = io.ReadFull(r, cmdBuf[:]); err != nil {
		return
	}
	cmd = cmdBuf[0]

	// Address
	_, address, err = shared.DecodeAddr(r)
	if err != nil {
		return
	}

	// Trailing CRLF
	var crlf [2]byte
	_, err = io.ReadFull(r, crlf[:])
	return
}
```

- [ ] **Step 2: Implement Trojan Dialer**

```go
// transport/trojan/dialer.go
package trojan

import (
	"context"
	"crypto/tls"
	"net"

	"shuttle/transport/shared"
)

// DialerConfig configures the Trojan client.
type DialerConfig struct {
	Server   string
	Password string
	TLS      shared.TLSOptions
}

// Dialer implements adapter.Dialer for Trojan.
type Dialer struct {
	server       string
	passwordHash string
	tls          *tls.Config
}

// NewDialer creates a new Trojan Dialer.
func NewDialer(cfg DialerConfig) (*Dialer, error) {
	tlsCfg, err := shared.BuildClientTLS(cfg.TLS)
	if err != nil {
		return nil, err
	}
	return &Dialer{
		server:       cfg.Server,
		passwordHash: HashPassword(cfg.Password),
		tls:          tlsCfg,
	}, nil
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var dialer net.Dialer
	rawConn, err := dialer.DialContext(ctx, "tcp", d.server)
	if err != nil {
		return nil, err
	}

	var conn net.Conn = rawConn
	if d.tls != nil {
		tlsConn := tls.Client(rawConn, d.tls)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			rawConn.Close()
			return nil, err
		}
		conn = tlsConn
	}

	cmd := byte(shared.CmdConnect)
	if network == "udp" {
		cmd = shared.CmdUDPAssociate
	}

	if err := EncodeRequest(conn, d.passwordHash, cmd, address); err != nil {
		conn.Close()
		return nil, err
	}

	// Trojan has no response header — data flows immediately
	return conn, nil
}

func (d *Dialer) Type() string { return "trojan" }
func (d *Dialer) Close() error { return nil }
```

- [ ] **Step 3: Implement Trojan Server with fallback**

```go
// transport/trojan/server.go
package trojan

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"

	"shuttle/adapter"
)

// ServerConfig configures the Trojan server.
type ServerConfig struct {
	Passwords map[string]string // SHA224 hash → user tag
	Fallback  net.Addr          // fallback address for non-Trojan connections
}

// Server implements Trojan inbound handling.
type Server struct {
	passwords map[string]string
	fallback  net.Addr
}

// NewServer creates a new Trojan server.
func NewServer(cfg ServerConfig) *Server {
	return &Server{
		passwords: cfg.Passwords,
		fallback:  cfg.Fallback,
	}
}

// Serve accepts and handles Trojan connections.
func (s *Server) Serve(ctx context.Context, ln net.Listener, handler adapter.ConnHandler) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				continue
			}
		}
		go s.handleConn(ctx, conn, handler)
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn, handler adapter.ConnHandler) {
	// Buffer the read so we can replay data to fallback if auth fails
	br := bufio.NewReader(conn)

	// Peek enough bytes for the password hash + CRLF
	peeked, err := br.Peek(58)
	if err != nil {
		s.fallbackOrClose(conn, br)
		return
	}

	hash := string(peeked[:56])
	if _, ok := s.passwords[hash]; !ok {
		// Auth failed — fallback to cover site
		s.fallbackOrClose(conn, br)
		return
	}

	// Auth passed — consume the peeked bytes and decode the full request
	prefixed := io.MultiReader(br, conn)
	hashStr, cmd, addr, err := DecodeRequest(prefixed)
	_ = hashStr
	if err != nil {
		conn.Close()
		return
	}

	network := "tcp"
	if cmd == shared.CmdUDPAssociate {
		network = "udp"
	}

	// Wrap remaining data as net.Conn
	remaining := &prefixConn{
		Reader: prefixed,
		Conn:   conn,
	}

	handler(ctx, remaining, adapter.ConnMetadata{
		Network:     network,
		Destination: addr,
		Source:      conn.RemoteAddr().String(),
	})
}

func (s *Server) fallbackOrClose(conn net.Conn, br *bufio.Reader) {
	if s.fallback != nil {
		// Relay to fallback (cover site)
		fb, err := net.Dial("tcp", s.fallback.String())
		if err != nil {
			conn.Close()
			return
		}
		go func() {
			io.Copy(fb, br) // replay buffered + remaining
			fb.Close()
		}()
		io.Copy(conn, fb)
		conn.Close()
		return
	}
	conn.Close()
}

func (s *Server) Type() string { return "trojan" }
func (s *Server) Close() error { return nil }

// prefixConn wraps a Reader + net.Conn.
type prefixConn struct {
	io.Reader
	net.Conn
}

func (c *prefixConn) Read(p []byte) (int, error) {
	return c.Reader.Read(p)
}
```

- [ ] **Step 4: Write Trojan test**

```go
// transport/trojan/dialer_test.go
package trojan_test

import (
	"context"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"shuttle/adapter"
	"shuttle/transport/shared"
	"shuttle/transport/trojan"
)

func TestTrojan_EchoThroughServer(t *testing.T) {
	password := "test-trojan-password"
	hash := trojan.HashPassword(password)

	// Echo backend
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer echoLn.Close()
	go func() {
		for {
			c, err := echoLn.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(c, c); c.Close() }()
		}
	}()

	// Trojan server (no TLS in unit test)
	trojanLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer trojanLn.Close()

	srv := trojan.NewServer(trojan.ServerConfig{
		Passwords: map[string]string{hash: "test-user"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Serve(ctx, trojanLn, func(ctx context.Context, conn net.Conn, meta adapter.ConnMetadata) {
		backend, err := net.Dial("tcp", meta.Destination)
		if err != nil {
			conn.Close()
			return
		}
		go func() { io.Copy(backend, conn); backend.Close() }()
		io.Copy(conn, backend)
		conn.Close()
	})

	// Trojan client (no TLS)
	dialer, err := trojan.NewDialer(trojan.DialerConfig{
		Server:   trojanLn.Addr().String(),
		Password: password,
		TLS:      shared.TLSOptions{Enabled: false},
	})
	require.NoError(t, err)

	conn, err := dialer.DialContext(context.Background(), "tcp", echoLn.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte("hello trojan"))
	require.NoError(t, err)

	buf := make([]byte, 12)
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	assert.Equal(t, "hello trojan", string(buf))
}

func TestTrojan_BadPasswordFallback(t *testing.T) {
	hash := trojan.HashPassword("correct-password")

	// Fallback server (returns "fallback" for any request)
	fallbackLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer fallbackLn.Close()
	go func() {
		for {
			c, err := fallbackLn.Accept()
			if err != nil {
				return
			}
			go func() {
				c.Write([]byte("HTTP/1.1 200 OK\r\n\r\nfallback"))
				c.Close()
			}()
		}
	}()

	trojanLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer trojanLn.Close()

	srv := trojan.NewServer(trojan.ServerConfig{
		Passwords: map[string]string{hash: "user"},
		Fallback:  fallbackLn.Addr(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Serve(ctx, trojanLn, func(ctx context.Context, conn net.Conn, meta adapter.ConnMetadata) {
		conn.Close()
	})

	// Connect with wrong password — should get fallback response
	conn, err := net.Dial("tcp", trojanLn.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	// Send garbage that doesn't match the hash
	conn.Write([]byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	buf := make([]byte, 100)
	n, _ := conn.Read(buf)
	assert.Contains(t, string(buf[:n]), "fallback")
}
```

- [ ] **Step 5: Run tests**

Run: `./scripts/test.sh --pkg ./transport/trojan/`
Expected: PASS

- [ ] **Step 6: Create factory and commit**

Create `transport/trojan/factory.go` following SS pattern. Then:

```bash
git add transport/trojan/
git commit -m "feat(trojan): implement Trojan client/server with SHA224 auth and fallback"
```

---

### Task 9: URI Parsers for Subscription Compatibility

**Files:**
- Create: `subscription/parser_uri.go`
- Create: `subscription/parser_uri_test.go`
- Modify: `subscription/subscription.go`

- [ ] **Step 1: Write URI parser tests**

```go
// subscription/parser_uri_test.go
package subscription_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"shuttle/subscription"
)

func TestParseURI_Shadowsocks(t *testing.T) {
	// Standard SS URI: ss://method:password@host:port#name
	// Base64 encoded userinfo: aes-256-gcm:test-password
	uri := "ss://YWVzLTI1Ni1nY206dGVzdC1wYXNzd29yZA@hk.example.com:8388#HK-01"
	node, err := subscription.ParseURI(uri)
	require.NoError(t, err)
	assert.Equal(t, "HK-01", node.Name)
	assert.Equal(t, "shadowsocks", node.Type)
	assert.Equal(t, "hk.example.com", node.Server)
	assert.Equal(t, 8388, node.Port)
	assert.Equal(t, "aes-256-gcm", node.Options["method"])
	assert.Equal(t, "test-password", node.Options["password"])
}

func TestParseURI_VLESS(t *testing.T) {
	uri := "vless://550e8400-e29b-41d4-a716-446655440000@jp.example.com:443?type=tcp&security=tls&sni=jp.example.com#JP-01"
	node, err := subscription.ParseURI(uri)
	require.NoError(t, err)
	assert.Equal(t, "JP-01", node.Name)
	assert.Equal(t, "vless", node.Type)
	assert.Equal(t, "jp.example.com", node.Server)
	assert.Equal(t, 443, node.Port)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", node.Options["uuid"])
	assert.Equal(t, "tls", node.Options["security"])
}

func TestParseURI_Trojan(t *testing.T) {
	uri := "trojan://my-password@us.example.com:443?sni=us.example.com#US-01"
	node, err := subscription.ParseURI(uri)
	require.NoError(t, err)
	assert.Equal(t, "US-01", node.Name)
	assert.Equal(t, "trojan", node.Type)
	assert.Equal(t, "us.example.com", node.Server)
	assert.Equal(t, 443, node.Port)
	assert.Equal(t, "my-password", node.Options["password"])
}
```

- [ ] **Step 2: Implement URI parser**

```go
// subscription/parser_uri.go
package subscription

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ParseURI parses a proxy URI (ss://, vless://, trojan://) into a ProxyNode.
func ParseURI(uri string) (*ProxyNode, error) {
	scheme := ""
	for _, s := range []string{"ss://", "vless://", "trojan://", "vmess://", "hysteria2://"} {
		if strings.HasPrefix(uri, s) {
			scheme = strings.TrimSuffix(s, "://")
			break
		}
	}
	if scheme == "" {
		return nil, fmt.Errorf("unsupported URI scheme: %s", uri)
	}

	switch scheme {
	case "ss":
		return parseSSURI(uri)
	case "vless":
		return parseVLESSURI(uri)
	case "trojan":
		return parseTrojanURI(uri)
	default:
		return nil, fmt.Errorf("parser not yet implemented for scheme: %s", scheme)
	}
}

func parseSSURI(uri string) (*ProxyNode, error) {
	// Format: ss://base64(method:password)@host:port#name
	// or:     ss://method:password@host:port#name (SIP002)
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	var method, password string
	if u.User != nil {
		// SIP002 format
		method = u.User.Username()
		password, _ = u.User.Password()
	} else {
		// Legacy format: base64-encoded userinfo before @
		// Re-parse: everything between ss:// and @ is base64
		rest := strings.TrimPrefix(uri, "ss://")
		atIdx := strings.LastIndex(rest, "@")
		if atIdx < 0 {
			return nil, fmt.Errorf("invalid SS URI: no @")
		}
		encoded := rest[:atIdx]
		decoded, err := base64.URLEncoding.DecodeString(encoded)
		if err != nil {
			decoded, err = base64.RawURLEncoding.DecodeString(encoded)
			if err != nil {
				return nil, fmt.Errorf("invalid SS URI base64: %w", err)
			}
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid SS URI: decoded %q", decoded)
		}
		method = parts[0]
		password = parts[1]

		// Re-parse for host:port#name
		u, err = url.Parse("ss://user@" + rest[atIdx+1:])
		if err != nil {
			return nil, err
		}
	}

	port, _ := strconv.Atoi(u.Port())
	name := u.Fragment
	if name == "" {
		name = u.Hostname()
	}

	return &ProxyNode{
		Name:   name,
		Type:   "shadowsocks",
		Server: u.Hostname(),
		Port:   port,
		Options: map[string]any{
			"method":   method,
			"password": password,
		},
	}, nil
}

func parseVLESSURI(uri string) (*ProxyNode, error) {
	// Format: vless://uuid@host:port?params#name
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	uuid := u.User.Username()
	port, _ := strconv.Atoi(u.Port())
	name := u.Fragment
	if name == "" {
		name = u.Hostname()
	}

	opts := map[string]any{
		"uuid": uuid,
	}
	for key, values := range u.Query() {
		if len(values) > 0 {
			opts[key] = values[0]
		}
	}

	return &ProxyNode{
		Name:    name,
		Type:    "vless",
		Server:  u.Hostname(),
		Port:    port,
		Options: opts,
	}, nil
}

func parseTrojanURI(uri string) (*ProxyNode, error) {
	// Format: trojan://password@host:port?params#name
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	password := u.User.Username()
	port, _ := strconv.Atoi(u.Port())
	name := u.Fragment
	if name == "" {
		name = u.Hostname()
	}

	opts := map[string]any{
		"password": password,
	}
	for key, values := range u.Query() {
		if len(values) > 0 {
			opts[key] = values[0]
		}
	}

	return &ProxyNode{
		Name:    name,
		Type:    "trojan",
		Server:  u.Hostname(),
		Port:    port,
		Options: opts,
	}, nil
}

// ProxyNode is a parsed node (shared with provider package).
// TODO: move to a shared location or use config.OutboundOptions.
type ProxyNode struct {
	Name    string         `json:"name"`
	Type    string         `json:"type"`
	Server  string         `json:"server"`
	Port    int            `json:"port"`
	Options map[string]any `json:"options"`
}
```

- [ ] **Step 3: Run tests**

Run: `./scripts/test.sh --pkg ./subscription/ --run TestParseURI`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add subscription/parser_uri.go subscription/parser_uri_test.go
git commit -m "feat(subscription): add URI parsers for ss://, vless://, trojan:// schemes"
```

---

### Task 10: Config Types and Server Integration

**Files:**
- Modify: `config/config.go`
- Modify: `server/server.go`

- [ ] **Step 1: Add protocol-specific outbound config types**

In `config/config.go`, add outbound option types that map to the new protocol configs:

```go
// Existing OutboundConfig gains these type values:
// type: "shadowsocks" → ShadowsocksOutbound
// type: "vless" → VLESSOutbound
// type: "trojan" → TrojanOutbound
```

The `OutboundConfig.Options` field (likely `map[string]any` or a typed options field) carries the per-protocol configuration. Check existing OutboundConfig structure and extend appropriately.

- [ ] **Step 2: Wire InboundHandler into server startup**

In `server/server.go`, extend the server startup to:
1. Check for per-request protocol inbound configs
2. For each one, call `adapter.GetDialerFactory(type)` → `NewInboundHandler()`
3. Start a `net.Listener` (with TLS if configured) and call `handler.Serve()`
4. Route connections through the existing server handler pipeline

The exact integration depends on the current server code — read `server/server.go` first.

- [ ] **Step 3: Run full test suite**

Run: `./scripts/test.sh`
Expected: All existing tests pass.

- [ ] **Step 4: Commit**

```bash
git add config/ server/
git commit -m "feat: wire SS/VLESS/Trojan protocols into config and server startup"
```

---

### Task 11: E2E Sandbox Tests

**Files:**
- Create: `test/e2e/protocol_test.go` (`//go:build sandbox`)

- [ ] **Step 1: Write E2E tests for all three protocols**

```go
//go:build sandbox

// test/e2e/protocol_test.go
package e2e

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests run in the Docker sandbox environment.
// They test the full proxy pipeline: client → SS/VLESS/Trojan → server → httpbin.

func TestE2E_Shadowsocks_ProxyHTTP(t *testing.T) {
	// Test: client SOCKS5 → SS outbound → SS server → httpbin
	// Uses sandbox infrastructure (server running SS inbound, client with SS outbound config)
	// Implementation depends on sandbox setup — configure server with SS inbound,
	// client with SS outbound, verify HTTP request reaches httpbin through proxy.
	t.Skip("implement after sandbox configs are updated for new protocols")
}

func TestE2E_VLESS_ProxyHTTP(t *testing.T) {
	t.Skip("implement after sandbox configs are updated for new protocols")
}

func TestE2E_Trojan_ProxyHTTP(t *testing.T) {
	t.Skip("implement after sandbox configs are updated for new protocols")
}
```

Note: Full E2E tests require updating sandbox Docker configs to include SS/VLESS/Trojan server inbounds. The test skeleton is created now; implementation is filled in once sandbox configs are updated.

- [ ] **Step 2: Commit**

```bash
git add test/e2e/protocol_test.go
git commit -m "test: add E2E test skeletons for SS/VLESS/Trojan protocols"
```
