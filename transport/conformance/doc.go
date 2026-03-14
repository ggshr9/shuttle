// Package conformance provides a shared test suite that all transport
// implementations must pass. It exercises the transport.ClientTransport,
// transport.ServerTransport, transport.Connection, and transport.Stream
// interfaces with a battery of correctness, concurrency, and edge-case tests.
//
// Transport authors supply a TransportFactory that creates a matched
// client+server pair, then call RunSuite from a _test.go file:
//
//	func TestConformance(t *testing.T) {
//	    conformance.RunSuite(t, myFactory)
//	}
package conformance
