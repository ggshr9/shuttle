package engine

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/congestion"
	"github.com/shuttleX/shuttle/internal/procnet"
	"github.com/shuttleX/shuttle/obfs"
	"github.com/shuttleX/shuttle/plugin"
	"github.com/shuttleX/shuttle/proxy"
	"github.com/shuttleX/shuttle/qos"
	"github.com/shuttleX/shuttle/router"
	"github.com/shuttleX/shuttle/router/geodata"
	"github.com/shuttleX/shuttle/stream"
	"github.com/shuttleX/shuttle/transport"
	"github.com/shuttleX/shuttle/transport/selector"
)

// buildCongestionControl creates the appropriate CC based on config.
func (e *Engine) buildCongestionControl(cfg *config.ClientConfig) quic.CongestionControl {
	var cc congestion.CongestionController
	switch cfg.Congestion.Mode {
	case "brutal":
		rate := cfg.Congestion.BrutalRate
		if rate == 0 {
			rate = 100 * 1024 * 1024
		}
		cc = congestion.NewBrutal(rate)
	case "bbr":
		cc = congestion.NewBBR(0)
	default:
		cc = congestion.NewAdaptive(&congestion.AdaptiveConfig{
			BrutalRate: cfg.Congestion.BrutalRate,
		}, e.logger)
	}
	return congestion.NewQUICAdapter(cc)
}

// buildTransports creates client transports from config.
func (e *Engine) buildTransports(cfg *config.ClientConfig, ccAdapter quic.CongestionControl) []transport.ClientTransport {
	var transports []transport.ClientTransport

	opts := adapter.FactoryOptions{
		Logger:            e.logger,
		CongestionControl: ccAdapter,
	}

	for name, factory := range adapter.All() {
		t, err := factory.NewClient(cfg, opts)
		if err != nil {
			e.logger.Warn("transport factory failed", "type", name, "err", err)
			continue
		}
		if t != nil {
			transports = append(transports, t)
		}
	}

	e.logger.Debug("transport setup", "count", len(transports))
	return transports
}

