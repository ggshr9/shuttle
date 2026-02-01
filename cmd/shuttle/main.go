package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shuttle-proxy/shuttle/config"
	"github.com/shuttle-proxy/shuttle/congestion"
	"github.com/shuttle-proxy/shuttle/crypto"
	"github.com/shuttle-proxy/shuttle/internal/sysopt"
	"github.com/shuttle-proxy/shuttle/plugin"
	"github.com/shuttle-proxy/shuttle/proxy"
	"github.com/shuttle-proxy/shuttle/router"
	"github.com/shuttle-proxy/shuttle/router/geodata"
	"github.com/shuttle-proxy/shuttle/transport"
	"github.com/shuttle-proxy/shuttle/transport/cdn"
	"github.com/shuttle-proxy/shuttle/transport/h3"
	"github.com/shuttle-proxy/shuttle/transport/reality"
	"github.com/shuttle-proxy/shuttle/transport/selector"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Shuttle v%s — Break the impossible triangle\n\n", version)
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  shuttle run -c <config.yaml>    Start the client\n")
		fmt.Fprintf(os.Stderr, "  shuttle version                 Show version\n")
		fmt.Fprintf(os.Stderr, "  shuttle genkey                  Generate key pair\n")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Printf("shuttle v%s\n", version)
	case "genkey":
		genKey()
	case "run":
		configPath := "config/client.example.yaml"
		for i, arg := range os.Args[2:] {
			if arg == "-c" && i+1 < len(os.Args[2:]) {
				configPath = os.Args[i+3]
			}
		}
		run(configPath)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func genKey() {
	pub, priv, err := crypto.GenerateKeyPair()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating key pair: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Private Key: %x\n", priv)
	fmt.Printf("Public Key:  %x\n", pub)
}

