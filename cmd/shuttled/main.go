package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/shuttle-proxy/shuttle/config"
	"github.com/shuttle-proxy/shuttle/internal/pool"
	"github.com/shuttle-proxy/shuttle/congestion"
	"github.com/shuttle-proxy/shuttle/crypto"
	"github.com/shuttle-proxy/shuttle/internal/logutil"
	"github.com/shuttle-proxy/shuttle/internal/qrterm"
	"github.com/shuttle-proxy/shuttle/internal/sysopt"
	"github.com/shuttle-proxy/shuttle/mesh"
	meshsignal "github.com/shuttle-proxy/shuttle/mesh/signal"
	"github.com/shuttle-proxy/shuttle/server"
	"github.com/shuttle-proxy/shuttle/server/admin"
	"github.com/shuttle-proxy/shuttle/server/audit"
	"github.com/shuttle-proxy/shuttle/transport"
	"github.com/shuttle-proxy/shuttle/transport/cdn"
	"github.com/shuttle-proxy/shuttle/transport/h3"
	"github.com/shuttle-proxy/shuttle/transport/reality"
	rtcTransport "github.com/shuttle-proxy/shuttle/transport/webrtc"
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
	if cfg.Debug.PprofEnabled {
		pprofListen := cfg.Debug.PprofListen
		if pprofListen == "" {
			pprofListen = "127.0.0.1:6060"
		}
		go func() {
			pprofMux := http.NewServeMux()
			pprofMux.HandleFunc("/debug/pprof/", pprof.Index)
			pprofMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
			pprofMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
			pprofMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
			pprofMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
			logger.Info("pprof enabled", "addr", pprofListen)
			if err := http.ListenAndServe(pprofListen, pprofMux); err != nil { //nolint:gosec // pprof debug server, local only
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
		realityServer := reality.NewServer(&reality.ServerConfig{
			ListenAddr: cfg.Listen,
			PrivateKey: cfg.Auth.PrivateKey,
			ShortIDs:   cfg.Transport.Reality.ShortIDs,
			TargetSNI:  cfg.Transport.Reality.TargetSNI,
			TargetAddr: cfg.Transport.Reality.TargetAddr,
			CertFile:   cfg.TLS.CertFile,
			KeyFile:    cfg.TLS.KeyFile,
		}, logger)
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
		adminServer, err = admin.ListenAndServe(&cfg.Admin, adminInfo, cfg, configPath, users, auditLog)
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

	streamSem := make(chan struct{}, maxConcurrentStreams)
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
				remoteIP := extractIP(conn.RemoteAddr())
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
			connWg.Add(1)
			go func() {
				defer connWg.Done()
				handleConnection(ctx, conn, peerTable, ipAllocator, signalHub, streamSem, users, reputation, auditLog, adminInfo, logger)
				if adminInfo != nil {
					adminInfo.ActiveConns.Add(-1)
				}
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
	go func() {
		sig := <-sigCh
		logger.Warn("received second signal, forcing immediate exit", "signal", sig)
		cancel()
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

	logger.Info("shuttled stopped gracefully")
}

func handleConnection(ctx context.Context, conn transport.Connection, peerTable *mesh.PeerTable, allocator *mesh.IPAllocator, signalHub *meshsignal.Hub, streamSem chan struct{}, users *admin.UserStore, reputation *server.Reputation, auditLog *audit.Logger, adminInfo *admin.ServerInfo, logger *slog.Logger) {
	defer conn.Close()

	remoteIP := extractIP(conn.RemoteAddr())

	for {
		stream, err := conn.AcceptStream(ctx)
		if err != nil {
			if ctx.Err() == nil {
				logger.Debug("accept stream error", "err", err)
			}
			return
		}

		select {
		case streamSem <- struct{}{}:
		default:
			logger.Warn("stream limit reached, rejecting")
			stream.Close()
			continue
		}

		go func() {
			defer func() { <-streamSem }()
			handleStream(ctx, stream, remoteIP, peerTable, allocator, signalHub, users, reputation, auditLog, adminInfo, logger)
		}()
	}
}

func handleStream(ctx context.Context, stream transport.Stream, remoteIP string, peerTable *mesh.PeerTable, allocator *mesh.IPAllocator, signalHub *meshsignal.Hub, users *admin.UserStore, reputation *server.Reputation, auditLog *audit.Logger, adminInfo *admin.ServerInfo, logger *slog.Logger) {
	defer stream.Close()

	// Read target address (first line). Use a buffered approach to avoid
	// losing bytes that come after the \n delimiter in the same read.
	// Set a deadline so a slow/malicious client cannot hold a goroutine forever.
	if dl, ok := stream.(interface{ SetReadDeadline(time.Time) error }); ok {
		dl.SetReadDeadline(time.Now().Add(10 * time.Second))
	}
	buf := make([]byte, 512)
	total := 0
	for {
		n, err := stream.Read(buf[total:])
		if err != nil {
			return
		}
		total += n
		// Look for newline in what we've read so far.
		if idx := findNewline(buf[:total]); idx >= 0 {
			header := string(buf[:idx])

			// Parse header: "TOKEN:target" or just "target" (backward compatible).
			// Tokens are 64-char hex strings, so we look for the first colon
			// and try to authenticate the prefix. If it fails (or no users
			// configured), treat the entire header as the target address.
			var target string
			var user *admin.UserState
			if users != nil && users.HasUsers() {
				if colonIdx := strings.IndexByte(header, ':'); colonIdx > 0 {
					token := header[:colonIdx]
					if u := users.Authenticate(token); u != nil {
						user = u
						target = header[colonIdx+1:]
						// Record successful auth for reputation
						if reputation != nil {
							reputation.RecordSuccess(remoteIP)
						}
					} else {
						// Token didn't match — reject the stream
						if reputation != nil {
							if reputation.RecordFailure(remoteIP) {
								logger.Warn("IP banned after auth failures", "ip", remoteIP)
							}
						}
						logger.Debug("auth failed, rejecting stream", "ip", remoteIP)
						return
					}
				} else {
					// No token provided but users are configured — reject
					logger.Debug("no auth token provided, rejecting stream", "ip", remoteIP)
					if reputation != nil {
						if reputation.RecordFailure(remoteIP) {
							logger.Warn("IP banned after auth failures", "ip", remoteIP)
						}
					}
					return
				}
			} else {
				target = header
			}

			// If user authenticated, enforce quota.
			if user != nil && user.QuotaExceeded() {
				logger.Info("quota exceeded, rejecting stream", "user", user.Name)
				return
			}

			// Check for mesh magic
			if target == "MESH" && peerTable != nil && allocator != nil {
				handleMeshStream(ctx, stream, peerTable, allocator, signalHub, logger)
				return
			}

			residual := buf[idx+1 : total] // bytes after \n

			// Clear the header-read deadline before relaying data.
			if dl, ok := stream.(interface{ SetReadDeadline(time.Time) error }); ok {
				dl.SetReadDeadline(time.Time{})
			}

			logger.Debug("proxying", "target", target)

			// SSRF protection: block connections to internal/private networks
			if isBlockedTarget(target) {
				logger.Warn("blocked SSRF attempt to internal target", "target", target, "ip", remoteIP)
				return
			}

			remote, err := net.DialTimeout("tcp", target, 10*time.Second)
			if err != nil {
				logger.Debug("dial target failed", "target", target, "err", err)
				return
			}
			defer remote.Close() //nolint:gocritic // not a real loop; reads until newline then returns

			// Forward any residual bytes that were read past the header.
			if len(residual) > 0 {
				if _, err := remote.Write(residual); err != nil {
					return
				}
			}

			// If user authenticated, wrap stream with byte counting and
			// track active connections.
			var rw io.ReadWriter = stream
			var counter *countingReadWriter
			if user != nil {
				user.ActiveConns.Add(1)
				defer user.ActiveConns.Add(-1) //nolint:gocritic // not a real loop; reads until newline then returns
				counter = &countingReadWriter{
					inner: stream,
					user:  user,
				}
				rw = counter
			}

			// Relay bidirectionally.
			startTime := time.Now()
			relay(rw, remote)

			// Record audit entry after relay completes.
			if auditLog != nil {
				entry := audit.Entry{
					Timestamp:  startTime,
					Target:     target,
					DurationMs: time.Since(startTime).Milliseconds(),
				}
				if user != nil {
					entry.User = user.Name
				}
				if counter != nil {
					entry.BytesIn = counter.bytesIn.Load()
					entry.BytesOut = counter.bytesOut.Load()
				}
				if adminInfo != nil {
					adminInfo.BytesSent.Add(entry.BytesOut)
					adminInfo.BytesRecv.Add(entry.BytesIn)
				}
				auditLog.Log(&entry)
			}
			return
		}
		if total >= len(buf) {
			logger.Debug("target header too long")
			return
		}
	}
}

// countingReadWriter wraps an io.ReadWriter and updates a user's byte counters.
// It also tracks per-stream byte counts for audit logging.
type countingReadWriter struct {
	inner    io.ReadWriter
	user     *admin.UserState
	bytesIn  atomic.Int64 // bytes read (client → server)
	bytesOut atomic.Int64 // bytes written (server → client)
}

func (c *countingReadWriter) Read(p []byte) (int, error) {
	n, err := c.inner.Read(p)
	if n > 0 {
		c.bytesIn.Add(int64(n))
		c.user.BytesRecv.Add(int64(n))
	}
	return n, err
}

func (c *countingReadWriter) Write(p []byte) (int, error) {
	n, err := c.inner.Write(p)
	if n > 0 {
		c.bytesOut.Add(int64(n))
		c.user.BytesSent.Add(int64(n))
	}
	return n, err
}

func handleMeshStream(ctx context.Context, stream transport.Stream, peerTable *mesh.PeerTable, allocator *mesh.IPAllocator, signalHub *meshsignal.Hub, logger *slog.Logger) {
	ip, err := allocator.Allocate()
	if err != nil {
		logger.Error("mesh: IP allocation failed", "err", err)
		return
	}
	defer allocator.Release(ip)

	// Send handshake: IP + mask + gateway
	handshake := mesh.EncodeHandshake(ip, allocator.Mask(), allocator.Gateway())
	if _, err := stream.Write(handshake); err != nil {
		logger.Error("mesh: handshake write failed", "err", err)
		return
	}

	// Register peer with a frame-writing wrapper
	fw := &meshFrameWriter{stream: stream}
	peerTable.Register(ip, fw)
	defer peerTable.Unregister(ip)

	// Register peer with signal hub if P2P is enabled
	if signalHub != nil {
		signalHub.Register(ip, fw)
		defer signalHub.Unregister(ip)
	}

	logger.Info("mesh peer connected", "ip", ip)

	// Read frames from this peer and forward to destination peers
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		pkt, err := mesh.ReadFrame(stream)
		if err != nil {
			logger.Debug("mesh peer disconnected", "ip", ip, "err", err)
			return
		}

		// Check if this is a signaling message
		if signalHub != nil && isSignalingPacket(pkt) {
			if err := signalHub.HandleMessage(pkt, ip); err != nil {
				logger.Debug("mesh: signal handling failed", "err", err)
			}
			continue
		}

		// Regular mesh packet - forward to destination
		if !peerTable.Forward(pkt) {
			logger.Debug("mesh: no route for packet", "src", ip)
		}
	}
}

// isSignalingPacket checks if a packet is a signaling message.
// Signaling messages have a specific format starting with a type byte
// in the range 0x01-0xFF (non-IP packet).
func isSignalingPacket(pkt []byte) bool {
	if len(pkt) < meshsignal.HeaderSize {
		return false
	}
	// Check if it looks like a signaling message by checking the type
	// Valid signaling types are 0x01-0x08 and 0xFF
	msgType := pkt[0]
	switch msgType {
	case meshsignal.SignalCandidate,
		meshsignal.SignalConnect,
		meshsignal.SignalConnectAck,
		meshsignal.SignalDisconnect,
		meshsignal.SignalPing,
		meshsignal.SignalPong,
		meshsignal.SignalError:
		return true
	default:
		// Check if it starts with IPv4 version (0x4X)
		// If not, it might be a signaling message
		return (pkt[0] >> 4) != 4
	}
}

// meshFrameWriter wraps a stream to write length-prefixed frames.
type meshFrameWriter struct {
	mu     sync.Mutex
	stream io.WriteCloser
}

func (fw *meshFrameWriter) Write(p []byte) (int, error) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if err := mesh.WriteFrame(fw.stream, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (fw *meshFrameWriter) Close() error {
	return fw.stream.Close()
}

func findNewline(b []byte) int {
	return bytes.IndexByte(b, '\n')
}

// extractIP extracts the IP address (without port) from a net.Addr.
func extractIP(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	s := addr.String()
	host, _, err := net.SplitHostPort(s)
	if err != nil {
		// Try parsing as bare IP (no port)
		if ip := net.ParseIP(s); ip != nil {
			return ip.String()
		}
		return s
	}
	return host
}

// isBlockedTarget checks whether the target address points to an internal or
// private network. It resolves the hostname to IP addresses first to prevent
// DNS rebinding attacks, then checks each resolved IP against blocked ranges.
func isBlockedTarget(target string) bool {
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		// target might not have a port; try as-is
		host = target
	}

	// Resolve hostname to IPs (also handles literal IPs)
	ips, err := net.LookupHost(host)
	if err != nil {
		// If we can't resolve, check if it's a literal IP
		if ip := net.ParseIP(host); ip != nil {
			return isBlockedIP(ip)
		}
		// Can't resolve — block by default to be safe
		return true
	}

	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if isBlockedIP(ip) {
			return true
		}
	}
	return false
}

// blockedCIDRs contains the CIDR ranges that should be blocked from proxying.
var blockedCIDRs = func() []*net.IPNet {
	cidrs := []string{
		"127.0.0.0/8",    // loopback
		"10.0.0.0/8",     // private
		"172.16.0.0/12",  // private
		"192.168.0.0/16", // private
		"169.254.0.0/16", // link-local
		"0.0.0.0/8",      // unspecified
		"::1/128",        // loopback v6
		"fe80::/10",      // link-local v6
		"fc00::/7",       // unique local v6
		"::/128",         // unspecified v6
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic("invalid blocked CIDR: " + cidr)
		}
		nets = append(nets, n)
	}
	return nets
}()

func isBlockedIP(ip net.IP) bool {
	for _, n := range blockedCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func relay(a io.ReadWriter, b io.ReadWriter) {
	done := make(chan struct{}, 2)
	go func() {
		buf := pool.Get(32 * 1024) // 32KB relay buffer from pool
		_, err := io.CopyBuffer(b, a, buf)
		pool.Put(buf)
		if err != nil {
			slog.Debug("relay a→b finished", "err", err)
		}
		// Close write side if possible to signal EOF
		if cw, ok := b.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
		done <- struct{}{}
	}()
	go func() {
		buf := pool.Get(32 * 1024) // 32KB relay buffer from pool
		_, err := io.CopyBuffer(a, b, buf)
		pool.Put(buf)
		if err != nil {
			slog.Debug("relay b→a finished", "err", err)
		}
		if cw, ok := a.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
		done <- struct{}{}
	}()
	<-done
	<-done
}
