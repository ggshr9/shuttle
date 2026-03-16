package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/congestion"
	"github.com/shuttleX/shuttle/crypto"
	"github.com/shuttleX/shuttle/internal/logutil"
	"github.com/shuttleX/shuttle/internal/qrterm"
	"github.com/shuttleX/shuttle/internal/sysopt"
	"github.com/shuttleX/shuttle/mesh"
	meshsignal "github.com/shuttleX/shuttle/mesh/signal"
	"github.com/shuttleX/shuttle/server"
	"github.com/shuttleX/shuttle/server/admin"
	"github.com/shuttleX/shuttle/server/audit"
	"github.com/shuttleX/shuttle/server/metrics"
	"github.com/shuttleX/shuttle/transport/cdn"
	"github.com/shuttleX/shuttle/transport/h3"
	"github.com/shuttleX/shuttle/transport/reality"
	rtcTransport "github.com/shuttleX/shuttle/transport/webrtc"
)

const (
	version         = "0.1.0"
	maxConcurrentStreams = 1024
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version", "-v", "--version":
		fmt.Printf("shuttled v%s\n", version)
	case "genkey":
		genKey()
	case "init":
		initCmd := flag.NewFlagSet("init", flag.ExitOnError)
		dir := initCmd.String("dir", "", "config directory (default: /etc/shuttle or ~/.shuttle)")
		domain := initCmd.String("domain", "", "server domain name (auto-detects IP if empty)")
		password := initCmd.String("password", "", "set password (auto-generate if empty)")
		transport := initCmd.String("transport", "both", "transport: h3, reality, both")
		listen := initCmd.String("listen", ":443", "listen address")
		force := initCmd.Bool("force", false, "overwrite existing config")
		initCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: shuttled init [flags]\n\nZero-config server bootstrap. Generates keys, certificates, and config.\n\nFlags:\n")
			initCmd.PrintDefaults()
		}
		initCmd.Parse(os.Args[2:])
		initServer(*dir, *domain, *password, *transport, *listen, *force)
	case "share":
		shareCmd := flag.NewFlagSet("share", flag.ExitOnError)
		configPath := shareCmd.String("c", "", "path to server config file (required)")
		addr := shareCmd.String("addr", "", "server address for clients (e.g. example.com:443)")
		name := shareCmd.String("name", "", "optional server display name")
		shareCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: shuttled share -c <config.yaml> --addr <domain:port>\n\nFlags:\n")
			shareCmd.PrintDefaults()
		}
		shareCmd.Parse(os.Args[2:])
		if *configPath == "" {
			shareCmd.Usage()
			os.Exit(1)
		}
		share(*configPath, *addr, *name)
	case "run":
		runCmd := flag.NewFlagSet("run", flag.ExitOnError)
		configPath := runCmd.String("c", "", "path to config file (auto-detects or auto-init if empty)")
		runCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: shuttled run [-c <config.yaml>]\n\nIf -c is not provided, looks for config in /etc/shuttle/ or ~/.shuttle/.\nIf no config found, auto-initializes with defaults.\n\nFlags:\n")
			runCmd.PrintDefaults()
		}
		runCmd.Parse(os.Args[2:])
		run(*configPath)
	case "completion":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: shuttled completion <bash|zsh|fish>\n")
			os.Exit(1)
		}
		printCompletion(os.Args[2])
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Shuttled v%s — Shuttle Server\n\n", version)
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  shuttled init                    Zero-config server setup (generates everything)\n")
	fmt.Fprintf(os.Stderr, "  shuttled run [-c config.yaml]    Start the server (auto-init if no config)\n")
	fmt.Fprintf(os.Stderr, "  shuttled share -c <config.yaml> --addr <domain:port>  Generate import URI\n")
	fmt.Fprintf(os.Stderr, "  shuttled genkey                  Generate key pair\n")
	fmt.Fprintf(os.Stderr, "  shuttled version                 Show version\n")
	fmt.Fprintf(os.Stderr, "  shuttled completion <shell>      Generate shell completions\n")
	fmt.Fprintf(os.Stderr, "  shuttled help                    Show this help\n")
}

