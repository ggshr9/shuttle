package adapter

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
)

// InboundDeps provides dependencies that inbound factories may need.
type InboundDeps struct {
	Logger *slog.Logger
}

// InboundFactory creates Inbound instances from JSON config.
type InboundFactory interface {
	Type() string
	Create(tag string, options json.RawMessage, deps InboundDeps) (Inbound, error)
}

var (
	inboundMu        sync.RWMutex
	inboundFactories = map[string]InboundFactory{}
)

// RegisterInbound adds an inbound factory to the global registry.
func RegisterInbound(f InboundFactory) {
	inboundMu.Lock()
	defer inboundMu.Unlock()
	inboundFactories[f.Type()] = f
}

// GetInbound returns the factory for the given inbound type, or nil.
func GetInbound(name string) InboundFactory {
	inboundMu.RLock()
	defer inboundMu.RUnlock()
	return inboundFactories[name]
}

// AllInbounds returns a copy of all registered inbound factories.
func AllInbounds() map[string]InboundFactory {
	inboundMu.RLock()
	defer inboundMu.RUnlock()
	out := make(map[string]InboundFactory, len(inboundFactories))
	for k, v := range inboundFactories {
		out[k] = v
	}
	return out
}

// CreateInbound looks up the factory for typ and creates an Inbound.
func CreateInbound(typ, tag string, opts json.RawMessage, deps InboundDeps) (Inbound, error) {
	f := GetInbound(typ)
	if f == nil {
		return nil, fmt.Errorf("unknown inbound type: %q", typ)
	}
	return f.Create(tag, opts, deps)
}

// ResetInboundRegistry clears all registered inbound factories. For testing only.
func ResetInboundRegistry() {
	inboundMu.Lock()
	defer inboundMu.Unlock()
	inboundFactories = map[string]InboundFactory{}
}
