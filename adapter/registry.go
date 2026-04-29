package adapter

import (
	"log/slog"
	"sync"

	"github.com/ggshr9/shuttle/config"
)

// TransportFactory creates client and server transports from config.
// Each transport package registers its factory via init().
// Factories return (nil, nil) when their transport is disabled in config.
type TransportFactory interface {
	Type() string
	NewClient(cfg *config.ClientConfig, opts FactoryOptions) (ClientTransport, error)
	NewServer(cfg *config.ServerConfig, opts FactoryOptions) (ServerTransport, error)
}

// FactoryOptions provides dependencies that factories may need.
type FactoryOptions struct {
	Logger            *slog.Logger
	CongestionControl interface{} // quic.CongestionControl — typed as interface{} to avoid quic-go import in adapter
}

var (
	registryMu sync.RWMutex
	registry   = map[string]TransportFactory{}
)

// Register adds a transport factory to the global registry.
func Register(f TransportFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[f.Type()] = f
}

// Get returns the factory for the given transport type, or nil.
func Get(name string) TransportFactory {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[name]
}

// All returns a copy of all registered factories.
func All() map[string]TransportFactory {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make(map[string]TransportFactory, len(registry))
	for k, v := range registry {
		out[k] = v
	}
	return out
}

// ResetRegistry clears all registered factories. For testing only.
func ResetRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = map[string]TransportFactory{}
}

// DialerFactory is optionally implemented by TransportFactory for per-request protocols.
// Existing multiplexed transport factories (h3, reality, cdn, webrtc) do not implement this.
type DialerFactory interface {
	NewDialer(cfg map[string]any, opts FactoryOptions) (Dialer, error)
	NewInboundHandler(cfg map[string]any, opts FactoryOptions) (InboundHandler, error)
}

// GetDialerFactory returns the DialerFactory for the given type, or nil if not supported.
func GetDialerFactory(typeName string) DialerFactory {
	f := Get(typeName)
	if f == nil {
		return nil
	}
	if df, ok := f.(DialerFactory); ok {
		return df
	}
	return nil
}