func printCompletion(shell string) {
	switch shell {
	case "bash":
		fmt.Print(`_shuttled() {
    local cur prev commands
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    commands="init run share genkey version completion help"

    if [ $COMP_CWORD -eq 1 ]; then
        COMPREPLY=( $(compgen -W "$commands" -- "$cur") )
        return
    fi

    case "$prev" in
        run|share)
            COMPREPLY=( $(compgen -W "-c" -- "$cur") )
            ;;
        -c)
            COMPREPLY=( $(compgen -f -X '!*.yaml' -- "$cur") $(compgen -f -X '!*.yml' -- "$cur") )
            ;;
        init)
            COMPREPLY=( $(compgen -W "--dir --domain --password --transport --listen --force" -- "$cur") )
            ;;
        --transport)
            COMPREPLY=( $(compgen -W "h3 reality both" -- "$cur") )
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") )
            ;;
    esac
}
complete -F _shuttled shuttled
`)
	case "zsh":
		fmt.Print(`#compdef shuttled

_shuttled() {
    local -a commands
    commands=(
        'init:Zero-config server setup'
        'run:Start the server'
        'share:Generate import URI'
        'genkey:Generate key pair'
        'version:Show version'
        'completion:Generate shell completions'
        'help:Show help'
    )

    _arguments -C \
        '1:command:->command' \
        '*::arg:->args'

    case $state in
        command)
            _describe 'command' commands
            ;;
        args)
            case $words[1] in
                run)
                    _arguments '-c[Config file]:file:_files -g "*.{yaml,yml}"'
                    ;;
                init)
                    _arguments \
                        '--dir[Config directory]:dir:_directories' \
                        '--domain[Server domain]:domain:' \
                        '--password[Password]:password:' \
                        '--transport[Transport]:transport:(h3 reality both)' \
                        '--listen[Listen address]:addr:' \
                        '--force[Overwrite existing]'
                    ;;
                share)
                    _arguments \
                        '-c[Config file]:file:_files -g "*.{yaml,yml}"' \
                        '--addr[Server address]:addr:' \
                        '--name[Display name]:name:'
                    ;;
                completion)
                    _values 'shell' bash zsh fish
                    ;;
            esac
            ;;
    esac
}

_shuttled "$@"
`)
	case "fish":
		fmt.Print(`# Fish completions for shuttled
complete -c shuttled -f
complete -c shuttled -n '__fish_use_subcommand' -a 'init' -d 'Zero-config server setup'
complete -c shuttled -n '__fish_use_subcommand' -a 'run' -d 'Start the server'
complete -c shuttled -n '__fish_use_subcommand' -a 'share' -d 'Generate import URI'
complete -c shuttled -n '__fish_use_subcommand' -a 'genkey' -d 'Generate key pair'
complete -c shuttled -n '__fish_use_subcommand' -a 'version' -d 'Show version'
complete -c shuttled -n '__fish_use_subcommand' -a 'completion' -d 'Generate shell completions'
complete -c shuttled -n '__fish_use_subcommand' -a 'help' -d 'Show help'
complete -c shuttled -n '__fish_seen_subcommand_from run' -s c -d 'Config file' -rF
complete -c shuttled -n '__fish_seen_subcommand_from init' -l dir -d 'Config directory' -rF
complete -c shuttled -n '__fish_seen_subcommand_from init' -l domain -d 'Server domain'
complete -c shuttled -n '__fish_seen_subcommand_from init' -l password -d 'Password'
complete -c shuttled -n '__fish_seen_subcommand_from init' -l transport -d 'Transport' -a 'h3 reality both'
complete -c shuttled -n '__fish_seen_subcommand_from init' -l listen -d 'Listen address'
complete -c shuttled -n '__fish_seen_subcommand_from init' -l force -d 'Overwrite existing'
complete -c shuttled -n '__fish_seen_subcommand_from share' -s c -d 'Config file' -rF
complete -c shuttled -n '__fish_seen_subcommand_from share' -l addr -d 'Server address'
complete -c shuttled -n '__fish_seen_subcommand_from share' -l name -d 'Display name'
complete -c shuttled -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'
`)
	default:
		fmt.Fprintf(os.Stderr, "Unsupported shell: %s (supported: bash, zsh, fish)\n", shell)
		os.Exit(1)
	}
}

