package webrtc

import (
	"testing"

	"github.com/shuttle-proxy/shuttle/transport"
	"github.com/shuttle-proxy/shuttle/transport/conformance"
)

func webrtcFactory(t testing.TB) (
	client transport.ClientTransport,
	server transport.ServerTransport,
	serverAddr string,
	cleanup func(),
) {
	t.Helper()

	// The WebRTC transport requires:
	// - A signaling server (HTTP or WebSocket) for SDP exchange
	// - STUN/TURN servers for ICE candidate gathering
	// - TLS certificates for the signaling endpoint
	//
	// Even with LoopbackOnly=true, the full WebRTC peer connection setup
	// involves async ICE negotiation that is not straightforward to wrap
	// in a synchronous factory.
	//
	// Skip until run in the sandbox environment with proper network setup.
	t.Skip("webrtc conformance: requires signaling server and ICE infrastructure")

	return nil, nil, "", func() {}
}

func TestConformance(t *testing.T) {
	conformance.RunSuite(t, webrtcFactory)
}
