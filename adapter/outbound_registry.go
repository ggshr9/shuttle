package adapter

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
)

// OutboundDeps provides dependencies that outbound factories may need.
type OutboundDeps struct {
	Logger *slog.Logger
}

// OutboundFactory creates Outbound instances from JSON config.
type OutboundFactory interface {
	Type() string
	Create(tag string, options json.RawMessage, deps OutboundDeps) (Outbound, error)
}

var (
	outboundMu        sync.RWMutex
	outboundFactories = map[string]OutboundFactory{}
)

// RegisterOutbound adds an outbound factory to the global registry.
func RegisterOutbound(f OutboundFactory) {
	outboundMu.Lock()
	defer outboundMu.Unlock()
	outboundFactories[f.Type()] = f
}

// GetOutbound returns the factory for the given outbound type, or nil.
func GetOutbound(name string) OutboundFactory {
	outboundMu.RLock()
	defer outboundMu.RUnlock()
	return outboundFactories[name]
}

// AllOutbounds returns a copy of all registered outbound factories.
func AllOutbounds() map[string]OutboundFactory {
	outboundMu.RLock()
	defer outboundMu.RUnlock()
	out := make(map[string]OutboundFactory, len(outboundFactories))
	for k, v := range outboundFactories {
		out[k] = v
	}
	return out
}

// CreateOutbound looks up the factory for typ and creates an Outbound.
func CreateOutbound(typ, tag string, opts json.RawMessage, deps OutboundDeps) (Outbound, error) {
	f := GetOutbound(typ)
	if f == nil {
		return nil, fmt.Errorf("unknown outbound type: %q", typ)
	}
	return f.Create(tag, opts, deps)
}

// ResetOutboundRegistry clears all registered outbound factories. For testing only.
func ResetOutboundRegistry() {
	outboundMu.Lock()
	defer outboundMu.Unlock()
	outboundFactories = map[string]OutboundFactory{}
}
