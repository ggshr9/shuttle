package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/crypto"
	"github.com/shuttleX/shuttle/internal/logutil"
	"github.com/shuttleX/shuttle/internal/qrterm"
	"github.com/shuttleX/shuttle/internal/sysopt"
	"github.com/shuttleX/shuttle/server"
	"github.com/shuttleX/shuttle/update"
)

// getVersion returns the current version, set via ldflags:
//   -X github.com/shuttleX/shuttle/update.Version=v0.3.1
func getVersion() string { return update.Version }

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version", "-v", "--version":
		fmt.Printf("shuttled v%s\n", getVersion())
	case "genkey":
		genKey()
	case "init":
		initCmd := flag.NewFlagSet("init", flag.ExitOnError)
		dir := initCmd.String("dir", "", "config directory (default: /etc/shuttle or ~/.shuttle)")
		domain := initCmd.String("domain", "", "server domain name (auto-detects IP if empty)")
		password := initCmd.String("password", "", "set password (auto-generate if empty)")
		transport := initCmd.String("transport", "both", "transport: h3, reality, both")
		listen := initCmd.String("listen", config.DefaultListenPort, "listen address")
		force := initCmd.Bool("force", false, "overwrite existing config")
		meshFlag := initCmd.Bool("mesh", false, "enable mesh VPN with P2P")
		initCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: shuttled init [flags]\n\nZero-config server bootstrap. Generates keys, certificates, and config.\n\nFlags:\n")
			initCmd.PrintDefaults()
		}
		_ = initCmd.Parse(os.Args[2:])
		initServer(&initParams{
			Dir:       *dir,
			Domain:    *domain,
			Password:  *password,
			Transport: *transport,
			Listen:    *listen,
			Force:     *force,
			Mesh:      *meshFlag,
		})
	case "share":
		shareCmd := flag.NewFlagSet("share", flag.ExitOnError)
		configPath := shareCmd.String("c", "", "path to server config file (required)")
		addr := shareCmd.String("addr", "", "server address for clients (e.g. example.com:443)")
		name := shareCmd.String("name", "", "optional server display name")
		shareCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: shuttled share -c <config.yaml> --addr <domain:port>\n\nFlags:\n")
			shareCmd.PrintDefaults()
		}
		_ = shareCmd.Parse(os.Args[2:])
		if *configPath == "" {
			shareCmd.Usage()
			os.Exit(1)
		}
		share(*configPath, *addr, *name)
	case "run":
		runCmd := flag.NewFlagSet("run", flag.ExitOnError)
		configPath := runCmd.String("c", "", "path to config file (auto-detects or auto-init if empty)")
		password := runCmd.String("p", "", "password (shortcut: auto-init + run in one step)")
		listen := runCmd.String("l", "", "listen address (default :443)")
		daemon := runCmd.Bool("d", false, "install as systemd service and start in background")
		runCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage:\n  shuttled run -p yourpassword          One-step server\n  shuttled run -p password -d            Install as service + start\n  shuttled run -c config.yaml            Use existing config\n  shuttled run                           Auto-detect or auto-init\n\nFlags:\n")
			runCmd.PrintDefaults()
		}
		_ = runCmd.Parse(os.Args[2:])
		if *password != "" {
			// Brook-style: one command to init + run
			opts := &config.InitOptions{Password: *password, Force: true}
			if *listen != "" {
				opts.Listen = *listen
			}
			result, err := config.Bootstrap(opts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Init failed: %v\n", err)
				os.Exit(1)
			}
			printInitResult(result)
			*configPath = result.ConfigPath
		}
		if *daemon {
			installAndStartService(*configPath)
			return
		}
		run(*configPath)
	case "stop":
		stopService()
	case "status":
		serviceStatus()
	case "uninstall":
		uninstallService()
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
	fmt.Fprintf(os.Stderr, "Shuttled v%s — Shuttle Server\n\n", getVersion())
	fmt.Fprintf(os.Stderr, "Quick start:\n")
	fmt.Fprintf(os.Stderr, "  sudo shuttled run -p password -d        Install as service + start\n")
	fmt.Fprintf(os.Stderr, "  shuttled run -p password                Foreground mode\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  shuttled run [-c config.yaml]           Start the server\n")
	fmt.Fprintf(os.Stderr, "  shuttled stop                           Stop the service\n")
	fmt.Fprintf(os.Stderr, "  shuttled status                         Check service status\n")
	fmt.Fprintf(os.Stderr, "  shuttled uninstall                      Remove the service\n")
	fmt.Fprintf(os.Stderr, "  shuttled init                           Generate config only\n")
	fmt.Fprintf(os.Stderr, "  shuttled share -c <config> --addr host  Generate import URI\n")
	fmt.Fprintf(os.Stderr, "  shuttled genkey                         Generate key pair\n")
	fmt.Fprintf(os.Stderr, "  shuttled version                        Show version\n")
	fmt.Fprintf(os.Stderr, "  shuttled completion <shell>             Shell completions\n")
	fmt.Fprintf(os.Stderr, "  shuttled help                           Show this help\n")
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
            COMPREPLY=( $(compgen -W "--dir --domain --password --transport --listen --force --mesh" -- "$cur") )
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
                        '--force[Overwrite existing]' \
                        '--mesh[Enable mesh VPN]'
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
complete -c shuttled -n '__fish_seen_subcommand_from init' -l mesh -d 'Enable mesh VPN'
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

