package vnet

import (
	"context"
	"fmt"
	"net"
	"sync"
)

// edge represents a directed link between two nodes.
type edge struct {
	from, to *Node
	link     LinkConfig
}

// Network is the topology manager for the virtual network.
type Network struct {
	mu    sync.Mutex
	nodes map[string]*Node
	edges []edge // directed edges
	rng   *deterministicRand
	clock Clock
	links []*link // track for cleanup
}

// Option configures a Network.
type Option func(*Network)

// WithSeed sets the global RNG seed for deterministic behavior.
func WithSeed(seed int64) Option {
	return func(n *Network) {
		n.rng = newRand(seed)
	}
}

// New creates a new virtual network.
func New(opts ...Option) *Network {
	n := &Network{
		nodes: make(map[string]*Node),
		rng:   newRand(0),
		clock: RealClock{},
	}
	for _, o := range opts {
		o(n)
	}
	return n
}

// AddNode creates a named node in the network.
func (n *Network) AddNode(name string) *Node {
	n.mu.Lock()
	defer n.mu.Unlock()
	nd := newNode(name)
	n.nodes[name] = nd
	return nd
}

// Link creates a bidirectional link between two nodes with the same config.
func (n *Network) Link(a, b *Node, cfg LinkConfig) {
	n.LinkAsymmetric(a, b, cfg, cfg)
}

// LinkAsymmetric creates a bidirectional link with different configs per direction.
func (n *Network) LinkAsymmetric(a, b *Node, aToB, bToA LinkConfig) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.edges = append(n.edges,
		edge{from: a, to: b, link: aToB},
		edge{from: b, to: a, link: bToA},
	)
}

// Listen creates a listener on the given node at the specified address.
func (n *Network) Listen(node *Node, addr string) (net.Listener, error) {
	if node.getListener(addr) != nil {
		return nil, fmt.Errorf("address %s already in use on node %s", addr, node.Name)
	}
	l := newVirtualListener(node, addr)
	node.addListener(addr, l)
	return l, nil
}

// Dial connects from one node to a listener address on any reachable node.
func (n *Network) Dial(ctx context.Context, from *Node, toAddr string) (net.Conn, error) {
	n.mu.Lock()
	// Find a path from 'from' to a node that has a listener on toAddr.
	targetNode, listener := n.findListener(from, toAddr)
	if targetNode == nil || listener == nil {
		n.mu.Unlock()
		return nil, fmt.Errorf("no listener at address %q reachable from node %q", toAddr, from.Name)
	}

	// Get the link configs along the path.
	aToBCfg, bToACfg := n.findLinkConfigs(from, targetNode)
	n.mu.Unlock()

	// Check context before proceeding.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Create conditioned connection pair.
	aToBLnk := newLink(aToBCfg, n.clock, newRand(n.rng.childSeed()))
	bToALnk := newLink(bToACfg, n.clock, newRand(n.rng.childSeed()))

	n.mu.Lock()
	n.links = append(n.links, aToBLnk, bToALnk)
	n.mu.Unlock()

	clientConn, serverConn := newConditionedConnPair(from, targetNode, aToBLnk, bToALnk)

	if !listener.deliver(serverConn) {
		clientConn.Close()
		serverConn.Close()
		return nil, fmt.Errorf("listener at %q is closed", toAddr)
	}

	return clientConn, nil
}

// Close shuts down the network and all active links.
func (n *Network) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, l := range n.links {
		l.close()
	}
	n.links = nil
	return nil
}

// findListener searches for a listener at addr reachable from the given node.
// Must be called with n.mu held.
func (n *Network) findListener(from *Node, addr string) (*Node, *virtualListener) {
	// Direct: check if 'from' itself has the listener.
	if l := from.getListener(addr); l != nil {
		return from, l
	}

	// BFS over edges to find reachable nodes with the listener.
	visited := map[*Node]bool{from: true}
	queue := []*Node{from}
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		for _, e := range n.edges {
			if e.from == curr && !visited[e.to] {
				visited[e.to] = true
				if l := e.to.getListener(addr); l != nil {
					return e.to, l
				}
				queue = append(queue, e.to)
			}
		}
	}
	return nil, nil
}

// findLinkConfigs returns the link configs for direct link between two nodes.
// If no direct link exists (multi-hop), returns zero configs.
// Must be called with n.mu held.
func (n *Network) findLinkConfigs(from, to *Node) (aToB, bToA LinkConfig) {
	for _, e := range n.edges {
		if e.from == from && e.to == to {
			aToB = e.link
		}
		if e.from == to && e.to == from {
			bToA = e.link
		}
	}
	return
}
