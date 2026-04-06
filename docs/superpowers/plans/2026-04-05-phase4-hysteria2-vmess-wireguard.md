# Phase 4: Hysteria2 + VMess + WireGuard

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add second-batch protocol support covering the long tail of proxy deployments.

**Architecture:** Same pattern as Phase 2 — each protocol implements `adapter.Dialer` (client) and `adapter.InboundHandler` (server, except WireGuard). Hysteria2 reuses Shuttle's existing `quicfork/` with Brutal CC hooks. VMess uses AEAD mode only. WireGuard is client-only using userspace implementation.

**Tech Stack:** Go 1.24+, `quicfork/` (Hysteria2), `github.com/v2fly/v2ray-core` (VMess), `golang.zx2c4.com/wireguard` + gVisor netstack (WireGuard)

**Spec:** `docs/superpowers/specs/2026-04-05-ecosystem-compatibility-design.md` — Section 5.4-5.6

**Depends on:** Phase 1 (adapter layer), Phase 2 patterns (shared TLS, addr codec)

---

## File Structure

### New Files
| File | Responsibility |
|------|---------------|
| `transport/hysteria2/dialer.go` | Hy2 client: QUIC+HTTP/3 auth+Brutal CC |
| `transport/hysteria2/server.go` | Hy2 server: QUIC listener+auth |
| `transport/hysteria2/factory.go` | Factory registration |
| `transport/hysteria2/protocol.go` | Hy2 HTTP/3 auth handshake + stream framing |
| `transport/hysteria2/dialer_test.go` | Client/server echo test |
| `transport/vmess/dialer.go` | VMess client: AEAD mode |
| `transport/vmess/server.go` | VMess server: AEAD mode |
| `transport/vmess/factory.go` | Factory registration |
| `transport/vmess/protocol.go` | VMess AEAD header codec |
| `transport/vmess/dialer_test.go` | Client/server echo test |
| `transport/wireguard/dialer.go` | WG client: userspace tunnel |
| `transport/wireguard/factory.go` | Factory registration (client only) |
| `transport/wireguard/netstack.go` | gVisor netstack integration |
| `transport/wireguard/dialer_test.go` | Dialer unit test |
| `subscription/parser_uri_hy2.go` | hysteria2:// URI parser |
| `subscription/parser_uri_vmess.go` | vmess:// URI parser (base64 JSON) |

### Modified Files
| File | Change |
|------|--------|
| `subscription/parser_uri.go` | Add hysteria2:// and vmess:// dispatch |
| `config/config.go` | Add Hysteria2/VMess/WireGuard outbound config types |
| `go.mod` | Add wireguard-go, gVisor, v2ray-core dependencies |

---

### Task 1: Hysteria2 Protocol

**Files:**
- Create: `transport/hysteria2/protocol.go`
- Create: `transport/hysteria2/dialer.go`
- Create: `transport/hysteria2/server.go`
- Create: `transport/hysteria2/factory.go`
- Create: `transport/hysteria2/dialer_test.go`

- [ ] **Step 1: Implement Hy2 protocol (HTTP/3 auth + stream framing)**

Hysteria2 uses HTTP/3 for the initial auth handshake, then QUIC streams for data:

```go
// transport/hysteria2/protocol.go
package hysteria2

// Hysteria2 auth flow:
// 1. Client opens QUIC connection with Brutal CC
// 2. Client sends HTTP/3 request: POST /auth with password in Hysteria-Auth header
// 3. Server responds 233 if OK, 404 if auth fails
// 4. Client opens QUIC streams for each TCP connection
// 5. Each stream starts with: [request_id(4)] [addr_len(2)] [addr] [padding_len(2)] [padding]

const (
	AuthPath       = "/auth"
	AuthHeader     = "Hysteria-Auth"
	StatusAuthOK   = 233
	FrameHeaderLen = 8 // request_id(4) + addr_len(2) + padding_len(2)
)

// EncodeStreamHeader writes the Hy2 stream header.
func EncodeStreamHeader(w io.Writer, requestID uint32, address string) error {
	addrBytes := []byte(address)
	header := make([]byte, 4+2+len(addrBytes)+2)
	binary.BigEndian.PutUint32(header[0:4], requestID)
	binary.BigEndian.PutUint16(header[4:6], uint16(len(addrBytes)))
	copy(header[6:], addrBytes)
	binary.BigEndian.PutUint16(header[6+len(addrBytes):], 0) // no padding
	_, err := w.Write(header)
	return err
}

// DecodeStreamHeader reads the Hy2 stream header.
func DecodeStreamHeader(r io.Reader) (requestID uint32, address string, err error) {
	var header [4 + 2]byte
	if _, err = io.ReadFull(r, header[:]); err != nil {
		return
	}
	requestID = binary.BigEndian.Uint32(header[0:4])
	addrLen := binary.BigEndian.Uint16(header[4:6])

	addrBuf := make([]byte, addrLen)
	if _, err = io.ReadFull(r, addrBuf); err != nil {
		return
	}
	address = string(addrBuf)

	// Read and discard padding
	var padLen [2]byte
	if _, err = io.ReadFull(r, padLen[:]); err != nil {
		return
	}
	pl := binary.BigEndian.Uint16(padLen[:])
	if pl > 0 {
		_, err = io.CopyN(io.Discard, r, int64(pl))
	}
	return
}
```

