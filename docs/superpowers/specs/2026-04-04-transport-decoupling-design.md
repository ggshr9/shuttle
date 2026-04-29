# Transport Layer Decoupling Design

## Problem

Shuttle's transport layer is protocol-driven: each transport (H3, Reality, CDN, WebRTC) bundles its own protocol stack, multiplexing, authentication, and encryption into a monolithic implementation. This causes:

1. **Engine hardcodes transport instantiation** — `engine_setup.go:51-127` and `server.go:110-168` directly import and construct each transport type. Adding a new transport requires editing core engine code.
2. **No component reuse** — yamux multiplexing is copy-pasted across Reality, CDN, and WebRTC. HMAC auth is reimplemented per transport.
3. **Congestion control is H3-only** — only H3 receives the CC adapter because QUIC exposes a CC hook; TCP-based transports have no CC integration point.
4. **Interfaces are scattered** — core abstractions live in `transport/iface.go`, `plugin/iface.go`, `congestion/adaptive.go` with no single entry point.

## Goals

- New transport types register themselves via `init()` — engine never imports transport packages directly
- Shared components (yamux, HMAC auth, TLS) are reusable building blocks, not copy-pasted
- Internal architecture is composable; external config does not change
- QUIC's integrated nature is respected, not forced into an abstraction that doesn't fit
- Minimal disruption: existing tests continue to pass, wire protocol unchanged by default

## Non-Goals

- Exposing pipeline composition to end users (config stays `transport.h3.enabled: true`)
- Replacing existing wire protocols (backward compatible)
- Building a generic proxy framework (Shuttle-specific optimizations preserved)

## Design

### 1. Centralized Interfaces — `adapter/` package

All core interfaces move to a single `adapter/` package. This is the only package that every subsystem imports for type contracts.

**adapter/transport.go:**
```go
package adapter

type Stream interface {
    io.ReadWriteCloser
    StreamID() uint64
}

type Connection interface {
    OpenStream(ctx context.Context) (Stream, error)
    AcceptStream(ctx context.Context) (Stream, error)
    Close() error
    LocalAddr() net.Addr
    RemoteAddr() net.Addr
}

type ClientTransport interface {
    Dial(ctx context.Context, addr string) (Connection, error)
    Type() string
    Close() error
}

type ServerTransport interface {
    Listen(ctx context.Context) error
    Accept(ctx context.Context) (Connection, error)
    Type() string
    Close() error
}
```

**adapter/security.go:**
```go
// SecureWrapper wraps a net.Conn with a security layer.
// Multiple wrappers can be chained: TLS → Noise → PQ-KEM.
type SecureWrapper interface {
    WrapClient(ctx context.Context, conn net.Conn) (net.Conn, error)
    WrapServer(ctx context.Context, conn net.Conn) (net.Conn, error)
}
```

**adapter/mux.go:**
```go
// Multiplexer creates a multiplexed Connection from a raw net.Conn.
type Multiplexer interface {
    Client(conn net.Conn) (Connection, error)
    Server(conn net.Conn) (Connection, error)
}
```

**adapter/auth.go:**
```go
// Authenticator performs client/server authentication on a connection.
type Authenticator interface {
    AuthClient(conn net.Conn) error
    AuthServer(conn net.Conn) (user string, err error)
}
```

**adapter/registry.go:**
```go
// TransportFactory creates client and server transports from config.
type TransportFactory interface {
    Type() string
    NewClient(cfg *config.ClientConfig) (ClientTransport, error)
    NewServer(cfg *config.ServerConfig) (ServerTransport, error)
}

var registry = map[string]TransportFactory{}

func Register(f TransportFactory) { registry[f.Type()] = f }
func Get(name string) TransportFactory { return registry[name] }
func All() map[string]TransportFactory { return registry }
```

Each transport package calls `adapter.Register()` in its `init()` function. Engine uses `adapter.All()` to discover available transports.

### 2. Two-Track Model

QUIC fundamentally bundles transport+security+multiplexing. TCP/WebRTC need these added externally. Rather than forcing a single abstraction, we acknowledge two tracks that converge at the `Connection` interface.

**Track A — Stream-native (QUIC):**
```
QUICDialer(TLS+ChromeFP+CC) → quic.Connection → QUICConnectionAdapter → Connection
```
No wrappers, no yamux. QUIC handles everything internally. The `QUICConnectionAdapter` simply adapts `quic.Connection` to `adapter.Connection`.

**Track B — Byte-stream (TCP, WebRTC DataChannel):**
```
net.Conn → [SecureWrapper chain] → Multiplexer → Connection
```
Security wrappers are composed as a chain. Multiplexer (yamux) converts the wrapped `net.Conn` into a multiplexed `Connection`.

**Shared building blocks for Track B:**

