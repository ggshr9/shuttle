package proxy

import (
	"testing"

	"github.com/shuttleX/shuttle/adapter"
)

func TestSOCKS5InboundFactory(t *testing.T) {
	adapter.ResetInboundRegistry()
	adapter.RegisterInbound(&SOCKS5InboundFactory{})

	f := adapter.GetInbound("socks5")
	if f == nil {
		t.Fatal("expected socks5 factory, got nil")
	}
	ib, err := f.Create("my-socks5", nil, adapter.InboundDeps{})
	if err != nil {
		t.Fatal(err)
	}
	if ib.Tag() != "my-socks5" {
		t.Errorf("Tag() = %q, want %q", ib.Tag(), "my-socks5")
	}
	if ib.Type() != "socks5" {
		t.Errorf("Type() = %q, want %q", ib.Type(), "socks5")
	}
}

func TestHTTPInboundFactory(t *testing.T) {
	adapter.ResetInboundRegistry()
	adapter.RegisterInbound(&HTTPInboundFactory{})

	f := adapter.GetInbound("http")
	if f == nil {
		t.Fatal("expected http factory, got nil")
	}
	ib, err := f.Create("my-http", nil, adapter.InboundDeps{})
	if err != nil {
		t.Fatal(err)
	}
	if ib.Tag() != "my-http" {
		t.Errorf("Tag() = %q, want %q", ib.Tag(), "my-http")
	}
	if ib.Type() != "http" {
		t.Errorf("Type() = %q, want %q", ib.Type(), "http")
	}
}

func TestSOCKS5InboundFactoryWithOptions(t *testing.T) {
	f := &SOCKS5InboundFactory{}
	opts := []byte(`{"listen":"127.0.0.1:9090","username":"user","password":"pass"}`)
	ib, err := f.Create("tagged", opts, adapter.InboundDeps{})
	if err != nil {
		t.Fatal(err)
	}
	s5, ok := ib.(*SOCKS5Inbound)
	if !ok {
		t.Fatal("expected *SOCKS5Inbound")
	}
	if s5.config.Listen != "127.0.0.1:9090" {
		t.Errorf("Listen = %q, want %q", s5.config.Listen, "127.0.0.1:9090")
	}
	if s5.config.Username != "user" {
		t.Errorf("Username = %q, want %q", s5.config.Username, "user")
	}
}

func TestHTTPInboundFactoryWithOptions(t *testing.T) {
	f := &HTTPInboundFactory{}
	opts := []byte(`{"listen":"127.0.0.1:8888","username":"u","password":"p"}`)
	ib, err := f.Create("tagged", opts, adapter.InboundDeps{})
	if err != nil {
		t.Fatal(err)
	}
	h, ok := ib.(*HTTPInbound)
	if !ok {
		t.Fatal("expected *HTTPInbound")
	}
	if h.config.Listen != "127.0.0.1:8888" {
		t.Errorf("Listen = %q, want %q", h.config.Listen, "127.0.0.1:8888")
	}
}