// buildRouter creates the routing engine including GeoIP/GeoSite and DNS.
func (e *Engine) buildRouter(cfg *config.ClientConfig) (*router.Router, *router.DNSResolver, *router.Prefetcher) {
	geoIPDB := router.NewGeoIPDB()
	geoSiteDB := router.NewGeoSiteDB()

	loaded := false
	if cfg.Routing.GeoData.Enabled {
		mgr := e.getOrCreateGeoManager(cfg)
		if ipEntries, err := mgr.LoadGeoIPEntries(); err == nil {
			for _, entry := range ipEntries {
				geoIPDB.LoadFromCIDRs(entry.CountryCode, entry.CIDRs)
			}
			e.logger.Info("loaded geodata GeoIP", "entries", len(ipEntries))
			loaded = true
		}
		if siteEntries, err := mgr.LoadGeoSiteEntries(); err == nil {
			for _, entry := range siteEntries {
				geoSiteDB.LoadCategory(entry.Category, entry.Domains)
			}
			e.logger.Info("loaded geodata GeoSite", "categories", len(siteEntries))
			loaded = true
		}
	}
	if !loaded {
		defaultIPEntries, defaultSiteEntries := geodata.LoadDefaults()
		for _, entry := range defaultIPEntries {
			geoIPDB.LoadFromCIDRs(entry.CountryCode, entry.CIDRs)
		}
		for _, entry := range defaultSiteEntries {
			geoSiteDB.LoadCategory(entry.Category, entry.Domains)
		}
	}

	// PersistentConn defaults to true when not explicitly set in config.
	persistentConn := true
	if cfg.Routing.DNS.PersistentConn != nil {
		persistentConn = *cfg.Routing.DNS.PersistentConn
	}
	dnsResolver := router.NewDNSResolver(&router.DNSConfig{
		DomesticServer: cfg.Routing.DNS.Domestic,
		RemoteServer:   cfg.Routing.DNS.Remote.Server,
		RemoteViaProxy: cfg.Routing.DNS.Remote.Via == "proxy",
		CacheSize:      10000,
		CacheTTL:       10 * time.Minute,
		Prefetch:       cfg.Routing.DNS.Prefetch,
		LeakPrevention: cfg.Routing.DNS.LeakPrevention,
		DomesticDoH:    cfg.Routing.DNS.DomesticDoH,
		StripECS:       cfg.Routing.DNS.StripECS,
		PersistentConn: persistentConn,
	}, geoIPDB, e.logger)

	var prefetcher *router.Prefetcher
	if cfg.Routing.DNS.Prefetch {
		prefetcher = router.NewPrefetcher(dnsResolver, 100, 30*time.Second, e.logger)
		dnsResolver.SetPrefetcher(prefetcher)
	}

	routerCfg := &router.RouterConfig{
		DefaultAction: router.Action(cfg.Routing.Default),
	}
	for _, rule := range cfg.Routing.Rules {
		r := router.Rule{Action: router.Action(rule.Action)}
		switch {
		case rule.Domains != "":
			r.Type = "domain"
			r.Values = []string{rule.Domains}
		case rule.GeoSite != "":
			r.Type = "geosite"
			r.Values = []string{rule.GeoSite}
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
	e.logger.Debug("router built", "rules", len(routerCfg.Rules), "default_action", routerCfg.DefaultAction, "dns_prefetch", cfg.Routing.DNS.Prefetch)
	rt := router.NewRouter(routerCfg, geoIPDB, geoSiteDB, e.logger)
	return rt, dnsResolver, prefetcher
}

// buildRetryConfig converts config.RetryConfig to engine.RetryConfig with parsed durations.
func (e *Engine) buildRetryConfig(cfg config.RetryConfig) RetryConfig {
	rc := DefaultRetryConfig()
	if cfg.MaxAttempts > 0 {
		rc.MaxAttempts = cfg.MaxAttempts
	}
	if cfg.InitialBackoff != "" {
		if d, err := time.ParseDuration(cfg.InitialBackoff); err == nil {
			rc.InitialBackoff = d
		} else {
			e.logger.Warn("invalid duration, using default", "field", "retry.initial_backoff", "value", cfg.InitialBackoff, "err", err)
		}
	}
	if cfg.MaxBackoff != "" {
		if d, err := time.ParseDuration(cfg.MaxBackoff); err == nil {
			rc.MaxBackoff = d
		} else {
			e.logger.Warn("invalid duration, using default", "field", "retry.max_backoff", "value", cfg.MaxBackoff, "err", err)
		}
	}
	return rc
}

// resolveTarget resolves a host string to an IP address, using the DNS resolver
// with a fallback to the system resolver.
func resolveTarget(ctx context.Context, host string, dnsResolver *router.DNSResolver) (net.IP, error) {
	if parsedIP := net.ParseIP(host); parsedIP != nil {
		return parsedIP, nil
	}
	ips, err := dnsResolver.Resolve(ctx, host)
	if err != nil || len(ips) == 0 {
		ips2, err2 := net.DefaultResolver.LookupIP(ctx, "ip4", host)
		if err2 != nil {
			return nil, fmt.Errorf("dns resolve %s: %w", host, err2)
		}
		ips = ips2
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no DNS results for %s", host)
	}
	return ips[0], nil
}

// dialProxyStream dials the proxy server, opens a stream, and returns the
// connection and stream. It handles retry, circuit breaker recording, and
// stream metric wrapping.
func (e *Engine) dialProxyStream(
	ctx context.Context,
	serverAddr, addr, network string,
	retryCfg RetryConfig,
	shaperCfg obfs.ShaperConfig,
	classifier *qos.Classifier,
) (net.Conn, error) {
	curSel := e.selector()
	if curSel == nil {
		return nil, fmt.Errorf("no active selector")
	}
	if cb := e.circuitBreaker; cb != nil && !cb.Allow() {
		return nil, fmt.Errorf("circuit breaker open for %s, retry after cooldown", serverAddr)
	}

	var conn transport.Connection
	var rawStream transport.Stream
	err := retryWithBackoff(ctx, retryCfg, func() error {
		var dialErr error
		conn, dialErr = curSel.Dial(ctx, serverAddr)
		if dialErr != nil {
			return fmt.Errorf("proxy dial: %w", dialErr)
		}
		rawStream, dialErr = conn.OpenStream(ctx)
		if dialErr != nil {
			return fmt.Errorf("open stream: %w", dialErr)
		}
		return nil
	})
	if err != nil {
		if cb := e.circuitBreaker; cb != nil {
			cb.RecordFailure()
		}
		return nil, err
	}
	if cb := e.circuitBreaker; cb != nil {
		cb.RecordSuccess()
	}

	// Wrap with measured stream for per-stream metrics.
	seq := atomic.AddUint64(&e.connSeq, 1)
	connID := strconv.FormatUint(seq, 16)
	st := e.streamTracker
	transportType := ""
	if curSel != nil {
		transportType = curSel.ActiveTransport()
	}
	metrics := st.Track(rawStream.StreamID(), addr, transportType)
	metrics.ConnID = connID
	measured := stream.NewMeasuredStream(rawStream, metrics)

	// Classify traffic and set QoS priority on the stream.
	if classifier != nil {
		port := extractPort(addr)
		priority := classifier.ClassifyPort(port)
		measured.SetPriority(int(priority))
	}

	// For UDP streams, prepend the UDP marker so the server uses UDP relay.
	var header []byte
	if network == "udp" {
		header = []byte(proxy.UDPStreamPrefix + addr + "\n")
	} else {
		header = []byte(addr + "\n")
	}
	if _, err := measured.Write(header); err != nil {
		measured.Close()
		return nil, fmt.Errorf("send target: %w", err)
	}

	sc := &streamConn{stream: measured, addr: addr}
	if shaperCfg.Enabled {
		shaper := obfs.NewShaper(measured, shaperCfg)
		return &shapedConn{streamConn: sc, shaper: shaper}, nil
	}
	return sc, nil
}

// createDialer builds the proxy dialer function.
func (e *Engine) createDialer(cfg *config.ClientConfig, rt *router.Router, dnsResolver *router.DNSResolver) func(context.Context, string, string) (net.Conn, error) {
	serverAddr := cfg.Server.Addr
	retryCfg := e.buildRetryConfig(cfg.Retry)
	shaperCfg := e.buildShaperConfig(cfg.Obfs)
	var classifier *qos.Classifier
	if cfg.QoS.Enabled {
		classifier = qos.NewClassifier(&cfg.QoS)
	}
	return func(dialCtx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
			port = ""
		}

		ip, err := resolveTarget(dialCtx, host, dnsResolver)
		if err != nil {
			return nil, err
		}

		procName := proxy.ProcessFromContext(dialCtx)
		action := rt.Match(host, ip, procName, "")

		switch action {
		case router.ActionDirect:
			return (&net.Dialer{}).DialContext(dialCtx, network, net.JoinHostPort(ip.String(), port))
		case router.ActionReject:
			return nil, fmt.Errorf("rejected: %s", addr)
		default:
			return e.dialProxyStream(dialCtx, serverAddr, addr, network, retryCfg, shaperCfg, classifier)
		}
	}
}

// startSOCKS5 starts the SOCKS5 proxy server if configured.
func (e *Engine) startSOCKS5(ctx context.Context, cfg *config.ClientConfig, dialer func(context.Context, string, string) (net.Conn, error), procResolver *procnet.Resolver) (func() error, error) {
	socks := proxy.NewSOCKS5Server(&proxy.SOCKS5Config{
		ListenAddr: cfg.Proxy.SOCKS5.Listen,
	}, dialer, e.logger)
	socks.ProcResolver = procResolver
	socks.QoSClassifier = qos.NewClassifier(&cfg.QoS)
	if err := socks.Start(ctx); err != nil {
		return nil, fmt.Errorf("socks5: %w", err)
	}
	return socks.Close, nil
}

// startHTTPProxy starts the HTTP CONNECT proxy server if configured.
func (e *Engine) startHTTPProxy(ctx context.Context, cfg *config.ClientConfig, dialer func(context.Context, string, string) (net.Conn, error), procResolver *procnet.Resolver) (func() error, error) {
	httpProxy := proxy.NewHTTPServer(&proxy.HTTPConfig{
		ListenAddr: cfg.Proxy.HTTP.Listen,
	}, dialer, e.logger)
	httpProxy.ProcResolver = procResolver
	httpProxy.QoSClassifier = qos.NewClassifier(&cfg.QoS)
	if err := httpProxy.Start(ctx); err != nil {
		return nil, fmt.Errorf("http proxy: %w", err)
	}
	return httpProxy.Close, nil
}

// startTUN starts the TUN device proxy and optionally the mesh network.
// Returns closers for all started components. If TUN fails, it logs a warning
// and returns nil (non-fatal).
func (e *Engine) startTUN(ctx context.Context, cfg *config.ClientConfig, dialer func(context.Context, string, string) (net.Conn, error), procResolver *procnet.Resolver) []func() error {
	tunServer := proxy.NewTUNServer(&proxy.TUNConfig{
		DeviceName: cfg.Proxy.TUN.DeviceName,
		CIDR:       cfg.Proxy.TUN.CIDR,
		MTU:        cfg.Proxy.TUN.MTU,
		AutoRoute:  cfg.Proxy.TUN.AutoRoute,
		TunFD:      cfg.Proxy.TUN.TunFD,
	}, dialer, e.logger)
	tunServer.ProcResolver = procResolver
	tunServer.QoSClassifier = qos.NewClassifier(&cfg.QoS)
	if err := tunServer.Start(ctx); err != nil {
		e.logger.Warn("TUN device failed", "err", err)
		return nil
	}
	closers := []func() error{tunServer.Close}
	if cfg.Mesh.Enabled {
		if mc := e.connectMesh(ctx, cfg, tunServer); mc != nil {
			closers = append(closers, mc.Close)
		}
	}
	return closers
}

// startProxies starts all configured local proxy servers.
func (e *Engine) startProxies(ctx context.Context, cfg *config.ClientConfig, dialer func(context.Context, string, string) (net.Conn, error), sel *selector.Selector, cancel context.CancelFunc) ([]func() error, error) {
	procResolver := procnet.NewResolver()
	var closers []func() error

	cleanup := func(err error) ([]func() error, error) {
		for _, c := range closers {
			_ = c()
		}
		sel.Close()
		cancel()
		e.mu.Lock()
		e.state = StateStopped
		e.sel = nil
		e.cancel = nil
		e.mu.Unlock()
		return nil, err
	}

	if cfg.Proxy.SOCKS5.Enabled {
		closer, err := e.startSOCKS5(ctx, cfg, dialer, procResolver)
		if err != nil {
			return cleanup(err)
		}
		closers = append(closers, closer)
	}

	if cfg.Proxy.HTTP.Enabled {
		closer, err := e.startHTTPProxy(ctx, cfg, dialer, procResolver)
		if err != nil {
			return cleanup(err)
		}
		closers = append(closers, closer)
	}

	if cfg.Proxy.TUN.Enabled {
		closers = append(closers, e.startTUN(ctx, cfg, dialer, procResolver)...)
	} else if cfg.Mesh.Enabled {
		e.logger.Warn("mesh requires TUN to be enabled, skipping mesh")
	}

	return closers, nil
}

// wrapDialer wraps a dialer function so that every successfully dialled
// connection is run through the plugin chain's OnConnect hooks. When the
// returned connection is closed, OnDisconnect is called on the full chain
// via the chainConn wrapper.
func (e *Engine) wrapDialer(
	dialer func(context.Context, string, string) (net.Conn, error),
	chain *plugin.Chain,
) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dialer(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		// Run the connection through the plugin chain (metrics, conntrack, logger).
		// The chain may wrap the conn (e.g. trackingConn for byte counting).
		wrapped, err := chain.OnConnect(conn, addr)
		if err != nil {
			conn.Close()
			return nil, err
		}
		return &chainConn{Conn: wrapped, chain: chain}, nil
	}
}

