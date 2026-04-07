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
	"github.com/shuttleX/shuttle/provider"
	"github.com/shuttleX/shuttle/obfs"
	"github.com/shuttleX/shuttle/proxy"
	"github.com/shuttleX/shuttle/qos"
	"github.com/shuttleX/shuttle/router"
	"github.com/shuttleX/shuttle/router/geodata"
	"github.com/shuttleX/shuttle/stream"
	"github.com/shuttleX/shuttle/transport"
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
// ruleProviders is passed to the router for rule-provider-based matching; may be nil.
func (e *Engine) buildRouter(cfg *config.ClientConfig, ruleProviders map[string]*provider.RuleProvider) (*router.Router, *router.DNSResolver, *router.Prefetcher) {
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
		// Wire hot-reload: after Manager downloads new files, reload into DBs.
		mgr.OnUpdate = func() {
			if ipEntries, err := mgr.LoadGeoIPEntries(); err == nil {
				rEntries := make([]router.GeoIPEntry, len(ipEntries))
				for i, entry := range ipEntries {
					rEntries[i] = router.GeoIPEntry{CountryCode: entry.CountryCode, CIDRs: entry.CIDRs}
				}
				geoIPDB.Reload(rEntries)
			}
			if siteEntries, err := mgr.LoadGeoSiteEntries(); err == nil {
				rEntries := make([]router.GeoSiteEntry, len(siteEntries))
				for i, entry := range siteEntries {
					rEntries[i] = router.GeoSiteEntry{Category: entry.Category, Domains: entry.Domains}
				}
				geoSiteDB.ReloadSites(rEntries)
			}
			e.logger.Info("geodata hot-reloaded")
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
		Mode:           cfg.Routing.DNS.Mode,
		FakeIPRange:    cfg.Routing.DNS.FakeIPRange,
		FakeIPFilter:   cfg.Routing.DNS.FakeIPFilter,
	}, geoIPDB, e.logger)

	var prefetcher *router.Prefetcher
	if cfg.Routing.DNS.Prefetch {
		prefetcher = router.NewPrefetcher(dnsResolver, 100, 30*time.Second, e.logger)
		dnsResolver.SetPrefetcher(prefetcher)
	}

	routerCfg := &router.RouterConfig{
		DefaultAction: router.Action(cfg.Routing.Default),
		RuleProviders: ruleProviders,
	}

	// Map config rule chain entries to router rule chain entries.
	for i := range cfg.Routing.RuleChain {
		entry := &cfg.Routing.RuleChain[i]
		routerCfg.RuleChain = append(routerCfg.RuleChain, router.RuleChainEntry{
			Match: router.RuleMatch{
				Domain:        entry.Match.Domain,
				DomainSuffix:  entry.Match.DomainSuffix,
				DomainKeyword: entry.Match.DomainKeyword,
				GeoSite:       entry.Match.GeoSite,
				IPCIDR:        entry.Match.IPCIDR,
				GeoIP:         entry.Match.GeoIP,
				Process:       entry.Match.Process,
				Protocol:      entry.Match.Protocol,
				NetworkType:   entry.Match.NetworkType,
				RuleProvider:  entry.Match.RuleProvider,
			},
			Logic:  entry.Logic,
			Action: entry.Action,
		})
	}

	for _, rule := range cfg.Routing.Rules {
		routerCfg.Rules = append(routerCfg.Rules, router.ConfigRuleToRouterRule(rule))
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
	rc.InitialBackoff = parseDurationOr(cfg.InitialBackoff, rc.InitialBackoff)
	rc.MaxBackoff = parseDurationOr(cfg.MaxBackoff, rc.MaxBackoff)
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

// dialProxyStream dials the proxy server and opens a stream. Resilience
// (retry + circuit breaker) is applied externally via ResilientOutbound
// middleware wrapping ProxyOutbound.
func (e *Engine) dialProxyStream(
	ctx context.Context,
	serverAddr, addr, network string,
	shaperCfg obfs.ShaperConfig,
	classifier *qos.Classifier,
) (net.Conn, error) {
	curSel := e.selector()
	if curSel == nil {
		return nil, fmt.Errorf("no active selector")
	}

	conn, err := curSel.Dial(ctx, serverAddr)
	if err != nil {
		return nil, fmt.Errorf("proxy dial: %w", err)
	}
	rawStream, err := conn.OpenStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
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

// buildShaperConfig converts config.ObfsConfig into an obfs.ShaperConfig.
// Returns a zero-value (Enabled=false) config if shaping is disabled or parsing fails.
func (e *Engine) buildShaperConfig(cfg config.ObfsConfig) obfs.ShaperConfig {
	if !cfg.ShapingEnabled {
		return obfs.ShaperConfig{}
	}
	sc := obfs.DefaultShaperConfig()
	sc.Enabled = true
	sc.MinDelay = parseDurationOr(cfg.MinDelay, sc.MinDelay)
	sc.MaxDelay = parseDurationOr(cfg.MaxDelay, sc.MaxDelay)
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
		e.geoManager = geodata.NewManager(&geodata.ManagerConfig{
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