func run(configPath string) {
	cfg, err := config.LoadClientConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Setup logger
	level := slog.LevelInfo
	switch cfg.Log.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	logger.Info("shuttle starting", "version", version)

	// Apply system optimizations
	sysopt.Apply(logger)

	// Setup context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutting down...")
		cancel()
	}()

	// --- Build congestion control ---
	var ccMode string
	var cc congestion.CongestionController
	switch cfg.Congestion.Mode {
	case "brutal":
		rate := cfg.Congestion.BrutalRate
		if rate == 0 {
			rate = 100 * 1024 * 1024
		}
		cc = congestion.NewBrutal(rate)
		ccMode = fmt.Sprintf("brutal (rate=%d)", rate)
	case "bbr":
		cc = congestion.NewBBR(0)
		ccMode = "bbr"
	default: // "adaptive" or empty
		cc = congestion.NewAdaptive(&congestion.AdaptiveConfig{
			BrutalRate: cfg.Congestion.BrutalRate,
		}, logger)
		ccMode = "adaptive"
	}
	ccAdapter := congestion.NewQUICAdapter(cc)
	logger.Info("congestion control", "mode", ccMode)

	// --- Load GeoIP/GeoSite data ---
	geoIPDB := router.NewGeoIPDB()
	geoSiteDB := router.NewGeoSiteDB()

	defaultIPEntries, defaultSiteEntries := geodata.LoadDefaults()
	for _, entry := range defaultIPEntries {
		geoIPDB.LoadFromCIDRs(entry.CountryCode, entry.CIDRs)
	}
	for _, entry := range defaultSiteEntries {
		geoSiteDB.LoadCategory(entry.Category, entry.Domains)
	}
	logger.Info("geodata loaded", "ip_countries", len(defaultIPEntries), "site_categories", len(defaultSiteEntries))

	// --- Setup DNS resolver ---
	dnsResolver := router.NewDNSResolver(&router.DNSConfig{
		DomesticServer: cfg.Routing.DNS.Domestic,
		RemoteServer:   cfg.Routing.DNS.Remote.Server,
		RemoteViaProxy: cfg.Routing.DNS.Remote.Via == "proxy",
		CacheSize:      10000,
		CacheTTL:       10 * time.Minute,
	}, geoIPDB, logger)

	// --- Build transports ---
	var transports []transport.ClientTransport

	if cfg.Transport.H3.Enabled {
		h3Client := h3.NewClient(&h3.ClientConfig{
			ServerAddr:        cfg.Server.Addr,
			ServerName:        cfg.Server.SNI,
			Password:          cfg.Server.Password,
			PathPrefix:        cfg.Transport.H3.PathPrefix,
			CongestionControl: ccAdapter,
		})
		transports = append(transports, h3Client)
	}

	if cfg.Transport.Reality.Enabled {
		realityClient := reality.NewClient(&reality.ClientConfig{
			ServerAddr: cfg.Server.Addr,
			ServerName: cfg.Transport.Reality.ServerName,
			ShortID:    cfg.Transport.Reality.ShortID,
			PublicKey:  cfg.Transport.Reality.PublicKey,
			Password:   cfg.Server.Password,
		})
		transports = append(transports, realityClient)
	}

	if cfg.Transport.CDN.Enabled {
		switch cfg.Transport.CDN.Mode {
		case "grpc":
			grpcClient := cdn.NewGRPCClient(&cdn.GRPCConfig{
				CDNDomain: cfg.Transport.CDN.Domain,
				Password:  cfg.Server.Password,
			})
			transports = append(transports, grpcClient)
		default: // "h2" or empty
			h2Client := cdn.NewH2Client(&cdn.H2Config{
				ServerAddr: cfg.Server.Addr,
				CDNDomain:  cfg.Transport.CDN.Domain,
				Path:       cfg.Transport.CDN.Path,
				Password:   cfg.Server.Password,
			})
			transports = append(transports, h2Client)
		}
	}

	if len(transports) == 0 {
		logger.Error("no transports enabled")
		os.Exit(1)
	}

	// Create transport selector
	sel := selector.New(transports, &selector.Config{
		Strategy: selector.StrategyAuto,
	}, logger)
	sel.Start(ctx)

	// --- Setup router ---
	routerCfg := &router.RouterConfig{
		DefaultAction: router.Action(cfg.Routing.Default),
	}
	for _, rule := range cfg.Routing.Rules {
		r := router.Rule{Action: router.Action(rule.Action)}
		switch {
		case rule.Domains != "":
			r.Type = "domain"
			r.Values = []string{rule.Domains}
		case rule.GeoIP != "":
			r.Type = "geoip"
			r.Values = []string{rule.GeoIP}
		case len(rule.Process) > 0:
			r.Type = "process"
			r.Values = rule.Process
		case rule.Protocol != "":
			r.Type = "protocol"
			r.Values = []string{rule.Protocol}
		case len(rule.IPCIDR) > 0:
			r.Type = "ip-cidr"
			r.Values = rule.IPCIDR
		}
		routerCfg.Rules = append(routerCfg.Rules, r)
	}
	rt := router.NewRouter(routerCfg, geoIPDB, geoSiteDB, logger)

	// Setup plugins
	metrics := plugin.NewMetrics()
	logPlugin := plugin.NewLogger(logger)
	_ = logPlugin
	_ = metrics

	// --- Dialer: DNS resolve + full routing ---
	dialer := func(dialCtx context.Context, network, addr string) (net.Conn, error) {
		host, port, _ := net.SplitHostPort(addr)

		// DNS resolution via anti-pollution resolver
		var ip net.IP
		if parsedIP := net.ParseIP(host); parsedIP != nil {
			ip = parsedIP
		} else {
			ips, err := dnsResolver.Resolve(dialCtx, host)
			if err != nil || len(ips) == 0 {
				// Fallback to system resolver
				ips2, err2 := net.DefaultResolver.LookupIP(dialCtx, "ip4", host)
				if err2 != nil {
					return nil, fmt.Errorf("dns resolve %s: %w", host, err)
				}
				ips = ips2
			}
			ip = ips[0]
		}

		// Full routing: domain → IP → protocol priority
		action := rt.Match(host, ip, "", "")

		switch action {
		case router.ActionDirect:
			logger.Debug("direct", "target", addr, "ip", ip)
			return (&net.Dialer{}).DialContext(dialCtx, network, net.JoinHostPort(ip.String(), port))
		case router.ActionReject:
			return nil, fmt.Errorf("rejected: %s", addr)
		default: // proxy
			logger.Debug("proxy", "target", addr)
			conn, err := sel.Dial(dialCtx, cfg.Server.Addr)
			if err != nil {
				return nil, fmt.Errorf("proxy dial: %w", err)
			}
			stream, err := conn.OpenStream(dialCtx)
			if err != nil {
				return nil, fmt.Errorf("open stream: %w", err)
			}
			header := []byte(addr + "\n")
			if _, err := stream.Write(header); err != nil {
				stream.Close()
				return nil, fmt.Errorf("send target: %w", err)
			}
			return &streamConn{stream: stream, addr: addr}, nil
		}
	}

	// --- Start local proxies ---
	if cfg.Proxy.SOCKS5.Enabled {
		socks := proxy.NewSOCKS5Server(&proxy.SOCKS5Config{
			ListenAddr: cfg.Proxy.SOCKS5.Listen,
		}, dialer, logger)
		if err := socks.Start(ctx); err != nil {
			logger.Error("failed to start SOCKS5", "err", err)
			os.Exit(1)
		}
		defer socks.Close()
	}

	if cfg.Proxy.HTTP.Enabled {
		httpProxy := proxy.NewHTTPServer(&proxy.HTTPConfig{
			ListenAddr: cfg.Proxy.HTTP.Listen,
		}, dialer, logger)
		if err := httpProxy.Start(ctx); err != nil {
			logger.Error("failed to start HTTP proxy", "err", err)
			os.Exit(1)
		}
		defer httpProxy.Close()
	}

	if cfg.Proxy.TUN.Enabled {
		tunServer := proxy.NewTUNServer(&proxy.TUNConfig{
			DeviceName: cfg.Proxy.TUN.DeviceName,
			CIDR:       cfg.Proxy.TUN.CIDR,
			MTU:        cfg.Proxy.TUN.MTU,
			AutoRoute:  cfg.Proxy.TUN.AutoRoute,
		}, dialer, logger)
		if err := tunServer.Start(ctx); err != nil {
			logger.Warn("TUN device failed (falling back to SOCKS5/HTTP)", "err", err)
		} else {
			defer tunServer.Close()
		}
	}

	logger.Info("shuttle is running",
		"socks5", cfg.Proxy.SOCKS5.Listen,
		"http", cfg.Proxy.HTTP.Listen,
		"transports", len(transports))

	<-ctx.Done()
	logger.Info("shuttle stopped")
}

// streamConn wraps a transport.Stream as a net.Conn.
type streamConn struct {
	stream transport.Stream
	addr   string
}

func (c *streamConn) Read(b []byte) (int, error)               { return c.stream.Read(b) }
func (c *streamConn) Write(b []byte) (int, error)              { return c.stream.Write(b) }
func (c *streamConn) Close() error                              { return c.stream.Close() }
func (c *streamConn) LocalAddr() net.Addr                       { return &net.TCPAddr{} }
func (c *streamConn) RemoteAddr() net.Addr                      { return &net.TCPAddr{} }
func (c *streamConn) SetDeadline(t time.Time) error             { return nil }
func (c *streamConn) SetReadDeadline(t time.Time) error         { return nil }
func (c *streamConn) SetWriteDeadline(t time.Time) error        { return nil }