| Component | Package | What it does |
|-----------|---------|-------------|
| `TLSWrapper` | `transport/security/tls` | TLS 1.3 with configurable SNI, ALPN, fingerprint |
| `NoiseWrapper` | `transport/security/noise` | Noise IK handshake + encryption |
| `PQKEMWrapper` | `transport/security/pqkem` | Post-quantum hybrid KEM exchange |
| `YamuxMux` | `transport/mux/yamux` | yamux session multiplexing |
| `HMACAuth` | `transport/auth/hmac` | Nonce + HMAC-SHA256 authentication |

### 3. Preset Transports (Existing Types Reimplemented)

Each preset is a factory that assembles components. The user-facing config does not change.

**H3 Preset (Track A):**
```go
func (f *H3Factory) NewClient(cfg *config.ClientConfig) (adapter.ClientTransport, error) {
    // QUIC is self-contained: TLS + mux + CC all inside quic-go
    // Just configure and return
    return &h3Client{
        quicConfig: buildQUICConfig(cfg),  // Chrome fingerprint, CC adapter
        tlsConfig:  buildTLSConfig(cfg),   // SNI, ALPN
        auth:       auth.NewHMAC(cfg.Transport.H3.Password),
    }, nil
}
```

**Reality Preset (Track B):**
```go
func (f *RealityFactory) NewClient(cfg *config.ClientConfig) (adapter.ClientTransport, error) {
    rc := cfg.Transport.Reality
    return transport.NewByteStreamClient(
        transport.ByteStreamConfig{
            Dialer: net.Dialer{},
            Addr:   rc.ServerAddr,
            Security: []adapter.SecureWrapper{
                tls.New(tls.Config{ServerName: rc.ServerName, MinVersion: tls.VersionTLS13}),
                noise.New(noise.Config{PublicKey: rc.PublicKey, Password: rc.Password}),
                pqkem.NewIf(rc.PostQuantum),  // returns no-op wrapper if disabled
            },
            Mux:  yamux.New(cfg.Yamux),
            Auth: nil,  // Noise provides implicit auth
        },
    ), nil
}
```

**CDN H2 Preset (Track B):**
```go
func (f *CDNH2Factory) NewClient(cfg *config.ClientConfig) (adapter.ClientTransport, error) {
    cc := cfg.Transport.CDN
    return transport.NewByteStreamClient(
        transport.ByteStreamConfig{
            Dialer:   net.Dialer{},
            Addr:     cc.ServerAddr,
            Security: []adapter.SecureWrapper{
                tls.New(tls.Config{ServerName: cc.FrontDomain}),
                h2framer.New(h2framer.Config{Host: cc.Domain, Path: cc.Path}),
            },
            Mux:  yamux.New(cfg.Yamux),
            Auth: auth.NewHMAC(cc.Password),
        },
    ), nil
}
```

### 4. ByteStreamClient — The Track B Assembler

A generic implementation that composes dialer + security chain + mux + auth into a `ClientTransport`. This eliminates copy-paste across Reality, CDN, and WebRTC.

**transport/bytestream.go:**
```go
type ByteStreamConfig struct {
    Dialer   adapter.Dialer              // How to establish raw connection
    Addr     string                      // Server address
    Security []adapter.SecureWrapper     // Security chain (applied in order)
    Mux      adapter.Multiplexer         // Stream multiplexer
    Auth     adapter.Authenticator       // nil = no auth step
}

type byteStreamClient struct {
    cfg    ByteStreamConfig
    closed atomic.Bool
}

func (c *byteStreamClient) Dial(ctx context.Context, addr string) (adapter.Connection, error) {
    // 1. Establish raw connection
    raw, err := c.cfg.Dialer.Dial(ctx, addr)
    if err != nil {
        return nil, err
    }

    // 2. Apply security chain
    conn := raw
    for _, wrapper := range c.cfg.Security {
        conn, err = wrapper.WrapClient(ctx, conn)
        if err != nil {
            raw.Close()
            return nil, err
        }
    }

    // 3. Authenticate
    if c.cfg.Auth != nil {
        if err := c.cfg.Auth.AuthClient(conn); err != nil {
            raw.Close()
            return nil, err
        }
    }

    // 4. Multiplex
    return c.cfg.Mux.Client(conn)
}
```

Server side has an equivalent `ByteStreamServer` with `Accept()` that reverses the chain (accept → unwrap security → auth → mux server).

### 5. Engine Changes

**Before:**
```go
// engine_setup.go — directly imports h3, reality, cdn, webrtc
import (
    "github.com/ggshr9/shuttle/transport/h3"
    "github.com/ggshr9/shuttle/transport/reality"
    "github.com/ggshr9/shuttle/transport/cdn"
    rtcTransport "github.com/ggshr9/shuttle/transport/webrtc"
)

func (e *Engine) buildTransports(cfg *config.ClientConfig, cc quic.CongestionControl) []transport.ClientTransport {
    if cfg.Transport.H3.Enabled {
        transports = append(transports, h3.NewClient(...))
    }
    if cfg.Transport.Reality.Enabled {
        transports = append(transports, reality.NewClient(...))
    }
    // ... hardcoded for each type
}
```

