package reality

import "net"

// peekConn wraps a net.Conn with a prepended byte buffer that is replayed
// on Read before reads fall through to the underlying conn. Used by the
// Reality server to "un-read" bytes consumed during PQ-vs-yamux detection.
//
// peekConn is not safe for concurrent use beyond what the underlying
// net.Conn already permits; callers must not Read from the conn outside
// the peekConn wrapper once wrapping is in effect.
type peekConn struct {
	net.Conn
	prefix []byte // bytes to replay first; emptied as Reads consume them
}

// Read drains the prefix buffer first, then falls through to the embedded
// net.Conn once the prefix is exhausted. Each Read call returns either
// prefix bytes or underlying-conn bytes, never a mix, matching
// net.Conn.Read semantics.
func (p *peekConn) Read(b []byte) (int, error) {
	if len(p.prefix) > 0 {
		n := copy(b, p.prefix)
		p.prefix = p.prefix[n:]
		if len(p.prefix) == 0 {
			p.prefix = nil
		}
		return n, nil
	}
	return p.Conn.Read(b)
}
