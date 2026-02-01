package engine

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/shuttle-proxy/shuttle/config"
	"github.com/shuttle-proxy/shuttle/congestion"
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

// Engine is the core shuttle client, managing transports, routing, and local proxies.
type Engine struct {
	mu      sync.RWMutex
	state   EngineState
	cfg     *config.ClientConfig
	logger  *slog.Logger
	metrics *plugin.Metrics

	sel    *selector.Selector
	cancel context.CancelFunc

	// Closers for local proxy servers
	closers []func() error

	// Event subscribers
	subMu sync.Mutex
	subs  []chan Event
}

// New creates a new Engine from the given config.
func New(cfg *config.ClientConfig) *Engine {
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

	return &Engine{
		state:   StateStopped,
		cfg:     cfg,
		logger:  logger,
		metrics: plugin.NewMetrics(),
	}
}

// Start initializes all subsystems and begins proxying.
func (e *Engine) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.state == StateRunning || e.state == StateStarting {
		e.mu.Unlock()
		return fmt.Errorf("engine already %s", e.state)
	}
	e.state = StateStarting
	e.mu.Unlock()

	e.emit(Event{Type: EventLog, Message: "engine starting"})

	sysopt.Apply(e.logger)

	ctx, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	e.cancel = cancel
	e.mu.Unlock()

	// --- Congestion control ---
	var cc congestion.CongestionController
	switch e.cfg.Congestion.Mode {
	case "brutal":
		rate := e.cfg.Congestion.BrutalRate
		if rate == 0 {
			rate = 100 * 1024 * 1024
		}
		cc = congestion.NewBrutal(rate)
	case "bbr":
		cc = congestion.NewBBR(0)
	default:
		cc = congestion.NewAdaptive(&congestion.AdaptiveConfig{
			BrutalRate: e.cfg.Congestion.BrutalRate,
		}, e.logger)
	}
	ccAdapter := congestion.NewQUICAdapter(cc)

	// --- GeoIP/GeoSite ---
	geoIPDB := router.NewGeoIPDB()
	geoSiteDB := router.NewGeoSiteDB()
	defaultIPEntries, defaultSiteEntries := geodata.LoadDefaults()
	for _, entry := range defaultIPEntries {
		geoIPDB.LoadFromCIDRs(entry.CountryCode, entry.CIDRs)
	}
	for _, entry := range defaultSiteEntries {
		geoSiteDB.LoadCategory(entry.Category, entry.Domains)
	}

	// --- DNS resolver ---
	dnsResolver := router.NewDNSResolver(&router.DNSConfig{
		DomesticServer: e.cfg.Routing.DNS.Domestic,
		RemoteServer:   e.cfg.Routing.DNS.Remote.Server,
		RemoteViaProxy: e.cfg.Routing.DNS.Remote.Via == "proxy",
		CacheSize:      10000,
		CacheTTL:       10 * time.Minute,
	}, geoIPDB, e.logger)

	// --- Build transports ---
	var transports []transport.ClientTransport

	if e.cfg.Transport.H3.Enabled {
		transports = append(transports, h3.NewClient(&h3.ClientConfig{
			ServerAddr:        e.cfg.Server.Addr,
			ServerName:        e.cfg.Server.SNI,
			Password:          e.cfg.Server.Password,
			PathPrefix:        e.cfg.Transport.H3.PathPrefix,
			CongestionControl: ccAdapter,
		}))
	}

	if e.cfg.Transport.Reality.Enabled {
		transports = append(transports, reality.NewClient(&reality.ClientConfig{
			ServerAddr: e.cfg.Server.Addr,
			ServerName: e.cfg.Transport.Reality.ServerName,
			ShortID:    e.cfg.Transport.Reality.ShortID,
			PublicKey:  e.cfg.Transport.Reality.PublicKey,
			Password:   e.cfg.Server.Password,
		}))
	}

	if e.cfg.Transport.CDN.Enabled {
		switch e.cfg.Transport.CDN.Mode {
		case "grpc":
			transports = append(transports, cdn.NewGRPCClient(&cdn.GRPCConfig{
				CDNDomain: e.cfg.Transport.CDN.Domain,
				Password:  e.cfg.Server.Password,
			}))
		default:
			transports = append(transports, cdn.NewH2Client(&cdn.H2Config{
				ServerAddr: e.cfg.Server.Addr,
				CDNDomain:  e.cfg.Transport.CDN.Domain,
				Path:       e.cfg.Transport.CDN.Path,
				Password:   e.cfg.Server.Password,
			}))
		}
	}

	if len(transports) == 0 {
		cancel()
		e.setState(StateStopped)
		return fmt.Errorf("no transports enabled")
	}

	sel := selector.New(transports, &selector.Config{
		Strategy: selector.StrategyAuto,
	}, e.logger)
	sel.Start(ctx)

	e.mu.Lock()
	e.sel = sel
	e.mu.Unlock()

	// --- Router ---
	routerCfg := &router.RouterConfig{
		DefaultAction: router.Action(e.cfg.Routing.Default),
	}
	for _, rule := range e.cfg.Routing.Rules {
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
	rt := router.NewRouter(routerCfg, geoIPDB, geoSiteDB, e.logger)

	// --- Dialer ---
	dialer := func(dialCtx context.Context, network, addr string) (net.Conn, error) {
		host, port, _ := net.SplitHostPort(addr)

		var ip net.IP
		if parsedIP := net.ParseIP(host); parsedIP != nil {
			ip = parsedIP
		} else {
			ips, err := dnsResolver.Resolve(dialCtx, host)
			if err != nil || len(ips) == 0 {
				ips2, err2 := net.DefaultResolver.LookupIP(dialCtx, "ip4", host)
				if err2 != nil {
					return nil, fmt.Errorf("dns resolve %s: %w", host, err)
				}
				ips = ips2
			}
			ip = ips[0]
		}

		action := rt.Match(host, ip, "", "")

		switch action {
		case router.ActionDirect:
			return (&net.Dialer{}).DialContext(dialCtx, network, net.JoinHostPort(ip.String(), port))
		case router.ActionReject:
			return nil, fmt.Errorf("rejected: %s", addr)
		default:
			conn, err := sel.Dial(dialCtx, e.cfg.Server.Addr)
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
	var closers []func() error

	// cleanup closes all already-started resources on failure.
	cleanup := func() {
		for _, c := range closers {
			c()
		}
		sel.Close()
		cancel()
		e.setState(StateStopped)
	}

	if e.cfg.Proxy.SOCKS5.Enabled {
		socks := proxy.NewSOCKS5Server(&proxy.SOCKS5Config{
			ListenAddr: e.cfg.Proxy.SOCKS5.Listen,
		}, dialer, e.logger)
		if err := socks.Start(ctx); err != nil {
			cleanup()
			return fmt.Errorf("socks5: %w", err)
		}
		closers = append(closers, socks.Close)
	}

	if e.cfg.Proxy.HTTP.Enabled {
		httpProxy := proxy.NewHTTPServer(&proxy.HTTPConfig{
			ListenAddr: e.cfg.Proxy.HTTP.Listen,
		}, dialer, e.logger)
		if err := httpProxy.Start(ctx); err != nil {
			cleanup()
			return fmt.Errorf("http proxy: %w", err)
		}
		closers = append(closers, httpProxy.Close)
	}

	if e.cfg.Proxy.TUN.Enabled {
		tunServer := proxy.NewTUNServer(&proxy.TUNConfig{
			DeviceName: e.cfg.Proxy.TUN.DeviceName,
			CIDR:       e.cfg.Proxy.TUN.CIDR,
			MTU:        e.cfg.Proxy.TUN.MTU,
			AutoRoute:  e.cfg.Proxy.TUN.AutoRoute,
		}, dialer, e.logger)
		if err := tunServer.Start(ctx); err != nil {
			e.logger.Warn("TUN device failed", "err", err)
		} else {
			closers = append(closers, tunServer.Close)
		}
	}

	e.mu.Lock()
	e.closers = closers
	e.state = StateRunning
	e.mu.Unlock()

	e.emit(Event{Type: EventConnected, Message: "engine started"})

	// Start speed sampling loop
	go e.speedLoop(ctx)

	return nil
}

// Stop shuts down the engine gracefully.
func (e *Engine) Stop() error {
	e.mu.Lock()
	if e.state != StateRunning {
		e.mu.Unlock()
		return fmt.Errorf("engine not running (state: %s)", e.state)
	}
	e.state = StateStopping
	cancel := e.cancel
	closers := e.closers
	sel := e.sel
	e.mu.Unlock()

	for _, c := range closers {
		c()
	}
	if sel != nil {
		sel.Close()
	}
	if cancel != nil {
		cancel()
	}

	e.mu.Lock()
	e.state = StateStopped
	e.closers = nil
	e.sel = nil
	e.cancel = nil
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
	if !cfg.Transport.H3.Enabled && !cfg.Transport.Reality.Enabled && !cfg.Transport.CDN.Enabled {
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

	e.mu.RLock()
	oldCfg := e.cfg
	running := e.state == StateRunning
	e.mu.RUnlock()

	if running {
		if err := e.Stop(); err != nil {
			return fmt.Errorf("stop for reload: %w", err)
		}
	}

	e.mu.Lock()
	e.cfg = cfg
	e.mu.Unlock()

	if err := e.Start(context.Background()); err != nil {
		// Rollback: restore old config and try to restart
		e.mu.Lock()
		e.cfg = oldCfg
		e.mu.Unlock()
		if running {
			if startErr := e.Start(context.Background()); startErr != nil {
				e.logger.Error("rollback restart failed", "err", startErr)
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

	if sel != nil {
		status.Transport = sel.ActiveTransport()
		for typ, probe := range sel.Probes() {
			status.Transports = append(status.Transports, TransportInfo{
				Type:      typ,
				Available: probe.Available,
				Latency:   probe.Latency.Milliseconds(),
			})
		}
	}

	return status
}

// Config returns the current config.
func (e *Engine) Config() *config.ClientConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cfg
}

// SetConfig updates the config without restarting the engine.
// Use this for non-critical changes like the saved server list.
func (e *Engine) SetConfig(cfg *config.ClientConfig) {
	e.mu.Lock()
	e.cfg = cfg
	e.mu.Unlock()
}

// Subscribe returns a channel that receives real-time engine events.
// The channel is buffered (64). Slow consumers will miss events.
func (e *Engine) Subscribe() <-chan Event {
	ch := make(chan Event, 64)
	e.subMu.Lock()
	e.subs = append(e.subs, ch)
	e.subMu.Unlock()
	return ch
}

// Unsubscribe removes a previously subscribed channel.
func (e *Engine) Unsubscribe(ch <-chan Event) {
	e.subMu.Lock()
	defer e.subMu.Unlock()
	for i, sub := range e.subs {
		if sub == ch {
			e.subs = append(e.subs[:i], e.subs[i+1:]...)
			close(sub)
			return
		}
	}
}

func (e *Engine) emit(ev Event) {
	ev.Timestamp = time.Now()
	e.subMu.Lock()
	defer e.subMu.Unlock()
	for _, ch := range e.subs {
		select {
		case ch <- ev:
		default:
			// slow consumer, drop event
		}
	}
}

func (e *Engine) setState(s EngineState) {
	e.mu.Lock()
	e.state = s
	e.mu.Unlock()
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