**After:**
```go
// engine_setup.go — imports only adapter
import "github.com/ggshr9/shuttle/adapter"

// Transport packages register via init() in a separate imports file:
// engine/imports.go
import (
    _ "github.com/ggshr9/shuttle/transport/h3"
    _ "github.com/ggshr9/shuttle/transport/reality"
    _ "github.com/ggshr9/shuttle/transport/cdn"
    _ "github.com/ggshr9/shuttle/transport/webrtc"
)

func (e *Engine) buildTransports(cfg *config.ClientConfig) []adapter.ClientTransport {
    var transports []adapter.ClientTransport
    for _, factory := range adapter.All() {
        t, err := factory.NewClient(cfg)
        if err != nil { continue }
        if t != nil {
            transports = append(transports, t)
        }
    }
    return transports
}
```

Each factory internally checks `cfg.Transport.{Type}.Enabled` and returns `nil` if disabled. Engine no longer knows about specific transport types.

Server side (`server/server.go`) uses the same pattern with `factory.NewServer(cfg)`.

### 6. Package Structure

```
adapter/                          # Core interfaces (NEW)
  transport.go                    # Stream, Connection, ClientTransport, ServerTransport
  security.go                     # SecureWrapper
  mux.go                          # Multiplexer
  auth.go                         # Authenticator
  registry.go                     # TransportFactory registry

transport/
  bytestream.go                   # ByteStreamClient/Server assembler (NEW)
  security/                       # Shared security wrappers (NEW)
    tls/tls.go                    #   TLS wrapper (extracted from reality/cdn)
    noise/noise.go                #   Noise IK wrapper (extracted from reality)
    pqkem/pqkem.go                #   PQ-KEM wrapper (extracted from reality)
  mux/                            # Shared multiplexers (NEW)
    yamux/yamux.go                #   yamux (extracted, was copy-pasted)
    quicmux/quicmux.go            #   QUIC native stream adapter
  auth/                           # Shared auth (EXISTS, extended)
    hmac.go                       #   HMAC-SHA256 (already exists)
  h3/                             # H3 preset (REFACTORED)
    factory.go                    #   TransportFactory registration
    client.go                     #   Track A implementation
    server.go
  reality/                        # Reality preset (REFACTORED)
    factory.go                    #   TransportFactory registration
    client.go                     #   Track B: TLS + Noise + PQ + yamux
    server.go
  cdn/                            # CDN preset (REFACTORED)
    factory.go                    #   TransportFactory registration
    h2.go                         #   Track B: TLS + H2Framer + yamux
    grpc.go                       #   Track B: TLS + GRPCFramer + yamux
    server.go
  webrtc/                         # WebRTC preset (REFACTORED)
    factory.go                    #   TransportFactory registration
    client.go                     #   Track B: signaling + yamux
    server.go
  selector/                       # Transport selection (UNCHANGED)
  resilient/                      # Reconnection wrapper (UNCHANGED)
  conformance/                    # Shared test suite (UNCHANGED)
```

### 7. Migration Strategy

Phase 1 (foundation): Create `adapter/` package, define all interfaces, add registry. All existing code continues working — just add the new package alongside.

Phase 2 (extract shared components): Extract yamux, TLS, HMAC into `transport/security/`, `transport/mux/`, keeping existing implementations working. Each extracted component implements `adapter.SecureWrapper` or `adapter.Multiplexer`.

Phase 3 (ByteStreamClient): Implement the generic byte-stream assembler. Test it in isolation using the extracted components.

Phase 4 (refactor transports): One transport at a time, refactor to use ByteStreamClient + extracted components + factory registration. Start with CDN (simplest), then Reality, then WebRTC. H3 gets a factory wrapper but keeps its internal QUIC logic.

Phase 5 (engine decoupling): Replace `buildTransports()` hardcoded instantiation with registry iteration. Replace server-side transport instantiation similarly. Remove direct transport imports from engine/.

Phase 6 (cleanup): Remove old interfaces from `transport/iface.go` (replaced by `adapter/`), update all consumers, run full test suite.

### 8. What Does NOT Change

- **User-facing config format** — `transport.h3.enabled`, `transport.reality.server_addr`, etc.
- **Wire protocols** — H3 HMAC auth, Reality Noise handshake, CDN H2/gRPC framing, WebRTC signaling
- **Selector behavior** — transport selection, migration, multipath all work the same
- **Engine lifecycle** — Start/Stop/Reload state machine unchanged
- **Test structure** — existing tests stay, conformance suite still validates each transport

### 9. Risk Assessment

| Risk | Mitigation |
|------|-----------|
| Abstraction overhead in hot path | ByteStreamClient.Dial is called once per connection, not per packet. Wrappers are `net.Conn` — zero overhead after setup. |
| Breaking existing transports during refactor | One transport at a time. Each phase ends with green test suite. Conformance tests catch regressions. |
| Over-abstracting H3/QUIC | H3 stays self-contained. Factory is a thin wrapper. No forced decomposition. |
| Interface bloat in adapter/ | 5 files, ~15 methods total. Smaller than most Go standard library interfaces. |
