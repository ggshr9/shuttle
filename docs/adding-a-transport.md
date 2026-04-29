# Adding a New Transport to Shuttle

This guide walks through adding a new transport protocol to Shuttle's pluggable transport system.

## Architecture Overview

Shuttle uses a **factory registry pattern**. Each transport:
1. Implements `adapter.ClientTransport` and/or `adapter.ServerTransport`
2. Registers an `adapter.TransportFactory` in `init()`
3. Is automatically discovered by the Engine at startup

## Step-by-Step

### 1. Create the package

```
transport/mytransport/
├── client.go       # ClientTransport implementation
├── server.go       # ServerTransport implementation
├── factory.go      # TransportFactory + init() registration
└── client_test.go  # Unit tests
```

### 2. Implement the interfaces

**Client** (`client.go`):
```go
package mytransport

import (
    "context"
    "github.com/ggshr9/shuttle/adapter"
)

type Client struct {
    // config fields
}

func (c *Client) Type() string { return "mytransport" }

func (c *Client) Dial(ctx context.Context, addr string) (adapter.Connection, error) {
    // Establish connection to server, return multiplexed connection
}

func (c *Client) Close() error {
    // Clean up resources
}

var _ adapter.ClientTransport = (*Client)(nil)
```

The `adapter.Connection` you return must implement:
- `OpenStream(ctx) (adapter.Stream, error)` — open a new multiplexed stream
- `AcceptStream(ctx) (adapter.Stream, error)` — accept incoming stream (server-side)
- `LocalAddr() / RemoteAddr()` — connection addresses
- `Close() error`

Each `adapter.Stream` must implement:
- `io.ReadWriteCloser`
- `StreamID() uint64`

### 3. Register the factory

**Factory** (`factory.go`):
```go
package mytransport

import (
    "github.com/ggshr9/shuttle/adapter"
    "github.com/ggshr9/shuttle/config"
)

func init() {
    adapter.Register(&factory{})
}

type factory struct{}

func (f *factory) Type() string { return "mytransport" }

func (f *factory) NewClient(cfg *config.ClientConfig, opts adapter.FactoryOptions) (adapter.ClientTransport, error) {
    if !cfg.Transport.MyTransport.Enabled {
        return nil, nil // nil means "not enabled", not an error
    }
    return &Client{/* init from cfg */}, nil
}

func (f *factory) NewServer(cfg *config.ServerConfig, opts adapter.FactoryOptions) (adapter.ServerTransport, error) {
    if !cfg.MyTransport.Enabled {
        return nil, nil
    }
    return &Server{/* init from cfg */}, nil
}
```

### 4. Add config fields

In `config/config.go`, add your transport's config section to `TransportConfig`:

```go
type TransportConfig struct {
    // ... existing fields ...
    MyTransport struct {
        Enabled bool   `yaml:"enabled"`
        // transport-specific fields
    } `yaml:"mytransport"`
}
```

### 5. Import for side effects

In `cmd/shuttle/main.go` and `cmd/shuttled/main.go`, add:

```go
import _ "github.com/ggshr9/shuttle/transport/mytransport"
```

This triggers `init()` which registers the factory.

### 6. Testing checklist

- [ ] Unit test: Client.Dial returns a working Connection
- [ ] Unit test: Connection.OpenStream returns a working Stream
- [ ] Unit test: Stream read/write round-trip
- [ ] Unit test: Factory returns nil when disabled in config
- [ ] Unit test: Factory returns Client when enabled in config
- [ ] Integration test (sandbox): full proxy round-trip through the transport

### 7. Conventions

- **Error wrapping**: Always use `fmt.Errorf("mytransport: %w", err)`
- **Logging**: Use `opts.Logger` from FactoryOptions, never create your own
- **Congestion control**: Access via `opts.CongestionControl` if your transport supports it
- **File size**: Keep files under 500 lines; split into client.go/server.go/conn.go if needed

## Existing transports for reference

| Transport | Package | Multiplexing | Key Feature |
|-----------|---------|-------------|-------------|
| H3 | `transport/h3/` | QUIC streams | Chrome TLS fingerprint |
| Reality | `transport/reality/` | yamux over TLS+Noise | SNI impersonation |
| CDN | `transport/cdn/` | gRPC/H2 streams | CDN passthrough |
| WebRTC | `transport/webrtc/` | yamux over DataChannel | NAT traversal |