func initServer(dir, domain, password, transport, listen string, force bool) {
	var transports []string
	switch transport {
	case "h3":
		transports = []string{"h3"}
	case "reality":
		transports = []string{"reality"}
	default:
		transports = []string{"h3", "reality"}
	}

	opts := &config.InitOptions{
		ConfigDir:  dir,
		Domain:     domain,
		Password:   password,
		Transports: transports,
		Listen:     listen,
		Force:      force,
	}

	result, err := config.Bootstrap(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Init failed: %v\n", err)
		os.Exit(1)
	}

	printInitResult(result)
}

func printInitResult(result *config.InitResult) {
	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════╗")
	fmt.Println("  ║       Shuttle Server — Ready!            ║")
	fmt.Println("  ╚══════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("  Config:     %s\n", result.ConfigPath)
	fmt.Printf("  Server:     %s\n", result.ServerAddr)
	fmt.Printf("  Password:   %s\n", result.Password)
	fmt.Printf("  Admin API:  http://127.0.0.1:9090/api/ (token: %s...)\n", result.AdminToken[:8])
	fmt.Println()
	fmt.Println("  ── Import URI (share with clients) ──")
	fmt.Println()
	fmt.Printf("  %s\n", result.ShareURI)
	fmt.Println()
	fmt.Println("  ── QR Code (scan with Shuttle app) ──")
	fmt.Println()
	qrterm.Print(os.Stdout, result.ShareURI)
	fmt.Println()
	fmt.Println("  ── Next Steps ──")
	fmt.Println()
	fmt.Printf("  Start:   shuttled run -c %s\n", result.ConfigPath)
	fmt.Println("  Client:  shuttle import \"<URI above>\"")
	fmt.Println("  Or paste the URI in Shuttle GUI -> Servers -> Import")
	fmt.Println()
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

func share(configPath, addr, name string) {
	cfg, err := config.LoadServerConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if addr == "" {
		addr = cfg.Listen
	}

	s := &config.ShareURI{
		Addr:     addr,
		Password: cfg.Auth.Password,
		Name:     name,
	}

	// Determine transport type
	h3Enabled := cfg.Transport.H3.Enabled
	realityEnabled := cfg.Transport.Reality.Enabled
	switch {
	case h3Enabled && realityEnabled:
		s.Transport = "both"
	case h3Enabled:
		s.Transport = "h3"
	case realityEnabled:
		s.Transport = "reality"
	}

	// Reality-specific fields
	if realityEnabled {
		s.PublicKey = cfg.Auth.PublicKey
		s.SNI = cfg.Transport.Reality.TargetSNI
		if len(cfg.Transport.Reality.ShortIDs) > 0 {
			s.ShortID = cfg.Transport.Reality.ShortIDs[0]
		}
	}

	fmt.Println(config.EncodeShareURI(s))
}

func run(configPath string) {
	// Auto-detect or auto-init config
	if configPath == "" {
		configPath = config.FindDefaultConfig()
		if configPath == "" {
			fmt.Fprintln(os.Stderr, "No config found. Auto-initializing...")
			result, err := config.Bootstrap(nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Auto-init failed: %v\n", err)
				os.Exit(1)
			}
			configPath = result.ConfigPath
			printInitResult(result)
			fmt.Fprintln(os.Stderr, "Starting server with auto-generated config...")
		}
	}

	cfg, err := config.LoadServerConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Setup logger
	logger := logutil.NewLogger(cfg.Log.Level, cfg.Log.Format)
	slog.SetDefault(logger)

	logger.Info("shuttled starting", "version", version)

	// Apply system optimizations
	sysopt.Apply(logger)

	// Start pprof server if enabled
	var pprofServer *http.Server
	if cfg.Debug.PprofEnabled {
		pprofListen := cfg.Debug.PprofListen
		if pprofListen == "" {
			pprofListen = "127.0.0.1:6060"
		}
		pprofMux := http.NewServeMux()
		pprofMux.HandleFunc("/debug/pprof/", pprof.Index)
		pprofMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		pprofMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		pprofMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		pprofMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		pprofServer = &http.Server{Addr: pprofListen, Handler: pprofMux}
		go func() {
			logger.Info("pprof enabled", "addr", pprofListen)
			if err := pprofServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("pprof server failed", "err", err)
			}
		}()
	}

	// Context for the main server lifecycle
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// --- Build congestion control for server-side QUIC ---
	adaptive := congestion.NewAdaptive(nil, logger)
	ccAdapter := congestion.NewQUICAdapter(adaptive)

	// Setup cover site
	coverHandler := server.NewCoverHandler(&server.CoverConfig{
		Mode:       cfg.Cover.Mode,
		StaticDir:  cfg.Cover.StaticDir,
		ReverseURL: cfg.Cover.ReverseURL,
	}, logger)

	// Create multi-listener
	ml := server.NewMultiListener(&server.ListenerConfig{
		ListenAddr: cfg.Listen,
	}, logger)

	// Register transports
	if cfg.Transport.H3.Enabled {
		h3Server := h3.NewServer(&h3.ServerConfig{
			ListenAddr:        cfg.Listen,
			CertFile:          cfg.TLS.CertFile,
			KeyFile:           cfg.TLS.KeyFile,
			Password:          cfg.Auth.Password,
			PathPrefix:        cfg.Transport.H3.PathPrefix,
			CoverSite:         coverHandler,
			CongestionControl: ccAdapter,
		}, logger)
		ml.AddTransport(h3Server)
	}

	if cfg.Transport.Reality.Enabled {
		realityServer, err := reality.NewServer(&reality.ServerConfig{
			ListenAddr: cfg.Listen,
			PrivateKey: cfg.Auth.PrivateKey,
			ShortIDs:   cfg.Transport.Reality.ShortIDs,
			TargetSNI:  cfg.Transport.Reality.TargetSNI,
			TargetAddr: cfg.Transport.Reality.TargetAddr,
			CertFile:   cfg.TLS.CertFile,
			KeyFile:    cfg.TLS.KeyFile,
			Yamux:      &cfg.Yamux,
		}, logger)
		if err != nil {
			logger.Error("reality transport init failed", "err", err)
			fmt.Fprintf(os.Stderr, "Reality transport: %v\n", err)
			os.Exit(1)
		}
		ml.AddTransport(realityServer)
	}

	if cfg.Transport.CDN.Enabled {
		cdnListen := cfg.Transport.CDN.Listen
		if cdnListen == "" {
			cdnListen = cfg.Listen
		}
		cdnServer := cdn.NewServer(&cdn.ServerConfig{
			ListenAddr: cdnListen,
			CertFile:   cfg.TLS.CertFile,
			KeyFile:    cfg.TLS.KeyFile,
			Password:   cfg.Auth.Password,
			Path:       cfg.Transport.CDN.Path,
		}, logger)
		ml.AddTransport(cdnServer)
	}

	if cfg.Transport.WebRTC.Enabled {
		webrtcServer := rtcTransport.NewServer(&rtcTransport.ServerConfig{
			SignalListen: cfg.Transport.WebRTC.SignalListen,
			CertFile:     cfg.TLS.CertFile,
			KeyFile:      cfg.TLS.KeyFile,
			Password:     cfg.Auth.Password,
			STUNServers:  cfg.Transport.WebRTC.STUNServers,
			TURNServers:  cfg.Transport.WebRTC.TURNServers,
			TURNUser:     cfg.Transport.WebRTC.TURNUser,
			TURNPass:     cfg.Transport.WebRTC.TURNPass,
			ICEPolicy:    cfg.Transport.WebRTC.ICEPolicy,
		}, logger)
		ml.AddTransport(webrtcServer)
	}

	// Start listening
	if err := ml.Start(ctx); err != nil {
		logger.Error("failed to start server", "err", err)
		os.Exit(1)
	}

	// Setup mesh if enabled
	var peerTable *mesh.PeerTable
	var ipAllocator *mesh.IPAllocator
	var signalHub *meshsignal.Hub
	if cfg.Mesh.Enabled {
		var err error
		ipAllocator, err = mesh.NewIPAllocator(cfg.Mesh.CIDR)
		if err != nil {
			logger.Error("failed to create mesh IP allocator", "err", err)
			os.Exit(1)
		}
		peerTable = mesh.NewPeerTable(logger)
		logger.Info("mesh enabled", "cidr", cfg.Mesh.CIDR)

		// Create signal hub for P2P signaling if enabled
		if cfg.Mesh.P2PEnabled {
			signalHub = meshsignal.NewHub(logger)
			logger.Info("mesh P2P signaling enabled")
		}
	}

	// Create IP reputation tracker if enabled
	var reputation *server.Reputation
	if cfg.Reputation.Enabled {
		maxFailures := cfg.Reputation.MaxFailures
		if maxFailures <= 0 {
			maxFailures = 5
		}
		reputation = server.NewReputation(server.ReputationConfig{
			Enabled:     true,
			MaxFailures: maxFailures,
		})
		// Periodic cleanup of expired records
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					reputation.Cleanup()
				}
			}
		}()
		logger.Info("IP reputation tracking enabled", "max_failures", maxFailures)
	}

	// Create user store (shared between admin API and stream handler)
	users := admin.NewUserStore(cfg.Admin.Users)

	// Create audit logger if enabled
	var auditLog *audit.Logger
	if cfg.Audit.Enabled {
		var err error
		auditLog, err = audit.NewLogger(cfg.Audit.LogDir, cfg.Audit.MaxEntries)
		if err != nil {
			logger.Error("failed to create audit logger", "err", err)
		} else {
			logger.Info("audit logging enabled", "log_dir", cfg.Audit.LogDir)
		}
	}

	// Create metrics collector
	metricsCollector := metrics.NewCollector()

	// Start admin API if enabled
	var adminInfo *admin.ServerInfo
	var adminServer *http.Server
	if cfg.Admin.Enabled {
		adminInfo = &admin.ServerInfo{
			StartTime:  time.Now(),
			Version:    version,
			ConfigPath: configPath,
		}
		var err error
		adminServer, err = admin.ListenAndServe(&cfg.Admin, adminInfo, cfg, configPath, users, auditLog, metricsCollector)
		if err != nil {
			logger.Error("failed to start admin API", "err", err)
		} else {
			logger.Info("admin API listening", "addr", cfg.Admin.Listen)
		}
	}

	// Start cluster manager if enabled
	var cluster *server.ClusterManager
	if cfg.Cluster.Enabled {
		clusterPeers := make([]server.PeerConfig, len(cfg.Cluster.Peers))
		for i, p := range cfg.Cluster.Peers {
			clusterPeers[i] = server.PeerConfig{Name: p.Name, Addr: p.Addr}
		}
		clusterInfo := &server.ClusterNodeInfo{Version: version}
		if adminInfo != nil {
			clusterInfo.ActiveConns = &adminInfo.ActiveConns
			clusterInfo.TotalConns = &adminInfo.TotalConns
			clusterInfo.BytesSent = &adminInfo.BytesSent
			clusterInfo.BytesRecv = &adminInfo.BytesRecv
		}
		cluster = server.NewClusterManager(&server.ClusterConfig{
			Enabled:  true,
			NodeName: cfg.Cluster.NodeName,
			Secret:   cfg.Cluster.Secret,
			Peers:    clusterPeers,
			Interval: cfg.Cluster.Interval,
			MaxConns: cfg.Cluster.MaxConns,
		}, clusterInfo, logger)
		cluster.Start(ctx)
		logger.Info("cluster enabled", "node", cfg.Cluster.NodeName, "peers", len(cfg.Cluster.Peers))
	}

	logger.Info("shuttled is running", "listen", cfg.Listen)

	// Create the connection handler with all dependencies
	h := &server.Handler{
		Users:      users,
		Reputation: reputation,
		AuditLog:   auditLog,
		PeerTable:  peerTable,
		Allocator:  ipAllocator,
		SignalHub:  signalHub,
		Metrics:    metricsCollector,
		AdminInfo:  adminInfo,
		StreamSem:  make(chan struct{}, maxConcurrentStreams),
		Logger:     logger,
	}

	var connWg sync.WaitGroup

	// Handle connections
	go func() {
		for {
			conn, err := ml.Accept(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				logger.Error("accept error", "err", err)
				continue
			}

			// Check IP reputation before processing connection
			if reputation != nil {
				remoteIP := server.ExtractIP(conn.RemoteAddr())
				if reputation.IsBanned(remoteIP) {
					logger.Debug("rejecting banned IP", "ip", remoteIP)
					conn.Close()
					continue
				}
			}

			if adminInfo != nil {
				adminInfo.TotalConns.Add(1)
				adminInfo.ActiveConns.Add(1)
			}

			// Determine transport name from connection metadata
			transportName := "unknown"
			if tn, ok := conn.(interface{ TransportName() string }); ok {
				transportName = tn.TransportName()
			}
			metricsCollector.ConnOpened(transportName)
			connStart := time.Now()

			connWg.Add(1)
			go func() {
				defer connWg.Done()
				h.HandleConnection(ctx, conn)
				if adminInfo != nil {
					adminInfo.ActiveConns.Add(-1)
				}
				metricsCollector.ConnClosed(transportName, time.Since(connStart))
			}()
		}
	}()

	// Two-phase shutdown: first signal = graceful drain, second = immediate exit
	sig := <-sigCh
	logger.Info("received signal, starting graceful shutdown", "signal", sig)

	// Phase 1: Stop accepting new connections
	ml.Close()

	// Parse drain timeout from config (default 30s)
	drainTimeout := 30 * time.Second
	if cfg.DrainTimeout != "" {
		if d, err := time.ParseDuration(cfg.DrainTimeout); err == nil {
			drainTimeout = d
		} else {
			logger.Warn("invalid drain_timeout, using default 30s", "value", cfg.DrainTimeout, "err", err)
		}
	}

	// Start a goroutine that forces immediate exit on second signal
	shutdownDone := make(chan struct{})
	go func() {
		select {
		case sig := <-sigCh:
			logger.Warn("received second signal, forcing immediate exit", "signal", sig)
			cancel()
		case <-shutdownDone:
		}
	}()

	// Wait for active connections to finish, with drain timeout
	drainDone := make(chan struct{})
	go func() {
		connWg.Wait()
		close(drainDone)
	}()

	select {
	case <-drainDone:
		logger.Info("all connections drained")
	case <-time.After(drainTimeout):
		logger.Warn("drain timeout reached, closing remaining connections", "timeout", drainTimeout)
	case <-ctx.Done():
		logger.Warn("forced shutdown, closing remaining connections")
	}

	// Cancel context to stop any remaining connection handlers
	cancel()

	// Shut down pprof server gracefully
	if pprofServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := pprofServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("pprof server shutdown error", "err", err)
		}
		shutdownCancel()
	}

	// Shut down admin server gracefully
	if adminServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := adminServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("admin server shutdown error", "err", err)
		}
		shutdownCancel()
	}

	// Stop cluster manager
	if cluster != nil {
		cluster.Stop()
	}

	// Close audit logger
	if auditLog != nil {
		auditLog.Close()
	}

	close(shutdownDone)
	logger.Info("shuttled stopped gracefully")
}
