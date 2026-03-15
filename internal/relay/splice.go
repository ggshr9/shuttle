package relay

import (
	"io"
	"syscall"
)

// CanSplice returns true if both sides support zero-copy relay.
// On Linux, when both sides are *net.TCPConn (or implement syscall.Conn),
// Go's io.Copy will use splice(2) to transfer data without copying
// through userspace.
func CanSplice(a, b io.ReadWriteCloser) bool {
	_, aOK := a.(syscall.Conn)
	_, bOK := b.(syscall.Conn)
	return aOK && bOK
}
