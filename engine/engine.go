package engine

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/shuttle-proxy/shuttle/config"
	"github.com/shuttle-proxy/shuttle/mesh"
	"github.com/shuttle-proxy/shuttle/congestion"
	"github.com/shuttle-proxy/shuttle/obfs"
	"github.com/shuttle-proxy/shuttle/internal/logutil"
	"github.com/shuttle-proxy/shuttle/internal/netmon"
	"github.com/shuttle-proxy/shuttle/internal/procnet"
	"github.com/shuttle-proxy/shuttle/internal/sysopt"
	"github.com/shuttle-proxy/shuttle/plugin"
	"github.com/shuttle-proxy/shuttle/proxy"
	"github.com/shuttle-proxy/shuttle/router"
	"github.com/shuttle-proxy/shuttle/stream"
	"github.com/shuttle-proxy/shuttle/router/geodata"
	"github.com/shuttle-proxy/shuttle/transport"
	"github.com/shuttle-proxy/shuttle/qos"
	"github.com/shuttle-proxy/shuttle/transport/cdn"
	"github.com/shuttle-proxy/shuttle/transport/h3"
	"github.com/shuttle-proxy/shuttle/transport/reality"
	"github.com/shuttle-proxy/shuttle/transport/selector"
	rtcTransport "github.com/shuttle-proxy/shuttle/transport/webrtc"
)

const eventChannelBuffer = 64

// Engine is the core shuttle client, managing transports, routing, and local proxies.
type Engine struct {
	mu      sync.RWMutex
	state   EngineState
	cfg     *config.ClientConfig
	logger  *slog.Logger
	metrics *plugin.Metrics

	// lifecycleMu serialises Start/Stop/Reload so that concurrent callers
	// cannot interleave their long-running init/shutdown sequences.
	lifecycleMu sync.Mutex

	sel       *selector.Selector
	cancel    context.CancelFunc
	parentCtx context.Context // stored for Reload

	// Closers for local proxy servers
	closers []func() error

	// Mesh client for P2P VPN
	meshClient *mesh.MeshClient

	// Network change monitor
	netMon *netmon.Monitor

	// Geo data manager
	geoManager *geodata.Manager

	// Current router (for PAC generation and conflict detection)
	currentRouter *router.Router

	// Stream-level metrics tracker
	streamTracker *stream.StreamTracker

	// Circuit breaker for transport connections
	circuitBreaker *CircuitBreaker

	// Plugin chain for connection tracking, filtering, and logging
	chain *plugin.Chain

	// Background goroutine tracking for clean shutdown
	bgWg sync.WaitGroup

	// Connection sequence counter for generating correlation IDs
	connSeq uint64

	// Event subscribers — stores bidirectional channels, Subscribe returns receive-only view
	subMu sync.Mutex
	subs  map[chan Event]struct{}
}

const stopTimeout = 10 * time.Second

// New creates a new Engine from the given config.
func New(cfg *config.ClientConfig) *Engine {
	logger := logutil.NewLogger(cfg.Log.Level, cfg.Log.Format)

	return &Engine{
		state:   StateStopped,
		cfg:     cfg,
		logger:  logger,
		metrics: plugin.NewMetrics(),
		subs:    make(map[chan Event]struct{}),
	}
}

// Start initializes all subsystems and begins proxying.
func (e *Engine) Start(ctx context.Context) error {
	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()
	return e.startInternal(ctx)
}

