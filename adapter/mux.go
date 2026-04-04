package adapter

import "net"

// Multiplexer creates a multiplexed Connection over a raw net.Conn.
type Multiplexer interface {
	Client(conn net.Conn) (Connection, error)
	Server(conn net.Conn) (Connection, error)
}
