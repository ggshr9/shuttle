package adapter

import "sync"

// TransportFactory creates client and server transports.
// Each transport package registers its factory via init().
type TransportFactory interface {
	Type() string
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
