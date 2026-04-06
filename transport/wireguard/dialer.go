// Package wireguard implements a userspace WireGuard dialer using wireguard-go
// and gVisor netstack. This is client-only — no server/inbound handler.
package wireguard

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"sync"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

// TunnelConfig defines the WireGuard tunnel parameters.
type TunnelConfig struct {
	PrivateKey string         // WG private key (base64)
	Addresses  []netip.Prefix // local virtual IPs
	DNS        []netip.Addr   // DNS servers
	MTU        int            // default 1280
	Peers      []PeerConfig
}

// PeerConfig defines a WireGuard peer.
type PeerConfig struct {
	PublicKey    string         // base64
	Endpoint    string         // host:port
	AllowedIPs  []netip.Prefix // e.g. 0.0.0.0/0
	PresharedKey string        // optional, base64
	Keepalive   int            // seconds, 0 = disabled
}

// Dialer routes traffic through a userspace WireGuard tunnel.
type Dialer struct {
	tnet   *netstack.Net
	dev    *device.Device
	mu     sync.Mutex
	closed bool
	log    *slog.Logger
}

// NewDialer creates and starts a WireGuard tunnel from the given config.
func NewDialer(cfg TunnelConfig, logger *slog.Logger) (*Dialer, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if cfg.PrivateKey == "" {
		return nil, fmt.Errorf("wireguard: missing private_key")
	}
	if len(cfg.Peers) == 0 {
		return nil, fmt.Errorf("wireguard: at least one peer required")
	}
	if cfg.MTU <= 0 {
		cfg.MTU = 1280
	}

	// Build IPC config string.
	ipc, err := buildIPCConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("wireguard: build ipc config: %w", err)
	}

	// Extract addresses and DNS for netstack TUN.
	addrs := cfg.Addresses
	if len(addrs) == 0 {
		// Default to a link-local address if none specified.
		addrs = []netip.Prefix{netip.MustParsePrefix("10.0.0.2/32")}
	}
	dns := cfg.DNS

	// Extract IPs from prefixes for netstack (it wants []netip.Addr).
	tunAddrs := make([]netip.Addr, len(addrs))
	for i, p := range addrs {
		tunAddrs[i] = p.Addr()
	}

	// Create gVisor userspace TUN device.
	tun, tnet, err := netstack.CreateNetTUN(tunAddrs, dns, cfg.MTU)
	if err != nil {
		return nil, fmt.Errorf("wireguard: create netstack tun: %w", err)
	}

	// Create wireguard-go device with a quiet logger.
	lvl := device.LogLevelSilent
	if logger.Enabled(context.Background(), slog.LevelDebug) {
		lvl = device.LogLevelVerbose
	}
	devLogger := device.NewLogger(lvl, "(wireguard) ")

	dev := device.NewDevice(tun, conn.NewDefaultBind(), devLogger)

	// Apply IPC configuration.
	if err := dev.IpcSet(ipc); err != nil {
		dev.Close()
		return nil, fmt.Errorf("wireguard: ipc set: %w", err)
	}

	// Bring up the device.
	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("wireguard: device up: %w", err)
	}

	logger.Info("wireguard tunnel started",
		"addresses", addrs,
		"peers", len(cfg.Peers),
		"mtu", cfg.MTU,
	)

	return &Dialer{
		tnet: tnet,
		dev:  dev,
		log:  logger,
	}, nil
}

// DialContext dials through the WireGuard tunnel.
func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil, fmt.Errorf("wireguard: dialer closed")
	}
	d.mu.Unlock()

	switch network {
	case "tcp", "tcp4", "tcp6":
		ap, err := resolveAddrPort(address)
		if err != nil {
			return nil, fmt.Errorf("wireguard: resolve %q: %w", address, err)
		}
		return d.tnet.DialContextTCPAddrPort(ctx, ap)
	case "udp", "udp4", "udp6":
		ap, err := resolveAddrPort(address)
		if err != nil {
			return nil, fmt.Errorf("wireguard: resolve %q: %w", address, err)
		}
		return d.tnet.DialUDPAddrPort(netip.AddrPort{}, ap)
	default:
		return nil, fmt.Errorf("wireguard: unsupported network %q", network)
	}
}

// Type returns the transport type identifier.
func (d *Dialer) Type() string { return "wireguard" }

// Close shuts down the WireGuard tunnel.
func (d *Dialer) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	d.dev.Close()
	d.log.Info("wireguard tunnel closed")
	return nil
}

// buildIPCConfig converts TunnelConfig to WireGuard IPC format.
// WireGuard IPC uses hex-encoded keys, not base64.
func buildIPCConfig(cfg TunnelConfig) (string, error) {
	var b strings.Builder

	privHex, err := base64ToHex(cfg.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("private key: %w", err)
	}
	fmt.Fprintf(&b, "private_key=%s\n", privHex)

	for _, peer := range cfg.Peers {
		pubHex, err := base64ToHex(peer.PublicKey)
		if err != nil {
			return "", fmt.Errorf("peer public key: %w", err)
		}
		fmt.Fprintf(&b, "public_key=%s\n", pubHex)

		if peer.PresharedKey != "" {
			pskHex, err := base64ToHex(peer.PresharedKey)
			if err != nil {
				return "", fmt.Errorf("preshared key: %w", err)
			}
			fmt.Fprintf(&b, "preshared_key=%s\n", pskHex)
		}

		if peer.Endpoint != "" {
			fmt.Fprintf(&b, "endpoint=%s\n", peer.Endpoint)
		}

		for _, aip := range peer.AllowedIPs {
			fmt.Fprintf(&b, "allowed_ip=%s\n", aip.String())
		}

		if peer.Keepalive > 0 {
			fmt.Fprintf(&b, "persistent_keepalive_interval=%d\n", peer.Keepalive)
		}
	}

	return b.String(), nil
}

// base64ToHex decodes a base64-encoded key to hex string.
func base64ToHex(b64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("invalid base64: %w", err)
	}
	if len(raw) != 32 {
		return "", fmt.Errorf("key must be 32 bytes, got %d", len(raw))
	}
	return hex.EncodeToString(raw), nil
}

// resolveAddrPort parses a host:port string into netip.AddrPort.
func resolveAddrPort(address string) (netip.AddrPort, error) {
	// Try direct parse first (for IP addresses).
	if ap, err := netip.ParseAddrPort(address); err == nil {
		return ap, nil
	}

	// Fall back to net.ResolveTCPAddr for hostname resolution.
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("split host:port: %w", err)
	}

	// Try parsing host as IP directly.
	if addr, err := netip.ParseAddr(host); err == nil {
		p, err := net.LookupPort("tcp", port)
		if err != nil {
			return netip.AddrPort{}, err
		}
		return netip.AddrPortFrom(addr, uint16(p)), nil
	}

	// Resolve hostname.
	ips, err := net.DefaultResolver.LookupNetIP(context.Background(), "ip", host)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("resolve %q: %w", host, err)
	}
	if len(ips) == 0 {
		return netip.AddrPort{}, fmt.Errorf("no addresses for %q", host)
	}

	p, err := net.LookupPort("tcp", port)
	if err != nil {
		return netip.AddrPort{}, err
	}
	return netip.AddrPortFrom(ips[0], uint16(p)), nil
}