// initParams groups the parameters for the init subcommand.
type initParams struct {
	Dir       string
	Domain    string
	Password  string
	Transport string
	Listen    string
	Force     bool
	Mesh      bool
}

func initServer(p *initParams) {
	var transports []string
	switch p.Transport {
	case "h3":
		transports = []string{"h3"}
	case "reality":
		transports = []string{"reality"}
	default:
		transports = []string{"h3", "reality"}
	}

	opts := &config.InitOptions{
		ConfigDir:  p.Dir,
		Domain:     p.Domain,
		Password:   p.Password,
		Transports: transports,
		Listen:     p.Listen,
		Force:      p.Force,
		Mesh:       p.Mesh,
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
	if result.MeshEnabled {
		fmt.Printf("  Mesh VPN:   %s (P2P: on)\n", result.MeshCIDR)
	}
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
		Mesh:     cfg.Mesh.Enabled,
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

	logger.Info("shuttled starting", "version", getVersion())

	// Apply system optimizations
	sysopt.Apply(logger)

	// Create the server with all subsystems
	srv, err := server.New(server.Config{
		ServerConfig: cfg,
		ConfigPath:   configPath,
		Version:      getVersion(),
		Logger:       logger,
	})
	if err != nil {
		logger.Error("failed to initialize server", "err", err)
		os.Exit(1)
	}

	// Context for the main server lifecycle
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Parse drain timeout from config (default 30s)
	drainTimeout := 30 * time.Second
	if cfg.DrainTimeout != "" {
		if d, err := time.ParseDuration(cfg.DrainTimeout); err == nil {
			drainTimeout = d
		} else {
			logger.Warn("invalid drain_timeout, using default 30s", "value", cfg.DrainTimeout, "err", err)
		}
	}

	// Ensure graceful shutdown runs regardless of exit path.
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), drainTimeout)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
		logger.Info("shuttled stopped gracefully")
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start the server in a goroutine (Start blocks on accept loop)
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	// Two-phase shutdown: first signal = graceful drain, second = immediate exit
	select {
	case sig := <-sigCh:
		logger.Info("received signal, starting graceful shutdown", "signal", sig)
	case err := <-errCh:
		if err != nil {
			logger.Error("server exited with error", "err", err)
			return // defer will run shutdown
		}
	}

	// Cancel context to stop accepting new connections
	cancel()

	// Start a goroutine that forces immediate shutdown on second signal
	go func() {
		sig := <-sigCh
		logger.Warn("received second signal, forcing immediate exit", "signal", sig)
		os.Exit(1)
	}()
}