- [ ] **Step 2: Implement Hy2 Dialer**

```go
// transport/hysteria2/dialer.go
package hysteria2

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

type DialerConfig struct {
	Server    string
	Password  string
	TLS       tls.Config
	Bandwidth BandwidthConfig // for Brutal CC
}

type BandwidthConfig struct {
	Up   uint64 // bytes/sec
	Down uint64 // bytes/sec
}

type Dialer struct {
	server    string
	password  string
	tlsCfg    *tls.Config
	bandwidth BandwidthConfig
	conn      atomic.Pointer[quic.Connection]
	reqID     atomic.Uint32
}

func NewDialer(cfg DialerConfig) (*Dialer, error) {
	tlsCfg := cfg.TLS
	tlsCfg.NextProtos = []string{"h3"} // Hysteria2 uses HTTP/3
	return &Dialer{
		server:    cfg.Server,
		password:  cfg.Password,
		tlsCfg:    &tlsCfg,
		bandwidth: cfg.Bandwidth,
	}, nil
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	qconn, err := d.getOrDialQUIC(ctx)
	if err != nil {
		return nil, err
	}

	stream, err := qconn.OpenStreamSync(ctx)
	if err != nil {
		// Connection might be stale, retry once
		d.conn.Store(nil)
		qconn, err = d.getOrDialQUIC(ctx)
		if err != nil {
			return nil, err
		}
		stream, err = qconn.OpenStreamSync(ctx)
		if err != nil {
			return nil, err
		}
	}

	reqID := d.reqID.Add(1)
	if err := EncodeStreamHeader(stream, reqID, address); err != nil {
		stream.Close()
		return nil, err
	}

	return &streamConn{Stream: stream, local: qconn.LocalAddr(), remote: qconn.RemoteAddr()}, nil
}

func (d *Dialer) getOrDialQUIC(ctx context.Context) (quic.Connection, error) {
	if c := d.conn.Load(); c != nil {
		return *c, nil
	}

	// Dial QUIC with Brutal CC
	// Note: quicfork hooks for Brutal CC will be configured via quic.Config
	qconn, err := quic.DialAddr(ctx, d.server, d.tlsCfg, &quic.Config{
		MaxIncomingStreams: 1024,
		// Brutal CC config attached via quicfork hooks
	})
	if err != nil {
		return nil, err
	}

	// HTTP/3 auth handshake
	if err := d.authenticate(ctx, qconn); err != nil {
		qconn.CloseWithError(0, "auth failed")
		return nil, err
	}

	d.conn.Store(&qconn)
	return qconn, nil
}

func (d *Dialer) authenticate(ctx context.Context, qconn quic.Connection) error {
	// Open HTTP/3 on the QUIC connection for auth
	rt := &http3.RoundTripper{}
	// Send auth request
	req, _ := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("https://%s%s", d.server, AuthPath), nil)
	req.Header.Set(AuthHeader, d.password)

	// In practice, Hy2 auth uses a dedicated QUIC stream.
	// Simplified: the auth is sent as the first HTTP/3 request.
	// Full implementation will use the Hy2 auth protocol directly.
	_ = rt // placeholder — actual HTTP/3 auth depends on quicfork API
	return nil
}

func (d *Dialer) Type() string { return "hysteria2" }
func (d *Dialer) Close() error {
	if c := d.conn.Load(); c != nil {
		(*c).CloseWithError(0, "closed")
	}
	return nil
}
```

Note: Full Hysteria2 auth implementation requires deeper integration with `quicfork/` for Brutal CC hooks. The test will validate the stream framing, with auth as a TODO for the actual quicfork wiring.

