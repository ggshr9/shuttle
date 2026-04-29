package auth_test

import (
	"net"
	"testing"

	"github.com/ggshr9/shuttle/adapter"
	"github.com/ggshr9/shuttle/transport/auth"
)

func TestHMACAuthenticator_RoundTrip(t *testing.T) {
	a := auth.NewHMACAuthenticator("test-password")
	var _ adapter.Authenticator = a

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	errCh := make(chan error, 1)
	go func() {
		_, err := a.AuthServer(serverConn)
		errCh <- err
	}()

	if err := a.AuthClient(clientConn); err != nil {
		t.Fatalf("AuthClient: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("AuthServer: %v", err)
	}
}

func TestHMACAuthenticator_WrongPassword(t *testing.T) {
	client := auth.NewHMACAuthenticator("correct-password")
	server := auth.NewHMACAuthenticator("wrong-password")

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	errCh := make(chan error, 1)
	go func() {
		_, err := server.AuthServer(serverConn)
		errCh <- err
	}()

	if err := client.AuthClient(clientConn); err != nil {
		t.Fatalf("AuthClient: %v", err)
	}
	if err := <-errCh; err == nil {
		t.Fatal("expected AuthServer to fail with wrong password")
	}
}
