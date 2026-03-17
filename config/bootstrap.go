package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	shuttleCrypto "github.com/shuttleX/shuttle/crypto"
)

// InitOptions configures the bootstrap process.
type InitOptions struct {
	ConfigDir  string   // default: /etc/shuttle (Linux root) or ~/.shuttle
	Listen     string   // default: ":443"
	Domain     string   // optional; auto-detects public IP if empty
	Password   string   // auto-generated if empty
	Transports []string // default: ["h3", "reality"]
	TargetSNI  string   // default: "www.microsoft.com"
	Force      bool     // overwrite existing config
	Mesh       bool     // enable mesh VPN with P2P
}

// InitResult contains the output of a successful bootstrap.
type InitResult struct {
	ConfigPath string
	ShareURI   string
	Password   string
	PublicKey  string
	ServerAddr string
	AdminToken string
	MeshEnabled bool
	MeshCIDR    string
}

// Bootstrap generates all server prerequisites and writes config to disk.
func Bootstrap(opts *InitOptions) (*InitResult, error) {
	if opts == nil {
		opts = &InitOptions{}
	}
	applyInitDefaults(opts)

	configPath := filepath.Join(opts.ConfigDir, "server.yaml")
	certPath := filepath.Join(opts.ConfigDir, "cert.pem")
	keyPath := filepath.Join(opts.ConfigDir, "key.pem")

	// Check existing config
	if !opts.Force {
		if _, err := os.Stat(configPath); err == nil {
			return nil, fmt.Errorf("config already exists at %s (use --force to overwrite)", configPath)
		}
	}

	// Ensure config directory
	if err := os.MkdirAll(opts.ConfigDir, 0700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}

	// Generate password
	password := opts.Password
	if password == "" {
		password = randomHex(16)
	}

	// Generate Curve25519 key pair
	pub, priv, err := shuttleCrypto.GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("generate key pair: %w", err)
	}
	pubHex := hex.EncodeToString(pub[:])
	privHex := hex.EncodeToString(priv[:])

	// Generate Reality short ID
	shortID := randomHex(8)

	// Generate admin token
	adminToken := randomHex(32)

	// Determine server address
	serverAddr := opts.Domain
	if serverAddr == "" {
		if ip := detectPublicIP(); ip != "" {
			serverAddr = ip
		} else {
			serverAddr = detectLocalIP()
		}
	}

	// Generate self-signed TLS certificate
	hosts := []string{serverAddr}
	if opts.Domain != "" && opts.Domain != serverAddr {
		hosts = append(hosts, opts.Domain)
	}
	certPEM, keyPEM, err := shuttleCrypto.GenerateSelfSignedCert(hosts, 365*24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("generate TLS cert: %w", err)
	}
	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		return nil, fmt.Errorf("write cert: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("write key: %w", err)
	}

	// Build config
	cfg := DefaultServerConfig()
	cfg.Listen = opts.Listen
	cfg.TLS.CertFile = certPath
	cfg.TLS.KeyFile = keyPath
	cfg.Auth.Password = password
	cfg.Auth.PrivateKey = privHex
	cfg.Auth.PublicKey = pubHex

	h3Enabled := contains(opts.Transports, "h3")
	realityEnabled := contains(opts.Transports, "reality")
	cfg.Transport.H3.Enabled = h3Enabled
	cfg.Transport.Reality.Enabled = realityEnabled
	cfg.Transport.Reality.ShortIDs = []string{shortID}

	if opts.TargetSNI != "" {
		cfg.Transport.Reality.TargetSNI = opts.TargetSNI
		cfg.Transport.Reality.TargetAddr = opts.TargetSNI + ":443"
	}

	cfg.Admin.Enabled = true
	cfg.Admin.Listen = "127.0.0.1:9090"
	cfg.Admin.Token = adminToken

	// Enable mesh VPN if requested
	if opts.Mesh {
		cfg.Mesh.Enabled = true
		cfg.Mesh.P2PEnabled = true
	}

	// Write config
	if err := SaveServerConfig(configPath, cfg); err != nil {
		return nil, fmt.Errorf("save config: %w", err)
	}

	// Build share URI
	listenPort := "443"
	if _, port, err := net.SplitHostPort(opts.Listen); err == nil {
		listenPort = port
	}
	shareAddr := serverAddr
	if listenPort != "443" {
		shareAddr = net.JoinHostPort(serverAddr, listenPort)
	} else {
		shareAddr = net.JoinHostPort(serverAddr, "443")
	}

	transportLabel := "both"
	switch {
	case h3Enabled && realityEnabled:
		transportLabel = "both"
	case h3Enabled:
		transportLabel = "h3"
	case realityEnabled:
		transportLabel = "reality"
	}

	shareURI := EncodeShareURI(&ShareURI{
		Addr:      shareAddr,
		Password:  password,
		Transport: transportLabel,
		PublicKey: pubHex,
		ShortID:   shortID,
		SNI:       opts.TargetSNI,
		Mesh:      opts.Mesh,
	})

	return &InitResult{
		ConfigPath:  configPath,
		ShareURI:    shareURI,
		Password:    password,
		PublicKey:    pubHex,
		ServerAddr:  shareAddr,
		AdminToken:  adminToken,
		MeshEnabled: opts.Mesh,
		MeshCIDR:    cfg.Mesh.CIDR,
	}, nil
}

// FindDefaultConfig looks for server config in standard locations.
func FindDefaultConfig() string {
	candidates := []string{
		"/etc/shuttle/server.yaml",
	}

	// Add home dir path
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".shuttle", "server.yaml"))
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func applyInitDefaults(opts *InitOptions) {
	if opts.ConfigDir == "" {
		if os.Getuid() == 0 || runtime.GOOS == "linux" {
			opts.ConfigDir = "/etc/shuttle"
		} else {
			if home, err := os.UserHomeDir(); err == nil {
				opts.ConfigDir = filepath.Join(home, ".shuttle")
			} else {
				opts.ConfigDir = ".shuttle"
			}
		}
	}
	if opts.Listen == "" {
		opts.Listen = DefaultListenPort
	}
	if len(opts.Transports) == 0 {
		opts.Transports = []string{"h3", "reality"}
	}
	if opts.TargetSNI == "" {
		opts.TargetSNI = "www.microsoft.com"
	}

	// Check environment variable overrides
	if v := os.Getenv("SHUTTLE_DOMAIN"); v != "" && opts.Domain == "" {
		opts.Domain = v
	}
	if v := os.Getenv("SHUTTLE_PASSWORD"); v != "" && opts.Password == "" {
		opts.Password = v
	}
	if v := os.Getenv("SHUTTLE_LISTEN"); v != "" && opts.Listen == DefaultListenPort {
		opts.Listen = v
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	io.ReadFull(rand.Reader, b)
	return hex.EncodeToString(b)
}

func detectPublicIP() string {
	urls := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
	}
	client := &http.Client{Timeout: 5 * time.Second}
	for _, u := range urls {
		resp, err := client.Get(u)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		if err != nil {
			continue
		}
		ip := strings.TrimSpace(string(body))
		if net.ParseIP(ip) != nil {
			return ip
		}
	}
	return ""
}

func detectLocalIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ip := ipnet.IP.To4(); ip != nil && !ip.IsLoopback() {
					return ip.String()
				}
			}
		}
	}
	return "127.0.0.1"
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