- [ ] **Step 3: Implement Hy2 server**

```go
// transport/hysteria2/server.go
package hysteria2

import (
	"context"
	"crypto/tls"
	"net"

	"github.com/quic-go/quic-go"

	"shuttle/adapter"
)

type ServerConfig struct {
	Password  string
	TLS       tls.Config
	Bandwidth BandwidthConfig
}

type Server struct {
	password  string
	tlsCfg    *tls.Config
	bandwidth BandwidthConfig
}

func NewServer(cfg ServerConfig) (*Server, error) {
	tlsCfg := cfg.TLS
	tlsCfg.NextProtos = []string{"h3"}
	return &Server{
		password:  cfg.Password,
		tlsCfg:    &tlsCfg,
		bandwidth: cfg.Bandwidth,
	}, nil
}

func (s *Server) Serve(ctx context.Context, ln net.Listener, handler adapter.ConnHandler) error {
	// Create QUIC listener
	ql, err := quic.Listen(ln.(*net.UDPConn), s.tlsCfg, &quic.Config{
		MaxIncomingStreams: 1024,
	})
	if err != nil {
		return err
	}

	for {
		qconn, err := ql.Accept(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				continue
			}
		}
		go s.handleQUICConn(ctx, qconn, handler)
	}
}

func (s *Server) handleQUICConn(ctx context.Context, qconn quic.Connection, handler adapter.ConnHandler) {
	// TODO: Verify HTTP/3 auth on first stream

	for {
		stream, err := qconn.AcceptStream(ctx)
		if err != nil {
			return
		}
		go func() {
			_, addr, err := DecodeStreamHeader(stream)
			if err != nil {
				stream.Close()
				return
			}

			conn := &streamConn{Stream: stream, local: qconn.LocalAddr(), remote: qconn.RemoteAddr()}
			handler(ctx, conn, adapter.ConnMetadata{
				Network:     "tcp",
				Destination: addr,
				Source:      qconn.RemoteAddr().String(),
			})
		}()
	}
}

func (s *Server) Type() string { return "hysteria2" }
func (s *Server) Close() error { return nil }
```

- [ ] **Step 4: Write test, factory, commit**

Follow the echo-through-server pattern from Phase 2. Register factory.

```bash
git add transport/hysteria2/
git commit -m "feat(hysteria2): implement Hysteria2 client/server with QUIC + Brutal CC"
```

---

### Task 2: VMess Protocol (AEAD only)

**Files:**
- Create: `transport/vmess/protocol.go`
- Create: `transport/vmess/dialer.go`
- Create: `transport/vmess/server.go`
- Create: `transport/vmess/factory.go`
- Create: `transport/vmess/dialer_test.go`

- [ ] **Step 1: Add v2ray-core dependency**

```bash
go get github.com/v2fly/v2ray-core/v5@latest
```

- [ ] **Step 2: Implement VMess Dialer using v2ray-core**

```go
// transport/vmess/dialer.go
package vmess

import (
	"context"
	"crypto/tls"
	"net"

	vmessout "github.com/v2fly/v2ray-core/v5/proxy/vmess/outbound"

	"shuttle/transport/shared"
)

type DialerConfig struct {
	Server string
	UUID   string
	Cipher string // "auto", "aes-128-gcm", "chacha20-poly1305", "none"
	TLS    shared.TLSOptions
}

type Dialer struct {
	server string
	uuid   string
	cipher string
	tls    *tls.Config
}

func NewDialer(cfg DialerConfig) (*Dialer, error) {
	tlsCfg, err := shared.BuildClientTLS(cfg.TLS)
	if err != nil {
		return nil, err
	}
	cipher := cfg.Cipher
	if cipher == "" || cipher == "auto" {
		cipher = "aes-128-gcm"
	}
	return &Dialer{
		server: cfg.Server,
		uuid:   cfg.UUID,
		cipher: cipher,
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

	// 2. TLS handshake if configured
	var conn net.Conn = rawConn
	if d.tls != nil {
		tlsConn := tls.Client(rawConn, d.tls)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			rawConn.Close()
			return nil, err
		}
		conn = tlsConn
	}

	// 3. VMess AEAD handshake
	// Use v2ray-core's VMess implementation for the protocol layer.
	// The exact API depends on the v2ray-core version.
	// Simplified: write VMess AEAD request header, read response header.
	vmessConn, err := wrapVMessClient(conn, d.uuid, d.cipher, network, address)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return vmessConn, nil
}

func (d *Dialer) Type() string { return "vmess" }
func (d *Dialer) Close() error { return nil }
```