// buildShaperConfig converts config.ObfsConfig into an obfs.ShaperConfig.
// Returns a zero-value (Enabled=false) config if shaping is disabled or parsing fails.
func (e *Engine) buildShaperConfig(cfg config.ObfsConfig) obfs.ShaperConfig {
	if !cfg.ShapingEnabled {
		return obfs.ShaperConfig{}
	}
	sc := obfs.DefaultShaperConfig()
	sc.Enabled = true
	if cfg.MinDelay != "" {
		if d, err := time.ParseDuration(cfg.MinDelay); err == nil {
			sc.MinDelay = d
		} else {
			e.logger.Warn("invalid duration, using default", "field", "obfs.min_delay", "value", cfg.MinDelay, "err", err)
		}
	}
	if cfg.MaxDelay != "" {
		if d, err := time.ParseDuration(cfg.MaxDelay); err == nil {
			sc.MaxDelay = d
		} else {
			e.logger.Warn("invalid duration, using default", "field", "obfs.max_delay", "value", cfg.MaxDelay, "err", err)
		}
	}
	if cfg.ChunkSize > 0 {
		sc.ChunkMinSize = cfg.ChunkSize
		// Set max to 2x min or at least 1400, whichever is larger
		sc.ChunkMaxSize = cfg.ChunkSize * 2
		if sc.ChunkMaxSize < 1400 {
			sc.ChunkMaxSize = 1400
		}
	}
	return sc
}

// getOrCreateGeoManager returns the existing geo manager or creates a new one.
func (e *Engine) getOrCreateGeoManager(cfg *config.ClientConfig) *geodata.Manager {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.geoManager == nil {
		e.geoManager = geodata.NewManager(geodata.ManagerConfig{
			Enabled:        cfg.Routing.GeoData.Enabled,
			DataDir:        cfg.Routing.GeoData.DataDir,
			AutoUpdate:     cfg.Routing.GeoData.AutoUpdate,
			UpdateInterval: cfg.Routing.GeoData.UpdateInterval,
			DirectListURL:  cfg.Routing.GeoData.DirectListURL,
			ProxyListURL:   cfg.Routing.GeoData.ProxyListURL,
			RejectListURL:  cfg.Routing.GeoData.RejectListURL,
			GFWListURL:     cfg.Routing.GeoData.GFWListURL,
			CNCidrURL:      cfg.Routing.GeoData.CNCidrURL,
			PrivateCidrURL: cfg.Routing.GeoData.PrivateCidrURL,
		}, e.logger)
	}
	return e.geoManager
}
