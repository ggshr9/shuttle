package wireguard

import (
	"fmt"
	"net"
	"net/netip"

	"github.com/ggshr9/shuttle/adapter"
	"github.com/ggshr9/shuttle/config"
)

func init() {
	adapter.Register(&factory{})
}

type factory struct{}

func (f *factory) Type() string { return "wireguard" }

// WireGuard doesn't use multiplexed transport — NewClient/NewServer return nil.
func (f *factory) NewClient(_ *config.ClientConfig, _ adapter.FactoryOptions) (adapter.ClientTransport, error) {
	return nil, nil
}

func (f *factory) NewServer(_ *config.ServerConfig, _ adapter.FactoryOptions) (adapter.ServerTransport, error) {
	return nil, nil
}

// NewDialer implements adapter.DialerFactory.
func (f *factory) NewDialer(cfg map[string]any, opts adapter.FactoryOptions) (adapter.Dialer, error) {
	tc, err := parseTunnelConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("wireguard factory: %w", err)
	}

	d, err := NewDialer(&tc, opts.Logger)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// NewInboundHandler returns nil — WireGuard is client-only.
func (f *factory) NewInboundHandler(_ map[string]any, _ adapter.FactoryOptions) (adapter.InboundHandler, error) {
	return nil, nil
}

// parseTunnelConfig converts a generic map to TunnelConfig.
func parseTunnelConfig(cfg map[string]any) (TunnelConfig, error) {
	tc := TunnelConfig{
		MTU: 1280,
	}

	if pk, ok := cfg["private_key"].(string); ok && pk != "" {
		tc.PrivateKey = pk
	} else {
		return tc, fmt.Errorf("missing private_key")
	}

	// Parse addresses.
	if addrs, ok := cfg["addresses"].([]any); ok {
		for _, a := range addrs {
			s, ok := a.(string)
			if !ok {
				continue
			}
			prefix, err := netip.ParsePrefix(s)
			if err != nil {
				return tc, fmt.Errorf("invalid address %q: %w", s, err)
			}
			tc.Addresses = append(tc.Addresses, prefix)
		}
	}

	// Parse DNS.
	if dns, ok := cfg["dns"].([]any); ok {
		for _, d := range dns {
			s, ok := d.(string)
			if !ok {
				continue
			}
			addr, err := netip.ParseAddr(s)
			if err != nil {
				return tc, fmt.Errorf("invalid dns %q: %w", s, err)
			}
			tc.DNS = append(tc.DNS, addr)
		}
	}

	if mtu, ok := cfg["mtu"].(int); ok && mtu > 0 {
		tc.MTU = mtu
	}

	// Parse peers.
	if peers, ok := cfg["peers"].([]any); ok {
		for _, p := range peers {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			pc, err := parsePeerConfig(pm)
			if err != nil {
				return tc, err
			}
			tc.Peers = append(tc.Peers, pc)
		}
	}

	if len(tc.Peers) == 0 {
		return tc, fmt.Errorf("at least one peer required")
	}

	return tc, nil
}

func parsePeerConfig(m map[string]any) (PeerConfig, error) {
	pc := PeerConfig{}

	if pk, ok := m["public_key"].(string); ok && pk != "" {
		pc.PublicKey = pk
	} else {
		return pc, fmt.Errorf("peer missing public_key")
	}

	if ep, ok := m["endpoint"].(string); ok {
		if _, _, err := net.SplitHostPort(ep); err != nil {
			return pc, fmt.Errorf("invalid peer endpoint %q: %w", ep, err)
		}
		pc.Endpoint = ep
	}

	if aips, ok := m["allowed_ips"].([]any); ok {
		for _, a := range aips {
			s, ok := a.(string)
			if !ok {
				continue
			}
			prefix, err := netip.ParsePrefix(s)
			if err != nil {
				return pc, fmt.Errorf("invalid allowed_ip %q: %w", s, err)
			}
			pc.AllowedIPs = append(pc.AllowedIPs, prefix)
		}
	}

	if psk, ok := m["preshared_key"].(string); ok {
		pc.PresharedKey = psk
	}

	if ka, ok := m["keepalive"].(int); ok {
		pc.Keepalive = ka
	}

	return pc, nil
}