Note: `wrapVMessClient` is the integration point with v2ray-core's VMess AEAD implementation. The exact API calls depend on the v2ray-core version available at implementation time. The protocol is complex (AES-128-CFB header + AEAD body) — using the reference implementation avoids reinventing it.

- [ ] **Step 3: Implement VMess server**

Similar pattern — use v2ray-core's VMess inbound handler to decode and validate, then hand off the decrypted connection.

- [ ] **Step 4: Write test, factory, commit**

```bash
git add transport/vmess/
git commit -m "feat(vmess): implement VMess AEAD client/server using v2ray-core"
```

---

### Task 3: WireGuard Client (Outbound Only)

**Files:**
- Create: `transport/wireguard/dialer.go`
- Create: `transport/wireguard/netstack.go`
- Create: `transport/wireguard/factory.go`
- Create: `transport/wireguard/dialer_test.go`

- [ ] **Step 1: Add dependencies**

```bash
go get golang.zx2c4.com/wireguard@latest
go get gvisor.dev/gvisor@latest
```

- [ ] **Step 2: Implement userspace WireGuard with gVisor netstack**

```go
// transport/wireguard/netstack.go
package wireguard

import (
	"net/netip"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

// TunnelConfig configures the userspace WireGuard tunnel.
type TunnelConfig struct {
	PrivateKey string
	Addresses  []netip.Prefix // local virtual IPs
	DNS        []netip.Addr
	MTU        int
	Peers      []PeerConfig
}

type PeerConfig struct {
	PublicKey    string
	Endpoint     string            // host:port
	AllowedIPs  []netip.Prefix
	PresharedKey string            // optional
	Keepalive    int               // seconds
}

// Tunnel wraps a userspace WireGuard device with gVisor netstack.
type Tunnel struct {
	dev  *device.Device
	tnet *netstack.Net
}

// NewTunnel creates a userspace WireGuard tunnel.
func NewTunnel(cfg TunnelConfig) (*Tunnel, error) {
	mtu := cfg.MTU
	if mtu == 0 {
		mtu = 1280
	}

	// Create gVisor TUN device
	tun, tnet, err := netstack.CreateNetTUN(cfg.Addresses, cfg.DNS, mtu)
	if err != nil {
		return nil, err
	}

	// Create WireGuard device
	dev := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))

	// Build IPC config
	ipc := buildIPC(cfg)
	if err := dev.IpcSet(ipc); err != nil {
		dev.Close()
		return nil, err
	}

	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, err
	}

	return &Tunnel{dev: dev, tnet: tnet}, nil
}

func (t *Tunnel) Close() error {
	t.dev.Close()
	return nil
}

func buildIPC(cfg TunnelConfig) string {
	var ipc string
	ipc += "private_key=" + hexKey(cfg.PrivateKey) + "\n"
	for _, peer := range cfg.Peers {
		ipc += "public_key=" + hexKey(peer.PublicKey) + "\n"
		if peer.PresharedKey != "" {
			ipc += "preshared_key=" + hexKey(peer.PresharedKey) + "\n"
		}
		ipc += "endpoint=" + peer.Endpoint + "\n"
		for _, allowed := range peer.AllowedIPs {
			ipc += "allowed_ip=" + allowed.String() + "\n"
		}
		if peer.Keepalive > 0 {
			ipc += "persistent_keepalive_interval=" + strconv.Itoa(peer.Keepalive) + "\n"
		}
	}
	return ipc
}
```

- [ ] **Step 3: Implement WireGuard Dialer**

```go
// transport/wireguard/dialer.go
package wireguard

import (
	"context"
	"net"
)

type DialerConfig struct {
	Tunnel TunnelConfig
}

type Dialer struct {
	tunnel *Tunnel
}

func NewDialer(cfg DialerConfig) (*Dialer, error) {
	tunnel, err := NewTunnel(cfg.Tunnel)
	if err != nil {
		return nil, err
	}
	return &Dialer{tunnel: tunnel}, nil
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// Dial through the WireGuard tunnel using gVisor's netstack
	switch network {
	case "tcp", "tcp4", "tcp6":
		return d.tunnel.tnet.DialContextTCPAddrPort(ctx, netip.MustParseAddrPort(address))
	case "udp", "udp4", "udp6":
		return d.tunnel.tnet.DialUDP(nil, netip.MustParseAddrPort(address))
	default:
		return nil, fmt.Errorf("unsupported network: %s", network)
	}
}

func (d *Dialer) Type() string { return "wireguard" }

func (d *Dialer) Close() error {
	return d.tunnel.Close()
}
```

