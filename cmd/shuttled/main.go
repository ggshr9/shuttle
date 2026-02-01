package main

import (
	"context"
	"fmt"
	"io"
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
	"github.com/shuttle-proxy/shuttle/server"
	"github.com/shuttle-proxy/shuttle/transport"
	"github.com/shuttle-proxy/shuttle/transport/h3"
	"github.com/shuttle-proxy/shuttle/transport/reality"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Shuttled v%s — Shuttle Server\n\n", version)
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  shuttled run -c <config.yaml>    Start the server\n")
		fmt.Fprintf(os.Stderr, "  shuttled version                 Show version\n")
		fmt.Fprintf(os.Stderr, "  shuttled genkey                  Generate key pair\n")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Printf("shuttled v%s\n", version)
	case "genkey":
		genKey()
	case "run":
		configPath := "config/server.example.yaml"
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
	cfg, err := config.LoadServerConfig(configPath)
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

	logger.Info("shuttled starting", "version", version)

	// Apply system optimizations
	sysopt.Apply(logger)

	// Context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutting down...")
		cancel()
	}()

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

	// Start listening
	if err := ml.Start(ctx); err != nil {
		logger.Error("failed to start server", "err", err)
		os.Exit(1)
	}
	defer ml.Close()

	logger.Info("shuttled is running", "listen", cfg.Listen)

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
			go handleConnection(ctx, conn, logger)
		}
	}()

	<-ctx.Done()
	logger.Info("shuttled stopped")
}

func handleConnection(ctx context.Context, conn transport.Connection, logger *slog.Logger) {
	defer conn.Close()

	for {
		stream, err := conn.AcceptStream(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			return
		}
		go handleStream(ctx, stream, logger)
	}
}

func handleStream(ctx context.Context, stream transport.Stream, logger *slog.Logger) {
	defer stream.Close()

	// Read target address (first line)
	buf := make([]byte, 512)
	n, err := stream.Read(buf)
	if err != nil {
		return
	}

	// Parse target
	target := string(buf[:n])
	for i, b := range target {
		if b == '\n' || b == '\r' {
			target = target[:i]
			break
		}
	}

	logger.Debug("proxying", "target", target)

	// Connect to target
	remote, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		logger.Debug("dial target failed", "target", target, "err", err)
		return
	}
	defer remote.Close()

	// Relay bidirectionally
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(remote, stream)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(stream, remote)
		done <- struct{}{}
	}()
	<-done
}
