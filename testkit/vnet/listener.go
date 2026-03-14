package vnet

import (
	"errors"
	"net"
	"sync"
)

// virtualListener implements net.Listener for the virtual network.
type virtualListener struct {
	addr     virtualAddr
	node     *Node
	incoming chan net.Conn
	closed   chan struct{}
	once     sync.Once
}

func newVirtualListener(node *Node, addr string) *virtualListener {
	return &virtualListener{
		addr:     virtualAddr{node: node.Name, addr: addr},
		node:     node,
		incoming: make(chan net.Conn, 64),
		closed:   make(chan struct{}),
	}
}

func (l *virtualListener) Accept() (net.Conn, error) {
	select {
	case conn, ok := <-l.incoming:
		if !ok {
			return nil, errors.New("listener closed")
		}
		return conn, nil
	case <-l.closed:
		return nil, errors.New("listener closed")
	}
}

func (l *virtualListener) Close() error {
	l.once.Do(func() {
		close(l.closed)
		l.node.removeListener(l.addr.addr)
	})
	return nil
}

func (l *virtualListener) Addr() net.Addr { return l.addr }

// deliver sends a connection to the listener's accept queue.
// Returns false if the listener is closed.
func (l *virtualListener) deliver(conn net.Conn) bool {
	select {
	case l.incoming <- conn:
		return true
	case <-l.closed:
		return false
	}
}
