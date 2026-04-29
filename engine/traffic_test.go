package engine

import (
	"context"
	"log/slog"
	"net"
	"testing"

	"github.com/ggshr9/shuttle/config"
	"github.com/ggshr9/shuttle/router"
)

func TestTrafficManager_BuildBuiltinOutbounds(t *testing.T) {
	tm := NewTrafficManager(slog.Default())
	cfg := config.DefaultClientConfig()

	t.Run("without engine", func(t *testing.T) {
		outbounds := tm.BuildBuiltinOutbounds(cfg, nil)
		if _, ok := outbounds["direct"]; !ok {
			t.Error("expected 'direct' outbound to be present")
		}
		if _, ok := outbounds["reject"]; !ok {
			t.Error("expected 'reject' outbound to be present")
		}
		if _, ok := outbounds["proxy"]; ok {
			t.Error("expected 'proxy' outbound to be absent when eng is nil")
		}
	})

	t.Run("with engine", func(t *testing.T) {
		eng := &Engine{logger: slog.Default()}
		outbounds := tm.BuildBuiltinOutbounds(cfg, eng)
		if _, ok := outbounds["direct"]; !ok {
			t.Error("expected 'direct' outbound to be present")
		}
		if _, ok := outbounds["reject"]; !ok {
			t.Error("expected 'reject' outbound to be present")
		}
		if _, ok := outbounds["proxy"]; !ok {
			t.Error("expected 'proxy' outbound to be present when eng is not nil")
		}
	})
}

func TestTrafficManager_BuildInboundRouter(t *testing.T) {
	tm := NewTrafficManager(slog.Default())
	rt := router.NewRouter(&router.RouterConfig{
		DefaultAction: router.ActionDirect,
	}, router.NewGeoIPDB(), router.NewGeoSiteDB(), nil)
	dnsResolver := router.NewDNSResolver(&router.DNSConfig{}, router.NewGeoIPDB(), nil)

	outbounds := tm.BuildBuiltinOutbounds(config.DefaultClientConfig(), nil)
	ibRouter := tm.BuildInboundRouter(rt, dnsResolver, outbounds, outbounds["direct"])
	if ibRouter == nil {
		t.Fatal("expected non-nil InboundRouter")
	}
}

func TestTrafficManager_CreateDialer_Direct(t *testing.T) {
	// Start a local TCP listener to dial to.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// Accept one connection in a goroutine.
	accepted := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
		close(accepted)
	}()

	// Router that always returns ActionDirect.
	rt := router.NewRouter(&router.RouterConfig{
		DefaultAction: router.ActionDirect,
	}, router.NewGeoIPDB(), router.NewGeoSiteDB(), nil)
	dnsResolver := router.NewDNSResolver(&router.DNSConfig{}, router.NewGeoIPDB(), nil)
	cfg := config.DefaultClientConfig()

	tm := NewTrafficManager(slog.Default())
	dialer := tm.CreateDialer(cfg, rt, dnsResolver, func(ctx context.Context, serverAddr, addr, network string) (net.Conn, error) {
		t.Error("dialProxy should not be called for direct connections")
		return nil, nil
	})

	conn, err := dialer(context.Background(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("expected successful dial, got: %v", err)
	}
	conn.Close()
	<-accepted
}

func TestTrafficManager_CreateDialer_Reject(t *testing.T) {
	// Router that always returns ActionReject.
	rt := router.NewRouter(&router.RouterConfig{
		DefaultAction: router.ActionReject,
	}, router.NewGeoIPDB(), router.NewGeoSiteDB(), nil)
	dnsResolver := router.NewDNSResolver(&router.DNSConfig{}, router.NewGeoIPDB(), nil)
	cfg := config.DefaultClientConfig()

	tm := NewTrafficManager(slog.Default())
	dialer := tm.CreateDialer(cfg, rt, dnsResolver, func(ctx context.Context, serverAddr, addr, network string) (net.Conn, error) {
		t.Error("dialProxy should not be called for rejected connections")
		return nil, nil
	})

	conn, err := dialer(context.Background(), "tcp", "127.0.0.1:12345")
	if err == nil {
		t.Fatal("expected error for rejected connection")
	}
	if conn != nil {
		conn.Close()
		t.Fatal("expected nil conn for rejected connection")
	}
}
