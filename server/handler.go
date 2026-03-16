package server

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shuttleX/shuttle/internal/relay"
	"github.com/shuttleX/shuttle/mesh"
	meshsignal "github.com/shuttleX/shuttle/mesh/signal"
	"github.com/shuttleX/shuttle/proxy"
	"github.com/shuttleX/shuttle/server/admin"
	"github.com/shuttleX/shuttle/server/audit"
	"github.com/shuttleX/shuttle/server/metrics"
	"github.com/shuttleX/shuttle/transport"
)

// Handler contains the server's business logic for handling incoming
// connections, streams, mesh traffic, and UDP relay.
type Handler struct {
	Users      *admin.UserStore
	Reputation *Reputation
	AuditLog   *audit.Logger
	PeerTable  *mesh.PeerTable
	Allocator  *mesh.IPAllocator
	SignalHub  *meshsignal.Hub
	Metrics    *metrics.Collector
	AdminInfo  *admin.ServerInfo
	StreamSem  chan struct{}
	Logger     *slog.Logger
}

// HandleConnection accepts streams from a multiplexed connection and
// dispatches each to HandleStream. It runs until the connection is closed
// or the context is cancelled.
func (h *Handler) HandleConnection(ctx context.Context, conn transport.Connection) {
	defer conn.Close()

	remoteIP := ExtractIP(conn.RemoteAddr())

	for {
		stream, err := conn.AcceptStream(ctx)
		if err != nil {
			if ctx.Err() == nil {
				h.Logger.Debug("accept stream error", "err", err)
			}
			return
		}

		select {
		case h.StreamSem <- struct{}{}:
		default:
			h.Logger.Warn("stream limit reached, rejecting")
			stream.Close()
			continue
		}

		h.Metrics.StreamOpened()
		go func() {
			defer func() {
				<-h.StreamSem
				h.Metrics.StreamClosed()
			}()
			h.HandleStream(ctx, stream, remoteIP)
		}()
	}
}

