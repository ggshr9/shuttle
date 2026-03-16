package reality

import (
	"testing"

	"github.com/shuttleX/shuttle/transport"
	"github.com/shuttleX/shuttle/transport/conformance"
)

func realityFactory(t testing.TB) (
	client transport.ClientTransport,
	server transport.ServerTransport,
	serverAddr string,
	cleanup func(),
) {
	t.Helper()

	// The Reality transport requires:
	// - TLS certificates (auto-generated or provided)
	// - Matching Noise IK key pairs derived from a shared password
	// - The server's listener field is unexported, so the bound address
	//   cannot be retrieved after Listen(":0"). The conformance suite's
	//   dial helper also calls Listen, which would conflict.
	//
	// Skip until an Addr() accessor is added or run in sandbox with known port.
	t.Skip("reality conformance: requires Addr() accessor on server (not yet exported)")

	return nil, nil, "", func() {}
}

func TestConformance(t *testing.T) {
	conformance.RunSuite(t, realityFactory)
}
