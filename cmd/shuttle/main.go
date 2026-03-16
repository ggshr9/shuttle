package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/crypto"
	"github.com/shuttleX/shuttle/engine"
	"github.com/shuttleX/shuttle/gui/api"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version", "-v", "--version":
		fmt.Printf("shuttle v%s\n", version)
	case "genkey":
		genKey()
	case "import":
		// Parse manually: find the shuttle:// URI and optional -o flag in any order.
		output := "config.yaml"
		var uri string
		args := os.Args[2:]
		for i := 0; i < len(args); i++ {
			if args[i] == "-o" && i+1 < len(args) {
				output = args[i+1]
				i++ // skip value
			} else if uri == "" {
				uri = args[i]
			}
		}
		if uri == "" {
			fmt.Fprintf(os.Stderr, "Usage: shuttle import <shuttle://...> [-o path]\n")
			os.Exit(1)
		}
		importURI(uri, output)
	case "run":
		runCmd := flag.NewFlagSet("run", flag.ExitOnError)
		configPath := runCmd.String("c", "", "path to config file (required)")
		runCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: shuttle run -c <config.yaml>\n\nFlags:\n")
			runCmd.PrintDefaults()
		}
		runCmd.Parse(os.Args[2:])
		if *configPath == "" {
			runCmd.Usage()
			os.Exit(1)
		}
		run(*configPath)
	case "api":
		apiCmd := flag.NewFlagSet("api", flag.ExitOnError)
		configPath := apiCmd.String("c", "", "path to config file (required)")
		listen := apiCmd.String("listen", "0.0.0.0:9090", "API listen address")
		autoConnect := apiCmd.Bool("auto-connect", false, "auto-connect on startup")
		apiCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: shuttle api -c <config.yaml> [--listen addr] [--auto-connect]\n\nHeadless API server (no GUI). For Docker/testing.\n\nFlags:\n")
			apiCmd.PrintDefaults()
		}
		apiCmd.Parse(os.Args[2:])
		if *configPath == "" {
			apiCmd.Usage()
			os.Exit(1)
		}
		runAPI(*configPath, *listen, *autoConnect)
	case "completion":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: shuttle completion <bash|zsh|fish>\n")
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
	fmt.Fprintf(os.Stderr, "Shuttle v%s — Break the impossible triangle\n\n", version)
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  shuttle run -c <config.yaml>            Start the client\n")
	fmt.Fprintf(os.Stderr, "  shuttle api -c <config.yaml>            Headless API server (for Docker/testing)\n")
	fmt.Fprintf(os.Stderr, "  shuttle import <shuttle://...>          Import server config from URI\n")
	fmt.Fprintf(os.Stderr, "  shuttle genkey                          Generate key pair\n")
	fmt.Fprintf(os.Stderr, "  shuttle version                         Show version\n")
	fmt.Fprintf(os.Stderr, "  shuttle completion <bash|zsh|fish>       Generate shell completions\n")
	fmt.Fprintf(os.Stderr, "  shuttle help                            Show this help\n")
}

func printCompletion(shell string) {
	switch shell {
	case "bash":
		fmt.Print(`_shuttle() {
    local cur prev commands
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    commands="run api import genkey version completion help"

    if [ $COMP_CWORD -eq 1 ]; then
        COMPREPLY=( $(compgen -W "$commands" -- "$cur") )
        return
    fi

    case "$prev" in
        run|api)
            COMPREPLY=( $(compgen -W "-c" -- "$cur") )
            ;;
        -c)
            COMPREPLY=( $(compgen -f -X '!*.yaml' -- "$cur") $(compgen -f -X '!*.yml' -- "$cur") )
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") )
            ;;
        api)
            COMPREPLY=( $(compgen -W "-c --listen --auto-connect" -- "$cur") )
            ;;
    esac
}
complete -F _shuttle shuttle
`)
	case "zsh":
		fmt.Print(`#compdef shuttle

_shuttle() {
    local -a commands
    commands=(
        'run:Start the client'
        'api:Headless API server'
        'import:Import server config from URI'
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
                api)
                    _arguments \
                        '-c[Config file]:file:_files -g "*.{yaml,yml}"' \
                        '--listen[API listen address]:addr:' \
                        '--auto-connect[Auto-connect on startup]'
                    ;;
                import)
                    _arguments \
                        '1:uri:' \
                        '-o[Output file]:file:_files -g "*.{yaml,yml}"'
                    ;;
                completion)
                    _values 'shell' bash zsh fish
                    ;;
            esac
            ;;
    esac
}

_shuttle "$@"
`)
	case "fish":
		fmt.Print(`# Fish completions for shuttle
complete -c shuttle -f
complete -c shuttle -n '__fish_use_subcommand' -a 'run' -d 'Start the client'
complete -c shuttle -n '__fish_use_subcommand' -a 'api' -d 'Headless API server'
complete -c shuttle -n '__fish_use_subcommand' -a 'import' -d 'Import server config from URI'
complete -c shuttle -n '__fish_use_subcommand' -a 'genkey' -d 'Generate key pair'
complete -c shuttle -n '__fish_use_subcommand' -a 'version' -d 'Show version'
complete -c shuttle -n '__fish_use_subcommand' -a 'completion' -d 'Generate shell completions'
complete -c shuttle -n '__fish_use_subcommand' -a 'help' -d 'Show help'
complete -c shuttle -n '__fish_seen_subcommand_from run' -s c -d 'Config file' -rF
complete -c shuttle -n '__fish_seen_subcommand_from api' -s c -d 'Config file' -rF
complete -c shuttle -n '__fish_seen_subcommand_from api' -l listen -d 'API listen address'
complete -c shuttle -n '__fish_seen_subcommand_from api' -l auto-connect -d 'Auto-connect on startup'
complete -c shuttle -n '__fish_seen_subcommand_from import' -s o -d 'Output file' -rF
complete -c shuttle -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'
`)
	default:
		fmt.Fprintf(os.Stderr, "Unsupported shell: %s (supported: bash, zsh, fish)\n", shell)
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

func importURI(uri, output string) {
	share, err := config.DecodeShareURI(uri)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid URI: %v\n", err)
		os.Exit(1)
	}
	data := config.RenderClientYAML(share)
	if err := os.WriteFile(output, []byte(data), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Config written to %s\nRun: shuttle run -c %s\n", output, output)
}

func runAPI(configPath, listen string, autoConnect bool) {
	cfg, err := config.LoadClientConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	eng := engine.New(cfg)

	// Start API server
	srv := api.NewServer(eng, nil)
	addr, err := srv.ListenAndServe(listen)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start API server: %v\n", err)
		os.Exit(1)
	}
	logger.Info("API server listening", "addr", addr)

	// Auto-connect if requested
	if autoConnect {
		if err := eng.Start(ctx); err != nil {
			logger.Error("auto-connect failed", "err", err)
		} else {
			logger.Info("auto-connected to server")
		}
	}

	<-ctx.Done()
	srv.Close()
	eng.Stop()
}

func run(configPath string) {
	cfg, err := config.LoadClientConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	eng := engine.New(cfg)
	if err := eng.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start: %v\n", err)
		os.Exit(1)
	}

	<-ctx.Done()
	eng.Stop()
}
