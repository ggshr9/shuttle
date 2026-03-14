package vnet

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/shuttle-proxy/shuttle/testkit/observe"
)

// edge represents a directed link between two nodes.
type edge struct {
	from, to *Node
	link     LinkConfig
}

// Network is the topology manager for the virtual network.
type Network struct {
	mu       sync.Mutex
	nodes    map[string]*Node
	edges    []edge // directed edges
	rng      *deterministicRand
	clock    Clock
	links    []*link // track for cleanup
	recorder *observe.Recorder
}

// Option configures a Network.
type Option func(*Network)

// WithSeed sets the global RNG seed for deterministic behavior.
func WithSeed(seed int64) Option {
	return func(n *Network) {
		n.rng = newRand(seed)
	}
}

// WithClock sets the clock used by all links in the network.
// Use NewVirtualClock for deterministic, instant-advancing time in tests.
func WithClock(c Clock) Option {
	return func(n *Network) {
		n.clock = c
	}
}

// WithRecorder attaches an observe.Recorder for event logging.
func WithRecorder(r *observe.Recorder) Option {
	return func(n *Network) {
		n.recorder = r
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
	aToBLnk := newLink(aToBCfg, n.clock, newRand(n.rng.childSeed()), n.recorder, from, targetNode)
	bToALnk := newLink(bToACfg, n.clock, newRand(n.rng.childSeed()), n.recorder, targetNode, from)

	n.mu.Lock()
	n.links = append(n.links, aToBLnk, bToALnk)
	n.mu.Unlock()

	clientConn, serverConn := newConditionedConnPair(from, targetNode, aToBLnk, bToALnk)

	if !listener.deliver(serverConn) {
		clientConn.Close()
		serverConn.Close()
		return nil, fmt.Errorf("listener at %q is closed", toAddr)
	}

	if n.recorder != nil {
		n.recorder.RecordF("dial", from.Name, targetNode.Name, "addr=%s", toAddr)
	}

	return clientConn, nil
}

// UpdateLink changes link conditions for all active connections from a to b.
// New connections will also use the updated config via the edge table.
func (n *Network) UpdateLink(a, b *Node, cfg LinkConfig) {
	n.mu.Lock()
	defer n.mu.Unlock()
	// Update edge config for future connections.
	for i := range n.edges {
		if n.edges[i].from == a && n.edges[i].to == b {
			n.edges[i].link = cfg
		}
	}
	// Update active links.
	for _, l := range n.links {
		if l.fromNode == a && l.toNode == b {
			l.UpdateConfig(cfg)
		}
	}
	if n.recorder != nil {
		n.recorder.RecordF("link-update", a.Name, b.Name,
			"latency=%v loss=%.2f bw=%d", cfg.Latency, cfg.Loss, cfg.Bandwidth)
	}
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

// findLinkConfigs returns the accumulated link configs for a path between two nodes.
// For multi-hop paths, latency, jitter, and loss are accumulated along the path.
// Bandwidth is set to the minimum along the path (bottleneck).
// Must be called with n.mu held.
func (n *Network) findLinkConfigs(from, to *Node) (aToB, bToA LinkConfig) {
	// Check direct link first.
	directFound := false
	for _, e := range n.edges {
		if e.from == from && e.to == to {
			aToB = e.link
			directFound = true
		}
		if e.from == to && e.to == from {
			bToA = e.link
		}
	}
	if directFound {
		return
	}

	// Multi-hop: BFS to find path, then accumulate configs.
	pathForward := n.findPath(from, to)
	pathReverse := n.findPath(to, from)
	if pathForward != nil {
		aToB = n.accumulateConfigs(pathForward)
	}
	if pathReverse != nil {
		bToA = n.accumulateConfigs(pathReverse)
	}
	return
}

// findPath returns the sequence of edges from src to dst using BFS.
// Must be called with n.mu held.
func (n *Network) findPath(src, dst *Node) []edge {
	type pathEntry struct {
		node *Node
		path []edge
	}
	visited := map[*Node]bool{src: true}
	queue := []pathEntry{{node: src}}
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		for _, e := range n.edges {
			if e.from == curr.node && !visited[e.to] {
				newPath := make([]edge, len(curr.path)+1)
				copy(newPath, curr.path)
				newPath[len(curr.path)] = e
				if e.to == dst {
					return newPath
				}
				visited[e.to] = true
				queue = append(queue, pathEntry{node: e.to, path: newPath})
			}
		}
	}
	return nil
}

// accumulateConfigs combines multiple link configs along a path.
// Latency and jitter are summed, loss is combined probabilistically,
// and bandwidth is the minimum (bottleneck).
func (n *Network) accumulateConfigs(path []edge) LinkConfig {
	if len(path) == 0 {
		return LinkConfig{}
	}
	acc := path[0].link
	for _, e := range path[1:] {
		acc.Latency += e.link.Latency
		acc.Jitter += e.link.Jitter
		// P(no loss) = product of (1 - loss_i)
		acc.Loss = 1.0 - (1.0-acc.Loss)*(1.0-e.link.Loss)
		if e.link.Bandwidth > 0 {
			if acc.Bandwidth == 0 || e.link.Bandwidth < acc.Bandwidth {
				acc.Bandwidth = e.link.Bandwidth
			}
		}
		// Reorder: take max probability
		if e.link.ReorderPct > acc.ReorderPct {
			acc.ReorderPct = e.link.ReorderPct
		}
		if e.link.ReorderDelay > acc.ReorderDelay {
			acc.ReorderDelay = e.link.ReorderDelay
		}
	}
	return acc
}
