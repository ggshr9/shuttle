package vnet

import (
	"net"
	"time"
)

// virtualConn wraps one end of a net.Pipe with virtual addressing.
type virtualConn struct {
	inner      net.Conn
	localAddr  net.Addr
	remoteAddr net.Addr
}

func (c *virtualConn) Read(b []byte) (int, error)         { return c.inner.Read(b) }
func (c *virtualConn) Write(b []byte) (int, error)        { return c.inner.Write(b) }
func (c *virtualConn) Close() error                       { return c.inner.Close() }
func (c *virtualConn) LocalAddr() net.Addr                { return c.localAddr }
func (c *virtualConn) RemoteAddr() net.Addr               { return c.remoteAddr }
func (c *virtualConn) SetDeadline(t time.Time) error      { return c.inner.SetDeadline(t) }
func (c *virtualConn) SetReadDeadline(t time.Time) error  { return c.inner.SetReadDeadline(t) }
func (c *virtualConn) SetWriteDeadline(t time.Time) error { return c.inner.SetWriteDeadline(t) }

// newConditionedConnPair creates a pair of net.Conns with link conditions applied.
//
// Architecture (A->B direction shown):
//
//	connA.Write -> appA_w -(pipe1)-> appA_r -> [aToBLink goroutine] -> appB_w -(pipe2)-> appB_r -> connB.Read
//
// The link goroutine applies latency, loss, jitter, and bandwidth shaping.
// The reverse direction (B->A) works symmetrically with bToALink.
func newConditionedConnPair(
	nodeA, nodeB *Node,
	aToBLink, bToALink *link,
) (net.Conn, net.Conn) {
	id := nextConnID()
	addrA := peerAddr(nodeA, id)
	addrB := peerAddr(nodeB, id)

	// A->B direction: A writes to pipe1, link reads pipe1 and writes to pipe2, B reads pipe2
	a2bRead, a2bWrite := net.Pipe() // A writes to a2bWrite; link reads from a2bRead
	b2aRead, b2aWrite := net.Pipe() // B writes to b2aWrite; link reads from b2aRead

	// Delivery pipes (post-conditioning)
	aDelivRead, aDelivWrite := net.Pipe() // bToA link writes to aDelivWrite; A reads from aDelivRead
	bDelivRead, bDelivWrite := net.Pipe() // aToB link writes to bDelivWrite; B reads from bDelivRead

	// Start condition goroutines
	go aToBLink.run(bDelivWrite, a2bRead)
	go bToALink.run(aDelivWrite, b2aRead)

	connA := &virtualConn{
		inner:      &duplexConn{reader: aDelivRead, writer: a2bWrite, otherWriter: aDelivWrite, otherReader: a2bRead},
		localAddr:  addrA,
		remoteAddr: addrB,
	}
	connB := &virtualConn{
		inner:      &duplexConn{reader: bDelivRead, writer: b2aWrite, otherWriter: bDelivWrite, otherReader: b2aRead},
		localAddr:  addrB,
		remoteAddr: addrA,
	}

	return connA, connB
}

// duplexConn combines separate read and write pipe ends into one conn.
// It also holds references to the "other" ends so Close can shut everything down.
type duplexConn struct {
	reader      net.Conn // read from this end
	writer      net.Conn // write to this end
	otherWriter net.Conn // the link goroutine's write end (our delivery pipe)
	otherReader net.Conn // the link goroutine's read end (our send pipe)
}

func (d *duplexConn) Read(b []byte) (int, error)  { return d.reader.Read(b) }
func (d *duplexConn) Write(b []byte) (int, error) { return d.writer.Write(b) }

func (d *duplexConn) Close() error {
	// Close our ends. This causes the link goroutines to get errors and exit.
	d.writer.Close()
	d.reader.Close()
	// Also close the other ends to unblock any stuck link goroutines.
	d.otherWriter.Close()
	d.otherReader.Close()
	return nil
}

func (d *duplexConn) LocalAddr() net.Addr  { return d.reader.LocalAddr() }
func (d *duplexConn) RemoteAddr() net.Addr { return d.reader.RemoteAddr() }

func (d *duplexConn) SetDeadline(t time.Time) error {
	e1 := d.reader.SetDeadline(t)
	e2 := d.writer.SetDeadline(t)
	if e1 != nil {
		return e1
	}
	return e2
}

func (d *duplexConn) SetReadDeadline(t time.Time) error  { return d.reader.SetReadDeadline(t) }
func (d *duplexConn) SetWriteDeadline(t time.Time) error { return d.writer.SetWriteDeadline(t) }