// startInternal is the lock-free core of Start; the caller must hold lifecycleMu.
func (e *Engine) startInternal(ctx context.Context) error {
	e.mu.Lock()
	if e.state == StateRunning || e.state == StateStarting {
		st := e.state
		e.mu.Unlock()
		return fmt.Errorf("engine already %s", st)
	}
	e.state = StateStarting
	e.parentCtx = ctx
	e.mu.Unlock()

	e.emit(Event{Type: EventLog, Message: "engine starting"})

	e.mu.Lock()
	e.streamTracker = stream.NewStreamTracker(0) // default 1000-entry ring
	e.mu.Unlock()

	sysopt.Apply(e.logger)

	e.mu.Lock()
	e.circuitBreaker = NewCircuitBreaker(CircuitBreakerConfig{
		OnStateChange: func(state CircuitState, cooldown time.Duration) {
			if state == CircuitOpen {
				e.emit(Event{Type: EventConnectionError, Error: "circuit breaker open", BackoffMs: cooldown.Milliseconds()})
			}
		},
	})
	e.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)

	// On any failure below, this helper resets state.
	fail := func(err error) error {
		cancel()
		e.mu.Lock()
		e.state = StateStopped
		e.cancel = nil
		e.sel = nil
		e.mu.Unlock()
		return err
	}

	e.mu.RLock()
	cfgSnap := e.cfg.DeepCopy()
	e.mu.RUnlock()

	ccAdapter := e.buildCongestionControl(cfgSnap)
	transports := e.buildTransports(cfgSnap, ccAdapter)
	if len(transports) == 0 {
		return fail(fmt.Errorf("no transports enabled; enable at least one in config (transport.h3, transport.reality, transport.cdn, or transport.webrtc)"))
	}

	strategy := selector.StrategyAuto
	switch cfgSnap.Transport.Preferred {
	case "multipath":
		strategy = selector.StrategyMultipath
	case "latency":
		strategy = selector.StrategyLatency
	case "priority":
		strategy = selector.StrategyPriority
	}

	sel := selector.New(transports, &selector.Config{
		Strategy:          strategy,
		ServerAddr:        cfgSnap.Server.Addr,
		MultipathSchedule: cfgSnap.Transport.MultipathSchedule,
		WarmUpConns:       cfgSnap.Transport.WarmUpConns,
	}, e.logger)
	sel.Start(ctx)

	e.mu.Lock()
	e.sel = sel
	e.cancel = cancel
	e.mu.Unlock()

	rt, dnsResolver, prefetcher := e.buildRouter(cfgSnap)
	if prefetcher != nil {
		go prefetcher.Start(ctx)
	}
	if cfgSnap.Routing.GeoData.Enabled && cfgSnap.Routing.GeoData.AutoUpdate {
		if gm := e.GeoManager(); gm != nil {
			gm.Start(ctx)
		}
	}
	dialer := e.createDialer(cfgSnap, rt, dnsResolver)

	// Build plugin chain: metrics (byte counting + stats), connection tracker
	// (lifecycle events), and logger (debug logging).
	connTracker := plugin.NewConnTracker(e)
	chain := plugin.NewChain(
		e.metrics,
		connTracker,
		plugin.NewLogger(e.logger),
	)
	if err := chain.Init(ctx); err != nil {
		return fail(fmt.Errorf("plugin chain init: %w", err))
	}
	e.mu.Lock()
	e.chain = chain
	e.mu.Unlock()

	// Wrap dialer so every proxied connection flows through the plugin chain.
	dialer = e.wrapDialer(dialer, chain)

	closers, err := e.startProxies(ctx, cfgSnap, dialer, sel, cancel)
	if err != nil {
		sel.Close()
		chain.Close() // Clean up plugin chain on proxy start failure
		return fail(err)
	}

	e.mu.Lock()
	e.closers = closers
	e.currentRouter = rt
	e.state = StateRunning
	e.mu.Unlock()

	e.emit(Event{Type: EventConnected, Message: "engine started"})
	e.bgWg.Add(1)
	go func() {
		defer e.bgWg.Done()
		e.speedLoop(ctx)
	}()

	// Start network change monitor to detect WiFi/cellular switches.
	nm := netmon.New(5 * time.Second)
	nm.OnChange(func() {
		e.logger.Info("network change detected")
		e.emit(Event{Type: EventNetworkChange, Message: "network change detected"})
	})
	nm.Start(ctx)
	e.mu.Lock()
	e.netMon = nm
	e.mu.Unlock()

	return nil
}

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

	if cfg.Transport.H3.Enabled {
		h3Cfg := &h3.ClientConfig{
			ServerAddr:         cfg.Server.Addr,
			ServerName:         cfg.Server.SNI,
			Password:           cfg.Server.Password,
			PathPrefix:         cfg.Transport.H3.PathPrefix,
			InsecureSkipVerify: cfg.Transport.H3.InsecureSkipVerify,
			CongestionControl:  ccAdapter,
		}
		if cfg.Transport.H3.Multipath.Enabled {
			probeInterval := 5 * time.Second
			if cfg.Transport.H3.Multipath.ProbeInterval != "" {
				if d, err := time.ParseDuration(cfg.Transport.H3.Multipath.ProbeInterval); err == nil {
					probeInterval = d
				} else {
					e.logger.Warn("invalid duration, using default", "field", "transport.h3.multipath.probe_interval", "value", cfg.Transport.H3.Multipath.ProbeInterval, "err", err)
				}
			}
			h3Cfg.Multipath = &h3.MultipathConfig{
				Enabled:       true,
				Interfaces:    cfg.Transport.H3.Multipath.Interfaces,
				Mode:          cfg.Transport.H3.Multipath.Mode,
				ProbeInterval: probeInterval,
			}
		}
		transports = append(transports, h3.NewClient(h3Cfg))
	}

	if cfg.Transport.Reality.Enabled {
		transports = append(transports, reality.NewClient(&reality.ClientConfig{
			ServerAddr: cfg.Server.Addr,
			ServerName: cfg.Transport.Reality.ServerName,
			ShortID:    cfg.Transport.Reality.ShortID,
			PublicKey:  cfg.Transport.Reality.PublicKey,
			Password:   cfg.Server.Password,
			Yamux:      &cfg.Yamux,
		}))
	}

	if cfg.Transport.CDN.Enabled {
		switch cfg.Transport.CDN.Mode {
		case "grpc":
			transports = append(transports, cdn.NewGRPCClient(&cdn.GRPCConfig{
				CDNDomain:   cfg.Transport.CDN.Domain,
				Password:    cfg.Server.Password,
				FrontDomain: cfg.Transport.CDN.FrontDomain,
			}, cdn.WithGRPCLogger(e.logger)))
		default:
			transports = append(transports, cdn.NewH2Client(&cdn.H2Config{
				ServerAddr:         cfg.Server.Addr,
				CDNDomain:          cfg.Transport.CDN.Domain,
				Path:               cfg.Transport.CDN.Path,
				Password:           cfg.Server.Password,
				FrontDomain:        cfg.Transport.CDN.FrontDomain,
				InsecureSkipVerify: cfg.Transport.CDN.InsecureSkipVerify,
			}, cdn.WithH2Logger(e.logger)))
		}
	}

	if cfg.Transport.WebRTC.Enabled {
		transports = append(transports, rtcTransport.NewClient(&rtcTransport.ClientConfig{
			SignalURL:   cfg.Transport.WebRTC.SignalURL,
			Password:    cfg.Server.Password,
			STUNServers: cfg.Transport.WebRTC.STUNServers,
			TURNServers: cfg.Transport.WebRTC.TURNServers,
			TURNUser:    cfg.Transport.WebRTC.TURNUser,
			TURNPass:    cfg.Transport.WebRTC.TURNPass,
			ICEPolicy:   cfg.Transport.WebRTC.ICEPolicy,
		}))
	}

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
			host = addr // fallback: use addr directly (e.g. bare hostname)
			port = ""
		}

		var ip net.IP
		if parsedIP := net.ParseIP(host); parsedIP != nil {
			ip = parsedIP
		} else {
			ips, err := dnsResolver.Resolve(dialCtx, host)
			if err != nil || len(ips) == 0 {
				ips2, err2 := net.DefaultResolver.LookupIP(dialCtx, "ip4", host)
				if err2 != nil {
					return nil, fmt.Errorf("dns resolve %s: %w", host, err2)
				}
				ips = ips2
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("no DNS results for %s", host)
			}
			ip = ips[0]
		}

		procName := proxy.ProcessFromContext(dialCtx)
		action := rt.Match(host, ip, procName, "")

		switch action {
		case router.ActionDirect:
			return (&net.Dialer{}).DialContext(dialCtx, network, net.JoinHostPort(ip.String(), port))
		case router.ActionReject:
			return nil, fmt.Errorf("rejected: %s", addr)
		default:
			curSel := e.selector()
			if curSel == nil {
				return nil, fmt.Errorf("no active selector")
			}
			// Check circuit breaker before attempting connection.
			if cb := e.circuitBreaker; cb != nil && !cb.Allow() {
				return nil, fmt.Errorf("circuit breaker open for %s, retry after cooldown", serverAddr)
			}
			var conn transport.Connection
			var rawStream transport.Stream
			err := retryWithBackoff(dialCtx, retryCfg, func() error {
				var dialErr error
				conn, dialErr = curSel.Dial(dialCtx, serverAddr)
				if dialErr != nil {
					return fmt.Errorf("proxy dial: %w", dialErr)
				}
				rawStream, dialErr = conn.OpenStream(dialCtx)
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
	}
}

// startProxies starts all configured local proxy servers.
func (e *Engine) startProxies(ctx context.Context, cfg *config.ClientConfig, dialer func(context.Context, string, string) (net.Conn, error), sel *selector.Selector, cancel context.CancelFunc) ([]func() error, error) {
	procResolver := procnet.NewResolver()
	var closers []func() error

	cleanup := func(err error) ([]func() error, error) {
		for _, c := range closers {
			c()
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
		socks := proxy.NewSOCKS5Server(&proxy.SOCKS5Config{
			ListenAddr: cfg.Proxy.SOCKS5.Listen,
		}, dialer, e.logger)
		socks.ProcResolver = procResolver
		socks.QoSClassifier = qos.NewClassifier(&cfg.QoS)
		if err := socks.Start(ctx); err != nil {
			return cleanup(fmt.Errorf("socks5: %w", err))
		}
		closers = append(closers, socks.Close)
	}

	if cfg.Proxy.HTTP.Enabled {
		httpProxy := proxy.NewHTTPServer(&proxy.HTTPConfig{
			ListenAddr: cfg.Proxy.HTTP.Listen,
		}, dialer, e.logger)
		httpProxy.ProcResolver = procResolver
		httpProxy.QoSClassifier = qos.NewClassifier(&cfg.QoS)
		if err := httpProxy.Start(ctx); err != nil {
			return cleanup(fmt.Errorf("http proxy: %w", err))
		}
		closers = append(closers, httpProxy.Close)
	}

	if cfg.Proxy.TUN.Enabled {
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
		} else {
			closers = append(closers, tunServer.Close)

			// Mesh setup: requires TUN to be running
			if cfg.Mesh.Enabled {
				if mc := e.connectMesh(ctx, cfg, tunServer); mc != nil {
					closers = append(closers, mc.Close)
				}
			}
		}
	} else if cfg.Mesh.Enabled {
		e.logger.Warn("mesh requires TUN to be enabled, skipping mesh")
	}

	return closers, nil
}

const meshMaxRetries = 3

// connectMesh attempts to establish a mesh connection with retries.
func (e *Engine) connectMesh(ctx context.Context, cfg *config.ClientConfig, tunServer *proxy.TUNServer) *mesh.MeshClient {
	serverAddr := cfg.Server.Addr
	var lastErr error
	for attempt := 1; attempt <= meshMaxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil
		}
		curSel := e.selector()
		if curSel == nil {
			e.logger.Warn("mesh: no active selector, skipping")
			return nil
		}
		conn, err := curSel.Dial(ctx, serverAddr)
		if err != nil {
			lastErr = err
			e.logger.Warn("mesh: dial failed, retrying", "attempt", attempt, "err", err)
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		mc, err := mesh.NewMeshClient(ctx, func(ctx context.Context) (io.ReadWriteCloser, error) {
			return conn.OpenStream(ctx)
		})
		if err != nil {
			conn.Close()
			lastErr = err
			e.logger.Warn("mesh: handshake failed, retrying", "attempt", attempt, "err", err)
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		tunServer.MeshClient = mc
		e.meshClient = mc // Store in engine for stats access
		if err := tunServer.AddMeshRoute(mc.MeshCIDR()); err != nil {
			e.logger.Warn("mesh: add route failed", "err", err)
		}
		e.bgWg.Add(1)
		go func() {
			defer e.bgWg.Done()
			tunServer.MeshReceiveLoop(ctx)
		}()
		e.logger.Info("mesh connected", "virtual_ip", mc.VirtualIP(), "cidr", mc.MeshCIDR())
		return mc
	}
	e.logger.Error("mesh: all attempts failed", "err", lastErr)
	return nil
}

// selector returns the current selector under read lock.
func (e *Engine) selector() *selector.Selector {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.sel
}

// Stop shuts down the engine gracefully.
func (e *Engine) Stop() error {
	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()
	return e.stopInternal()
}

// stopInternal is the lock-free core of Stop; the caller must hold lifecycleMu.
func (e *Engine) stopInternal() error {
	e.mu.Lock()
	if e.state != StateRunning {
		st := e.state
		e.mu.Unlock()
		return fmt.Errorf("engine not running (state: %s)", st)
	}
	e.state = StateStopping
	cancel := e.cancel
	closers := e.closers
	sel := e.sel
	e.mu.Unlock()

	// Cancel context first so accept loops and in-flight dials unblock,
	// then close listeners/selector (whose wg.Wait won't hang).
	if cancel != nil {
		cancel()
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, c := range closers {
			c()
		}
		if sel != nil {
			sel.Close()
		}
	}()
	select {
	case <-done:
	case <-time.After(stopTimeout):
		e.logger.Warn("engine stop timed out, forcing shutdown")
	}

	// Wait for background goroutines (speedLoop, MeshReceiveLoop) to exit.
	bgDone := make(chan struct{})
	go func() {
		e.bgWg.Wait()
		close(bgDone)
	}()
	select {
	case <-bgDone:
	case <-time.After(5 * time.Second):
		e.logger.Warn("background goroutines did not exit within timeout")
	}

	e.mu.Lock()
	if e.netMon != nil {
		e.netMon.Stop()
		e.netMon = nil
	}
	if e.geoManager != nil {
		e.geoManager.Stop()
		e.geoManager = nil
	}
	if e.chain != nil {
		e.chain.Close()
		e.chain = nil
	}
	e.state = StateStopped
	e.closers = nil
	e.sel = nil
	e.cancel = nil
	e.currentRouter = nil
	e.streamTracker = nil
	e.circuitBreaker = nil
	e.mu.Unlock()

	e.emit(Event{Type: EventDisconnected, Message: "engine stopped"})
	return nil
}

// ValidateConfig checks whether a config can start an engine
// (at least one transport enabled, server addr set).
func ValidateConfig(cfg *config.ClientConfig) error {
	if cfg.Server.Addr == "" {
		return fmt.Errorf("server address is required")
	}
	if !cfg.Transport.H3.Enabled && !cfg.Transport.Reality.Enabled && !cfg.Transport.CDN.Enabled && !cfg.Transport.WebRTC.Enabled {
		return fmt.Errorf("at least one transport must be enabled")
	}
	return nil
}

// Reload stops and restarts the engine with a new config.
// The new config is validated before stopping; if invalid the engine keeps running.
func (e *Engine) Reload(cfg *config.ClientConfig) error {
	if err := ValidateConfig(cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()

	e.mu.RLock()
	oldCfg := e.cfg
	running := e.state == StateRunning
	parentCtx := e.parentCtx
	e.mu.RUnlock()

	if parentCtx == nil {
		parentCtx = context.Background()
	}

	if running {
		if err := e.stopInternal(); err != nil {
			return fmt.Errorf("stop for reload: %w", err)
		}
	}

	e.mu.Lock()
	e.cfg = cfg
	e.mu.Unlock()

	if err := e.startInternal(parentCtx); err != nil {
		// Rollback: restore old config and try to restart
		e.mu.Lock()
		e.cfg = oldCfg
		e.mu.Unlock()
		if running {
			if rollbackErr := e.startInternal(parentCtx); rollbackErr != nil {
				e.logger.Error("rollback restart failed", "err", rollbackErr)
				e.emit(Event{Type: EventError, Error: fmt.Sprintf("reload rollback failed: %v", rollbackErr)})
				return fmt.Errorf("new config: %w; rollback failed: %v", err, rollbackErr)
			}
		}
		return fmt.Errorf("start with new config: %w", err)
	}
	return nil
}

// Status returns a snapshot of the engine's current state.
func (e *Engine) Status() EngineStatus {
	e.mu.RLock()
	state := e.state
	sel := e.sel
	mc := e.meshClient
	cfg := e.cfg
	st := e.streamTracker
	cb := e.circuitBreaker
	e.mu.RUnlock()

	stats := e.metrics.Stats()
	up, down := e.metrics.Speed()

	status := EngineStatus{
		State:         state.String(),
		ActiveConns:   stats["active_conns"],
		TotalConns:    stats["total_conns"],
		BytesSent:     stats["bytes_sent"],
		BytesReceived: stats["bytes_received"],
		UploadSpeed:   up,
		DownloadSpeed: down,
	}

	if cb != nil {
		status.CircuitState = cb.State().String()
	}

	// Add stream-level metrics summary.
	if st != nil {
		sum := st.Summary()
		status.Streams = &StreamStats{
			TotalStreams:    sum.TotalStreams,
			ActiveStreams:   sum.ActiveStreams,
			TotalBytesSent: sum.TotalBytesSent,
			TotalBytesRecv: sum.TotalBytesRecv,
			AvgDurationMs:  sum.AvgDuration.Milliseconds(),
		}
		status.TransportBreakdown = st.ByTransport()
	}

	if sel != nil {
		status.Transport = sel.ActiveTransport()
		for typ, probe := range sel.Probes() {
			status.Transports = append(status.Transports, TransportInfo{
				Type:      typ,
				Available: probe.Available,
				Latency:   probe.Latency.Milliseconds(),
			})
		}
		if paths := sel.ActivePaths(); len(paths) > 0 {
			for _, sp := range paths {
				status.MultipathPaths = append(status.MultipathPaths, PathInfo{
					Transport:     sp.Transport,
					Latency:       sp.Latency,
					ActiveStreams:  sp.ActiveStreams,
					TotalStreams:   sp.TotalStreams,
					Available:     sp.Available,
					Failures:      sp.Failures,
					BytesSent:     sp.BytesSent,
					BytesReceived: sp.BytesReceived,
				})
			}
		}
	}

	// Add mesh status
	if cfg != nil && cfg.Mesh.Enabled {
		meshStatus := &MeshStatus{Enabled: true}
		if mc != nil {
			meshStatus.VirtualIP = mc.VirtualIP().String()
			meshStatus.CIDR = mc.MeshCIDR()
			for _, peer := range mc.ListPeers() {
				mp := MeshPeer{
					VirtualIP: peer.VirtualIP,
					State:     peer.State,
					Method:    peer.Method,
				}
				if peer.Quality != nil {
					mp.AvgRTT = peer.Quality.AvgRTT.Milliseconds()
					mp.MinRTT = peer.Quality.MinRTT.Milliseconds()
					mp.MaxRTT = peer.Quality.MaxRTT.Milliseconds()
					mp.Jitter = peer.Quality.Jitter.Milliseconds()
					mp.PacketLoss = peer.Quality.LossRate
					mp.Score = peer.Quality.Score
				}
				meshStatus.Peers = append(meshStatus.Peers, mp)
			}
		}
		status.Mesh = meshStatus
	}

	return status
}

// StreamStats returns an aggregate summary of per-stream metrics.
func (e *Engine) StreamStats() stream.StreamSummary {
	e.mu.RLock()
	st := e.streamTracker
	e.mu.RUnlock()
	if st == nil {
		return stream.StreamSummary{}
	}
	return st.Summary()
}

// MultipathStats returns per-path statistics from the H3 multipath manager, or nil if multipath is not active.
func (e *Engine) MultipathStats() []h3.PathStats {
	e.mu.RLock()
	sel := e.sel
	e.mu.RUnlock()
	if sel == nil {
		return nil
	}
	for _, t := range sel.Transports() {
		if h3Client, ok := t.(*h3.Client); ok {
			if stats := h3Client.MultipathStats(); stats != nil {
				return stats
			}
		}
	}
	return nil
}

// StreamTracker returns the current stream tracker, or nil if the engine is not running.
func (e *Engine) StreamTracker() *stream.StreamTracker {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.streamTracker
}

// Config returns a deep copy of the current config.
func (e *Engine) Config() config.ClientConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return *e.cfg.DeepCopy()
}

// SetConfig updates the config without restarting the engine.
// Use this for non-critical changes like the saved server list.
func (e *Engine) SetConfig(cfg *config.ClientConfig) {
	cp := cfg.DeepCopy()
	e.mu.Lock()
	e.cfg = cp
	e.mu.Unlock()
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

// GeoManager returns the geo data manager, or nil if not enabled.
func (e *Engine) GeoManager() *geodata.Manager {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.geoManager
}

// CurrentRouter returns the active router, or nil if not running.
func (e *Engine) CurrentRouter() *router.Router {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentRouter
}

// Subscribe returns a channel that receives real-time engine events.
// The channel is buffered. Slow consumers will miss events.
func (e *Engine) Subscribe() chan Event {
	ch := make(chan Event, eventChannelBuffer)
	e.subMu.Lock()
	e.subs[ch] = struct{}{}
	e.subMu.Unlock()
	return ch
}

// Unsubscribe removes and closes a previously subscribed channel.
func (e *Engine) Unsubscribe(ch chan Event) {
	e.subMu.Lock()
	defer e.subMu.Unlock()
	if _, ok := e.subs[ch]; ok {
		delete(e.subs, ch)
		close(ch)
	}
}

func (e *Engine) emit(ev Event) {
	ev.Timestamp = time.Now()
	e.subMu.Lock()
	defer e.subMu.Unlock()
	for ch := range e.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// EmitConnectionEvent emits a connection event to all subscribers.
// This is used by plugins to report connection open/close events.
func (e *Engine) EmitConnectionEvent(connID, state, target, rule, protocol, processName string, bytesIn, bytesOut, durationMs int64) {
	e.emit(Event{
		Type:        EventConnection,
		ConnID:      connID,
		ConnState:   state,
		Target:      target,
		Rule:        rule,
		Protocol:    protocol,
		ProcessName: processName,
		BytesIn:     bytesIn,
		BytesOut:    bytesOut,
		DurationMs:  durationMs,
	})
}

func (e *Engine) speedLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			up, down := e.metrics.SampleSpeed()
			e.emit(Event{
				Type:     EventSpeedTick,
				Upload:   up,
				Download: down,
			})
		}
	}
}

