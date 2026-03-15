package vnet

import (
	"fmt"
	"net"
	"sync"
)

// Node represents a virtual network endpoint.
type Node struct {
	Name string

	mu        sync.Mutex
	listeners map[string]*virtualListener // addr -> listener
}

func newNode(name string) *Node {
	return &Node{
		Name:      name,
		listeners: make(map[string]*virtualListener),
	}
}

func (n *Node) addListener(addr string, l *virtualListener) {
	n.mu.Lock()
	n.listeners[addr] = l
	n.mu.Unlock()
}

func (n *Node) removeListener(addr string) {
	n.mu.Lock()
	delete(n.listeners, addr)
	n.mu.Unlock()
}

func (n *Node) getListener(addr string) *virtualListener {
	n.mu.Lock()
	l := n.listeners[addr]
	n.mu.Unlock()
	return l
}

// virtualAddr implements net.Addr for virtual network addresses.
type virtualAddr struct {
	node string
	addr string
}

func (a virtualAddr) Network() string { return "vnet" }
func (a virtualAddr) String() string  { return a.node + "/" + a.addr }

var connCounter uint64
var connCounterMu sync.Mutex

func nextConnID() uint64 {
	connCounterMu.Lock()
	connCounter++
	id := connCounter
	connCounterMu.Unlock()
	return id
}

func peerAddr(n *Node, id uint64) net.Addr {
	return virtualAddr{node: n.Name, addr: fmt.Sprintf("conn-%d", id)}
}