// HandleStream reads the target header from a stream, authenticates the
// user (if configured), checks SSRF protection, and relays traffic to
// the destination or dispatches to mesh/UDP handlers.
func (h *Handler) HandleStream(ctx context.Context, stream transport.Stream, remoteIP string) {
	defer stream.Close()

	// Read target address (first line). Use a buffered approach to avoid
	// losing bytes that come after the \n delimiter in the same read.
	// Set a deadline so a slow/malicious client cannot hold a goroutine forever.
	if dl, ok := stream.(interface{ SetReadDeadline(time.Time) error }); ok {
		dl.SetReadDeadline(time.Now().Add(10 * time.Second))
	}
	buf := make([]byte, 512)
	total := 0
	for {
		n, err := stream.Read(buf[total:])
		if err != nil {
			return
		}
		total += n
		// Look for newline in what we've read so far.
		if idx := bytes.IndexByte(buf[:total], '\n'); idx >= 0 {
			header := string(buf[:idx])

			// Parse header: "TOKEN:target" or just "target" (backward compatible).
			// Tokens are 64-char hex strings, so we look for the first colon
			// and try to authenticate the prefix. If it fails (or no users
			// configured), treat the entire header as the target address.
			var target string
			var user *admin.UserState
			if h.Users != nil && h.Users.HasUsers() {
				if colonIdx := strings.IndexByte(header, ':'); colonIdx > 0 {
					token := header[:colonIdx]
					if u := h.Users.Authenticate(token); u != nil {
						user = u
						target = header[colonIdx+1:]
						// Record successful auth for reputation
						if h.Reputation != nil {
							h.Reputation.RecordSuccess(remoteIP)
						}
					} else {
						// Token didn't match — reject the stream
						if h.Reputation != nil {
							if h.Reputation.RecordFailure(remoteIP) {
								h.Logger.Warn("IP banned after auth failures", "ip", remoteIP)
							}
						}
						h.Logger.Debug("auth failed, rejecting stream", "ip", remoteIP)
						h.Metrics.RecordAuthFailure()
						return
					}
				} else {
					// No token provided but users are configured — reject
					h.Logger.Debug("no auth token provided, rejecting stream", "ip", remoteIP)
					h.Metrics.RecordAuthFailure()
					if h.Reputation != nil {
						if h.Reputation.RecordFailure(remoteIP) {
							h.Logger.Warn("IP banned after auth failures", "ip", remoteIP)
						}
					}
					return
				}
			} else {
				target = header
			}

			// If user authenticated, enforce quota.
			if user != nil && user.QuotaExceeded() {
				h.Logger.Info("quota exceeded, rejecting stream", "user", user.Name)
				return
			}

			// Check for mesh magic
			if target == "MESH" && h.PeerTable != nil && h.Allocator != nil {
				h.HandleMeshStream(ctx, stream)
				return
			}

			residual := buf[idx+1 : total] // bytes after \n

			// Clear the header-read deadline before relaying data.
			if dl, ok := stream.(interface{ SetReadDeadline(time.Time) error }); ok {
				dl.SetReadDeadline(time.Time{})
			}

			// Check for UDP prefix: "UDP:target" → UDP relay mode.
			isUDP := false
			if strings.HasPrefix(target, proxy.UDPStreamPrefix) {
				target = strings.TrimPrefix(target, proxy.UDPStreamPrefix)
				isUDP = true
			}

			h.Logger.Debug("proxying", "target", target, "udp", isUDP)

			// SSRF protection: block connections to internal/private networks
			if IsBlockedTarget(target) {
				h.Logger.Warn("blocked SSRF attempt to internal target", "target", target, "ip", remoteIP)
				return
			}

			if isUDP {
				// UDP relay mode: read framed datagrams from stream, send as UDP.
				HandleUDPRelay(ctx, stream, target, residual, h.Logger)
				return
			}

			remote, err := net.DialTimeout("tcp", target, 10*time.Second)
			if err != nil {
				h.Logger.Debug("dial target failed", "target", target, "err", err)
				return
			}
			defer remote.Close() //nolint:gocritic // not a real loop; reads until newline then returns

			// Forward any residual bytes that were read past the header.
			if len(residual) > 0 {
				if _, err := remote.Write(residual); err != nil {
					return
				}
			}

			// If user authenticated, wrap stream with byte counting and
			// track active connections.
			var rw io.ReadWriter = stream
			var counter *countingReadWriter
			if user != nil {
				user.ActiveConns.Add(1)
				defer user.ActiveConns.Add(-1) //nolint:gocritic // not a real loop; reads until newline then returns
				counter = &countingReadWriter{
					inner: stream,
					user:  user,
				}
				rw = counter
			}

			// Relay bidirectionally.
			startTime := time.Now()
			ServerRelay(rw, remote)

			// Record bytes in metrics collector
			if counter != nil {
				h.Metrics.RecordBytes(counter.bytesIn.Load(), counter.bytesOut.Load())
			}

			// Record audit entry after relay completes.
			if h.AuditLog != nil {
				entry := audit.Entry{
					Timestamp:  startTime,
					Target:     target,
					DurationMs: time.Since(startTime).Milliseconds(),
				}
				if user != nil {
					entry.User = user.Name
				}
				if counter != nil {
					entry.BytesIn = counter.bytesIn.Load()
					entry.BytesOut = counter.bytesOut.Load()
				}
				if h.AdminInfo != nil {
					h.AdminInfo.BytesSent.Add(entry.BytesOut)
					h.AdminInfo.BytesRecv.Add(entry.BytesIn)
				}
				h.AuditLog.Log(&entry)
			}
			return
		}
		if total >= len(buf) {
			h.Logger.Debug("target header too long")
			return
		}
	}
}

// HandleMeshStream handles a mesh VPN stream: allocates a virtual IP,
// registers the peer, and forwards mesh frames between peers.
func (h *Handler) HandleMeshStream(ctx context.Context, stream transport.Stream) {
	ip, err := h.Allocator.Allocate()
	if err != nil {
		h.Logger.Error("mesh: IP allocation failed", "err", err)
		return
	}
	defer h.Allocator.Release(ip)

	// Send handshake: IP + mask + gateway
	handshake := mesh.EncodeHandshake(ip, h.Allocator.Mask(), h.Allocator.Gateway())
	if _, err := stream.Write(handshake); err != nil {
		h.Logger.Error("mesh: handshake write failed", "err", err)
		return
	}

	// Register peer with a frame-writing wrapper
	fw := &meshFrameWriter{stream: stream}
	h.PeerTable.Register(ip, fw)
	defer h.PeerTable.Unregister(ip)

	// Register peer with signal hub if P2P is enabled
	if h.SignalHub != nil {
		h.SignalHub.Register(ip, fw)
		defer h.SignalHub.Unregister(ip)
	}

	h.Logger.Info("mesh peer connected", "ip", ip)

	// Read frames from this peer and forward to destination peers
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		pkt, err := mesh.ReadFrame(stream)
		if err != nil {
			h.Logger.Debug("mesh peer disconnected", "ip", ip, "err", err)
			return
		}

		// Check if this is a signaling message
		if h.SignalHub != nil && isSignalingPacket(pkt) {
			if err := h.SignalHub.HandleMessage(pkt, ip); err != nil {
				h.Logger.Debug("mesh: signal handling failed", "err", err)
			}
			continue
		}

		// Regular mesh packet - forward to destination
		if !h.PeerTable.Forward(pkt) {
			h.Logger.Debug("mesh: no route for packet", "src", ip)
		}
	}
}

