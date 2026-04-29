package server

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/ggshr9/shuttle/proxy"
)

// HandleUDPRelay relays UDP datagrams between a transport stream and a remote
// UDP endpoint. It reads framed datagrams from the stream, sends them via UDP,
// and sends responses back as framed datagrams on the stream.
func HandleUDPRelay(ctx context.Context, stream io.ReadWriteCloser, target string, residual []byte, logger *slog.Logger) {
	udpAddr, err := net.ResolveUDPAddr("udp", target)
	if err != nil {
		logger.Debug("udp relay: resolve failed", "target", target, "err", err)
		return
	}
	udpConn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		logger.Debug("udp relay: dial failed", "target", target, "err", err)
		return
	}
	defer udpConn.Close()

	// If there are residual bytes from header parsing, prepend them for frame reading.
	var reader io.Reader = stream
	if len(residual) > 0 {
		reader = io.MultiReader(bytes.NewReader(residual), stream)
	}

	// Stream → UDP: read frames from stream, send as UDP datagrams.
	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			if ctx.Err() != nil {
				return
			}
			_, payload, err := proxy.ReadUDPFrame(reader)
			if err != nil {
				if err != io.EOF {
					logger.Debug("udp relay: read frame error", "err", err)
				}
				return
			}
			if _, err := udpConn.Write(payload); err != nil {
				logger.Debug("udp relay: udp write error", "err", err)
				return
			}
		}
	}()

	// UDP → Stream: read UDP responses, write as frames back on stream.
	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 65535)
		for {
			if ctx.Err() != nil {
				return
			}
			_ = udpConn.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, err := udpConn.Read(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// Idle timeout — no more responses expected.
					return
				}
				logger.Debug("udp relay: udp read error", "err", err)
				return
			}
			if err := proxy.WriteUDPFrame(stream, target, buf[:n]); err != nil {
				logger.Debug("udp relay: write frame error", "err", err)
				return
			}
		}
	}()

	<-done
	// Once one direction is done, close everything and wait for the other.
	udpConn.Close()
	stream.Close()
	<-done
}
