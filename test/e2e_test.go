package test

import (
	"context"
	"net"
	"testing"

	"github.com/shuttle-proxy/shuttle/proxy"
)

func TestSOCKS5ServerStartStop(t *testing.T) {
	dialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.Dial(network, addr)
	}

	srv := proxy.NewSOCKS5Server(&proxy.SOCKS5Config{
		ListenAddr: "127.0.0.1:0",
	}, dialer, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Find a free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	srv2 := proxy.NewSOCKS5Server(&proxy.SOCKS5Config{
		ListenAddr: addr,
	}, dialer, nil)

	if err := srv2.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Verify it's listening
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("connect to socks5: %v", err)
	}
	conn.Close()

	srv2.Close()
	_ = srv // avoid unused
}

func TestHTTPProxyStartStop(t *testing.T) {
	dialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.Dial(network, addr)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	srv := proxy.NewHTTPServer(&proxy.HTTPConfig{
		ListenAddr: addr,
	}, dialer, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("connect to http proxy: %v", err)
	}
	conn.Close()

	srv.Close()
}
