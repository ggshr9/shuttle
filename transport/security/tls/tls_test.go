package tls_test

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"testing"
	"time"

	"github.com/ggshr9/shuttle/adapter"
	shuttlecrypto "github.com/ggshr9/shuttle/crypto"
	shuttletls "github.com/ggshr9/shuttle/transport/security/tls"
)

func TestTLSWrapper_ClientServer(t *testing.T) {
	certPEM, keyPEM, err := shuttlecrypto.GenerateSelfSignedCert(nil, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}

	wrapper := shuttletls.New(shuttletls.Config{
		ServerName:         "test.example.com",
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2"},
		MinVersion:         tls.VersionTLS13,
		ServerCert:         &cert,
	})
	var _ adapter.SecureWrapper = wrapper

	clientRaw, serverRaw := net.Pipe()
	defer clientRaw.Close()
	defer serverRaw.Close()

	errCh := make(chan error, 1)
	go func() {
		serverConn, err := wrapper.WrapServer(context.Background(), serverRaw)
		if err != nil {
			errCh <- err
			return
		}
		buf := make([]byte, 5)
		if _, err := io.ReadFull(serverConn, buf); err != nil {
			errCh <- err
			return
		}
		if _, err := serverConn.Write(buf); err != nil {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	clientConn, err := wrapper.WrapClient(context.Background(), clientRaw)
	if err != nil {
		t.Fatalf("WrapClient: %v", err)
	}
	if _, err := clientConn.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(clientConn, buf); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if string(buf) != "hello" {
		t.Fatalf("got %q, want %q", buf, "hello")
	}
	if err := <-errCh; err != nil {
		t.Fatalf("server error: %v", err)
	}
}
