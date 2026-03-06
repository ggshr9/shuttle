package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/shuttle-proxy/shuttle/config"
	"github.com/shuttle-proxy/shuttle/congestion"
	"github.com/shuttle-proxy/shuttle/crypto"
	"github.com/shuttle-proxy/shuttle/internal/sysopt"
	"github.com/shuttle-proxy/shuttle/mesh"
	meshsignal "github.com/shuttle-proxy/shuttle/mesh/signal"
	"github.com/shuttle-proxy/shuttle/server"
	"github.com/shuttle-proxy/shuttle/transport"
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
	case "run":
		runCmd := flag.NewFlagSet("run", flag.ExitOnError)
		configPath := runCmd.String("c", "", "path to config file (required)")
		runCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: shuttled run -c <config.yaml>\n\nFlags:\n")
			runCmd.PrintDefaults()
		}
		runCmd.Parse(os.Args[2:])
		if *configPath == "" {
			runCmd.Usage()
			os.Exit(1)
		}
		run(*configPath)
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
	fmt.Fprintf(os.Stderr, "  shuttled run -c <config.yaml>    Start the server\n")
	fmt.Fprintf(os.Stderr, "  shuttled genkey                  Generate key pair\n")
	fmt.Fprintf(os.Stderr, "  shuttled version                 Show version\n")
	fmt.Fprintf(os.Stderr, "  shuttled help                    Show this help\n")
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
	defer ml.Close()

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

	logger.Info("shuttled is running", "listen", cfg.Listen)

	streamSem := make(chan struct{}, maxConcurrentStreams)

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
			go handleConnection(ctx, conn, peerTable, ipAllocator, signalHub, streamSem, logger)
		}
	}()

	<-ctx.Done()
	logger.Info("shuttled stopped")
}

func handleConnection(ctx context.Context, conn transport.Connection, peerTable *mesh.PeerTable, allocator *mesh.IPAllocator, signalHub *meshsignal.Hub, streamSem chan struct{}, logger *slog.Logger) {
	defer conn.Close()

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
			handleStream(ctx, stream, peerTable, allocator, signalHub, logger)
		}()
	}
}

func handleStream(ctx context.Context, stream transport.Stream, peerTable *mesh.PeerTable, allocator *mesh.IPAllocator, signalHub *meshsignal.Hub, logger *slog.Logger) {
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
			target := string(buf[:idx])

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

			remote, err := net.DialTimeout("tcp", target, 10*time.Second)
			if err != nil {
				logger.Debug("dial target failed", "target", target, "err", err)
				return
			}
			defer remote.Close()

			// Forward any residual bytes that were read past the header.
			if len(residual) > 0 {
				if _, err := remote.Write(residual); err != nil {
					return
				}
			}

			// Relay bidirectionally.
			relay(stream, remote)
			return
		}
		if total >= len(buf) {
			logger.Debug("target header too long")
			return
		}
	}
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
			if err := signalHub.HandleMessage(pkt); err != nil {
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

func relay(a io.ReadWriter, b io.ReadWriter) {
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(b, a)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(a, b)
		done <- struct{}{}
	}()
	<-done
	<-done
}
