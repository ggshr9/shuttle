package h3

import (
	"testing"

	"github.com/shuttle-proxy/shuttle/transport"
	"github.com/shuttle-proxy/shuttle/transport/conformance"
)

func h3Factory(t testing.TB) (
	client transport.ClientTransport,
	server transport.ServerTransport,
	serverAddr string,
	cleanup func(),
) {
	t.Helper()

	// The H3 transport requires:
	// - TLS certificates for the QUIC listener
	// - A custom auth handshake (HMAC on a control stream) before data streams
	// - The conformance suite opens streams directly, but H3 expects the first
	//   stream to be the auth control stream.
	//
	// A proper factory would generate a self-signed cert, start the server,
	// and configure the client with InsecureSkipVerify. However, the server's
	// Listen address is not easily retrievable (listener is unexported), and
	// the auth protocol needs special handling.
	//
	// Skip until run in the sandbox environment or an adapter layer is added.
	t.Skip("h3 conformance: requires QUIC/TLS setup and auth protocol adapter")

	return nil, nil, "", func() {}
}

func TestConformance(t *testing.T) {
	conformance.RunSuite(t, h3Factory)
}