- [ ] **Step 4: Implement factory (client only, no server)**

```go
// transport/wireguard/factory.go
package wireguard

import "shuttle/adapter"

func init() {
	adapter.Register(&factory{})
}

type factory struct{}

func (f *factory) Type() string { return "wireguard" }

func (f *factory) NewClient(cfg *config.ClientConfig, opts adapter.FactoryOptions) (adapter.ClientTransport, error) {
	return nil, nil
}
func (f *factory) NewServer(cfg *config.ServerConfig, opts adapter.FactoryOptions) (adapter.ServerTransport, error) {
	return nil, nil
}

func (f *factory) NewDialer(cfg map[string]any, opts adapter.FactoryOptions) (adapter.Dialer, error) {
	// Parse TunnelConfig from cfg map
	tunnelCfg, err := parseTunnelConfig(cfg)
	if err != nil {
		return nil, err
	}
	return NewDialer(DialerConfig{Tunnel: tunnelCfg})
}

func (f *factory) NewInboundHandler(cfg map[string]any, opts adapter.FactoryOptions) (adapter.InboundHandler, error) {
	return nil, nil // WireGuard: client only
}
```

- [ ] **Step 5: Write test, commit**

WireGuard unit tests are limited without a real peer. Test tunnel creation and basic setup:

```bash
git add transport/wireguard/
git commit -m "feat(wireguard): implement userspace WireGuard dialer with gVisor netstack"
```

---

### Task 4: URI Parsers for Hysteria2 and VMess

**Files:**
- Create: `subscription/parser_uri_hy2.go`
- Create: `subscription/parser_uri_vmess.go`
- Modify: `subscription/parser_uri.go`

- [ ] **Step 1: Add hysteria2:// parser**

```go
// subscription/parser_uri_hy2.go
package subscription

import (
	"net/url"
	"strconv"
)

func parseHysteria2URI(uri string) (*ProxyNode, error) {
	// Format: hysteria2://password@host:port?sni=xxx&insecure=1#name
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
		Type:    "hysteria2",
		Server:  u.Hostname(),
		Port:    port,
		Options: opts,
	}, nil
}
```

- [ ] **Step 2: Add vmess:// parser**

```go
// subscription/parser_uri_vmess.go
package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

func parseVMessURI(uri string) (*ProxyNode, error) {
	// Format: vmess://base64-json
	encoded := uri[len("vmess://"):]
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("invalid vmess base64: %w", err)
		}
	}

	var v struct {
		V    interface{} `json:"v"`
		PS   string      `json:"ps"`   // name
		Add  string      `json:"add"`  // server
		Port interface{} `json:"port"` // port (string or int)
		ID   string      `json:"id"`   // UUID
		Aid  interface{} `json:"aid"`  // alterId
		Scy  string      `json:"scy"`  // cipher
		Net  string      `json:"net"`  // transport
		Type string      `json:"type"` // header type
		Host string      `json:"host"`
		Path string      `json:"path"`
		TLS  string      `json:"tls"`
		SNI  string      `json:"sni"`
	}
	if err := json.Unmarshal(decoded, &v); err != nil {
		return nil, fmt.Errorf("invalid vmess JSON: %w", err)
	}

	port := 0
	switch p := v.Port.(type) {
	case float64:
		port = int(p)
	case string:
		fmt.Sscanf(p, "%d", &port)
	}

	name := v.PS
	if name == "" {
		name = v.Add
	}

	return &ProxyNode{
		Name:   name,
		Type:   "vmess",
		Server: v.Add,
		Port:   port,
		Options: map[string]any{
			"uuid":   v.ID,
			"cipher": v.Scy,
			"net":    v.Net,
			"host":   v.Host,
			"path":   v.Path,
			"tls":    v.TLS,
			"sni":    v.SNI,
		},
	}, nil
}
```

- [ ] **Step 3: Register in ParseURI**

In `subscription/parser_uri.go`, add to the switch:

```go
case "hysteria2", "hy2":
    return parseHysteria2URI(uri)
case "vmess":
    return parseVMessURI(uri)
```

- [ ] **Step 4: Write tests, commit**

```bash
git add subscription/
git commit -m "feat(subscription): add hysteria2:// and vmess:// URI parsers"
```