// chainConn wraps a net.Conn so that closing it calls chain.OnDisconnect,
// ensuring all plugins in the chain (metrics, logger, etc.) are notified.
type chainConn struct {
	net.Conn
	chain     *plugin.Chain
	closeOnce sync.Once
}

func (c *chainConn) Close() error {
	c.closeOnce.Do(func() {
		c.chain.OnDisconnect(c.Conn)
	})
	return c.Conn.Close()
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

// shapedConn wraps a streamConn so that writes go through an obfs.Shaper
// (randomized chunking and inter-packet delays) while reads pass through unchanged.
type shapedConn struct {
	*streamConn
	shaper *obfs.Shaper
}

func (c *shapedConn) Write(b []byte) (int, error) { return c.shaper.Write(b) }

// ReadFrom disables zero-copy so that writes always go through the Shaper.
func (c *shapedConn) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(struct{ io.Writer }{c}, r)
}

// streamConn wraps a transport.Stream as a net.Conn.
type streamConn struct {
	stream transport.Stream
	addr   string
}

func (c *streamConn) Read(b []byte) (int, error)         { return c.stream.Read(b) }
func (c *streamConn) Write(b []byte) (int, error)        { return c.stream.Write(b) }
func (c *streamConn) Close() error                        { return c.stream.Close() }
func (c *streamConn) LocalAddr() net.Addr                 { return &net.TCPAddr{} }
func (c *streamConn) RemoteAddr() net.Addr                { return &net.TCPAddr{} }
func (c *streamConn) SetDeadline(t time.Time) error       { return nil }
func (c *streamConn) SetReadDeadline(t time.Time) error   { return nil }
func (c *streamConn) SetWriteDeadline(t time.Time) error  { return nil }

// ReadFrom delegates to the underlying stream's ReadFrom if available,
// preserving zero-copy (splice) capability on Linux.
func (c *streamConn) ReadFrom(r io.Reader) (int64, error) {
	if rf, ok := c.stream.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}
	return io.Copy(struct{ io.Writer }{c.stream}, r)
}

// WriteTo delegates to the underlying stream's WriteTo if available,
// preserving zero-copy (splice) capability on Linux.
func (c *streamConn) WriteTo(w io.Writer) (int64, error) {
	if wt, ok := c.stream.(io.WriterTo); ok {
		return wt.WriteTo(w)
	}
	return io.Copy(w, struct{ io.Reader }{c.stream})
}

// extractPort parses a host:port address and returns the port as uint16.
// Returns 0 if the address is malformed or the port is out of range.
func extractPort(addr string) uint16 {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	var port int
	for _, c := range portStr {
		if c < '0' || c > '9' {
			return 0
		}
		port = port*10 + int(c-'0')
		if port > 65535 {
			return 0
		}
	}
	return uint16(port)
}
