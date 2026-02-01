package engine

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/shuttle-proxy/shuttle/config"
	"github.com/shuttle-proxy/shuttle/congestion"
	"github.com/shuttle-proxy/shuttle/internal/procnet"
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

const eventChannelBuffer = 64

// Engine is the core shuttle client, managing transports, routing, and local proxies.
type Engine struct {
	mu      sync.RWMutex
	state   EngineState
	cfg     *config.ClientConfig
	logger  *slog.Logger
	metrics *plugin.Metrics

	sel       *selector.Selector
	cancel    context.CancelFunc
	parentCtx context.Context // stored for Reload

	// Closers for local proxy servers
	closers []func() error

	// Event subscribers — stores bidirectional channels, Subscribe returns receive-only view
	subMu sync.Mutex
	subs  map[chan Event]struct{}
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
		subs:    make(map[chan Event]struct{}),
	}
}

// Start initializes all subsystems and begins proxying.
func (e *Engine) Start(ctx context.Context) error {
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

	sysopt.Apply(e.logger)

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
		return fail(fmt.Errorf("no transports enabled"))
	}

	sel := selector.New(transports, &selector.Config{
		Strategy: selector.StrategyAuto,
	}, e.logger)
	sel.Start(ctx)

	e.mu.Lock()
	e.sel = sel
	e.cancel = cancel
	e.mu.Unlock()

	rt, dnsResolver := e.buildRouter(cfgSnap)
	dialer := e.createDialer(cfgSnap, rt, dnsResolver)

	closers, err := e.startProxies(ctx, cfgSnap, dialer, sel, cancel)
	if err != nil {
		return err
	}

	e.mu.Lock()
	e.closers = closers
	e.state = StateRunning
	e.mu.Unlock()

	e.emit(Event{Type: EventConnected, Message: "engine started"})
	go e.speedLoop(ctx)

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
		transports = append(transports, h3.NewClient(&h3.ClientConfig{
			ServerAddr:        cfg.Server.Addr,
			ServerName:        cfg.Server.SNI,
			Password:          cfg.Server.Password,
			PathPrefix:        cfg.Transport.H3.PathPrefix,
			CongestionControl: ccAdapter,
		}))
	}

	if cfg.Transport.Reality.Enabled {
		transports = append(transports, reality.NewClient(&reality.ClientConfig{
			ServerAddr: cfg.Server.Addr,
			ServerName: cfg.Transport.Reality.ServerName,
			ShortID:    cfg.Transport.Reality.ShortID,
			PublicKey:  cfg.Transport.Reality.PublicKey,
			Password:   cfg.Server.Password,
		}))
	}

	if cfg.Transport.CDN.Enabled {
		switch cfg.Transport.CDN.Mode {
		case "grpc":
			transports = append(transports, cdn.NewGRPCClient(&cdn.GRPCConfig{
				CDNDomain: cfg.Transport.CDN.Domain,
				Password:  cfg.Server.Password,
			}))
		default:
			transports = append(transports, cdn.NewH2Client(&cdn.H2Config{
				ServerAddr: cfg.Server.Addr,
				CDNDomain:  cfg.Transport.CDN.Domain,
				Path:       cfg.Transport.CDN.Path,
				Password:   cfg.Server.Password,
			}))
		}
	}

	return transports
}

// buildRouter creates the routing engine including GeoIP/GeoSite and DNS.
func (e *Engine) buildRouter(cfg *config.ClientConfig) (*router.Router, *router.DNSResolver) {
	geoIPDB := router.NewGeoIPDB()
	geoSiteDB := router.NewGeoSiteDB()
	defaultIPEntries, defaultSiteEntries := geodata.LoadDefaults()
	for _, entry := range defaultIPEntries {
		geoIPDB.LoadFromCIDRs(entry.CountryCode, entry.CIDRs)
	}
	for _, entry := range defaultSiteEntries {
		geoSiteDB.LoadCategory(entry.Category, entry.Domains)
	}

	dnsResolver := router.NewDNSResolver(&router.DNSConfig{
		DomesticServer: cfg.Routing.DNS.Domestic,
		RemoteServer:   cfg.Routing.DNS.Remote.Server,
		RemoteViaProxy: cfg.Routing.DNS.Remote.Via == "proxy",
		CacheSize:      10000,
		CacheTTL:       10 * time.Minute,
	}, geoIPDB, e.logger)

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
	return rt, dnsResolver
}

// createDialer builds the proxy dialer function.
func (e *Engine) createDialer(cfg *config.ClientConfig, rt *router.Router, dnsResolver *router.DNSResolver) func(context.Context, string, string) (net.Conn, error) {
	serverAddr := cfg.Server.Addr
	return func(dialCtx context.Context, network, addr string) (net.Conn, error) {
		host, port, _ := net.SplitHostPort(addr)

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
			conn, err := curSel.Dial(dialCtx, serverAddr)
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
		}, dialer, e.logger)
		if err := tunServer.Start(ctx); err != nil {
			e.logger.Warn("TUN device failed", "err", err)
		} else {
			closers = append(closers, tunServer.Close)
		}
	}

	return closers, nil
}

// selector returns the current selector under read lock.
func (e *Engine) selector() *selector.Selector {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.sel
}

// Stop shuts down the engine gracefully.
func (e *Engine) Stop() error {
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
	parentCtx := e.parentCtx
	e.mu.RUnlock()

	if parentCtx == nil {
		parentCtx = context.Background()
	}

	if running {
		if err := e.Stop(); err != nil {
			return fmt.Errorf("stop for reload: %w", err)
		}
	}

	e.mu.Lock()
	e.cfg = cfg
	e.mu.Unlock()

	if err := e.Start(parentCtx); err != nil {
		// Rollback: restore old config and try to restart
		e.mu.Lock()
		e.cfg = oldCfg
		e.mu.Unlock()
		if running {
			if startErr := e.Start(parentCtx); startErr != nil {
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