// countingReadWriter wraps an io.ReadWriter and updates a user's byte counters.
// It also tracks per-stream byte counts for audit logging.
type countingReadWriter struct {
	inner    io.ReadWriter
	user     *admin.UserState
	bytesIn  atomic.Int64 // bytes read (client → server)
	bytesOut atomic.Int64 // bytes written (server → client)
}

func (c *countingReadWriter) Read(p []byte) (int, error) {
	n, err := c.inner.Read(p)
	if n > 0 {
		c.bytesIn.Add(int64(n))
		c.user.BytesRecv.Add(int64(n))
	}
	return n, err
}

func (c *countingReadWriter) Write(p []byte) (int, error) {
	n, err := c.inner.Write(p)
	if n > 0 {
		c.bytesOut.Add(int64(n))
		c.user.BytesSent.Add(int64(n))
	}
	return n, err
}

// isSignalingPacket checks if a packet is a signaling message.
// Signaling messages have a specific format starting with a type byte
// in the range 0x01-0xFF (non-IP packet).
func isSignalingPacket(pkt []byte) bool {
	if len(pkt) < meshsignal.HeaderSize {
		return false
	}
	// Check if it looks like a signaling message by checking the type
	// Valid signaling types are 0x01-0x08 and 0xFF
	msgType := pkt[0]
	switch msgType {
	case meshsignal.SignalCandidate,
		meshsignal.SignalConnect,
		meshsignal.SignalConnectAck,
		meshsignal.SignalDisconnect,
		meshsignal.SignalPing,
		meshsignal.SignalPong,
		meshsignal.SignalError:
		return true
	default:
		// Check if it starts with IPv4 version (0x4X)
		// If not, it might be a signaling message
		return (pkt[0] >> 4) != 4
	}
}

// meshFrameWriter wraps a stream to write length-prefixed frames.
type meshFrameWriter struct {
	mu     sync.Mutex
	stream io.WriteCloser
}

func (fw *meshFrameWriter) Write(p []byte) (int, error) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if err := mesh.WriteFrame(fw.stream, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (fw *meshFrameWriter) Close() error {
	return fw.stream.Close()
}

// ExtractIP extracts the IP address (without port) from a net.Addr.
func ExtractIP(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	s := addr.String()
	host, _, err := net.SplitHostPort(s)
	if err != nil {
		// Try parsing as bare IP (no port)
		if ip := net.ParseIP(s); ip != nil {
			return ip.String()
		}
		return s
	}
	return host
}

// ServerRelay copies data bidirectionally using the shared relay package,
// which supports zero-copy (splice) when both sides are raw TCP connections.
// It wraps io.ReadWriter arguments into io.ReadWriteCloser when necessary.
func ServerRelay(a io.ReadWriter, b io.ReadWriter) {
	rwcA := asReadWriteCloser(a)
	rwcB := asReadWriteCloser(b)
	relay.Relay(rwcA, rwcB)
}

// asReadWriteCloser returns the value unchanged if it already implements
// io.ReadWriteCloser, otherwise wraps it with a no-op Close.
func asReadWriteCloser(rw io.ReadWriter) io.ReadWriteCloser {
	if rwc, ok := rw.(io.ReadWriteCloser); ok {
		return rwc
	}
	return readWriteNopCloser{rw}
}

type readWriteNopCloser struct {
	io.ReadWriter
}

func (readWriteNopCloser) Close() error { return nil }
