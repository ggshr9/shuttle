package plugin

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// Plugin is the interface for middleware plugins.
type Plugin interface {
	Name() string
	Init(ctx context.Context) error
	Close() error
}

// ConnPlugin can intercept connections.
type ConnPlugin interface {
	Plugin
	OnConnect(conn net.Conn, target string) (net.Conn, error)
	OnDisconnect(conn net.Conn)
}

// DataPlugin can inspect/modify data.
type DataPlugin interface {
	Plugin
	OnData(data []byte, direction Direction) []byte
}

// Direction indicates data flow direction.
type Direction int

const (
	Inbound  Direction = iota
	Outbound
)

// Chain manages an ordered list of plugins.
type Chain struct {
	plugins []Plugin
}

// NewChain creates a new plugin chain.
func NewChain(plugins ...Plugin) *Chain {
	return &Chain{plugins: plugins}
}

// Init initializes all plugins in order.
// If any plugin fails to initialize, already-initialized plugins are closed.
func (c *Chain) Init(ctx context.Context) error {
	for i, p := range c.plugins {
		if err := p.Init(ctx); err != nil {
			// Close already-initialized plugins in reverse order
			for j := i - 1; j >= 0; j-- {
				c.plugins[j].Close()
			}
			return fmt.Errorf("plugin %s init: %w", p.Name(), err)
		}
	}
	return nil
}

// Close closes all plugins in reverse order.
// All plugins are closed even if some return errors; errors are aggregated.
func (c *Chain) Close() error {
	var errs []error
	for i := len(c.plugins) - 1; i >= 0; i-- {
		if err := c.plugins[i].Close(); err != nil {
			errs = append(errs, fmt.Errorf("plugin %s close: %w", c.plugins[i].Name(), err))
		}
	}
	return errors.Join(errs...)
}

// OnConnect runs the connection through all ConnPlugin instances in order.
// If any plugin returns an error, the connection is rejected.
// The returned net.Conn may be wrapped (e.g. for byte tracking).
func (c *Chain) OnConnect(conn net.Conn, target string) (net.Conn, error) {
	for _, p := range c.plugins {
		if cp, ok := p.(ConnPlugin); ok {
			var err error
			conn, err = cp.OnConnect(conn, target)
			if err != nil {
				return nil, fmt.Errorf("plugin %s: %w", p.Name(), err)
			}
		}
	}
	return conn, nil
}

// OnDisconnect notifies all ConnPlugin instances in reverse order.
func (c *Chain) OnDisconnect(conn net.Conn) {
	for i := len(c.plugins) - 1; i >= 0; i-- {
		if cp, ok := c.plugins[i].(ConnPlugin); ok {
			cp.OnDisconnect(conn)
		}
	}
}
