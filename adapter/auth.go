package adapter

import "net"

// Authenticator performs authentication on a connection.
// AuthClient sends credentials; AuthServer verifies and returns the user identity.
type Authenticator interface {
	AuthClient(conn net.Conn) error
	AuthServer(conn net.Conn) (user string, err error)
}
