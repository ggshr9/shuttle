package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"os/signal"
	"syscall"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/crypto"
	"github.com/shuttleX/shuttle/engine"
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
		fmt.Printf("shuttle v%s\n", getVersion())
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
		configPath := runCmd.String("c", "", "path to config file")
		serverAddr := runCmd.String("s", "", "server address (shortcut: generate config from server+password)")
		password := runCmd.String("p", "", "password (use with -s for quick connect)")
		runCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage:\n  shuttle run -s server:443 -p password   Quick connect (like Brook)\n  shuttle run -c config.yaml              Use existing config\n\nFlags:\n")
			runCmd.PrintDefaults()
		}
		_ = runCmd.Parse(os.Args[2:])
		if *serverAddr != "" && *password != "" {
			// Brook-style: quick connect without config file
			cfg := config.DefaultClientConfig()
			cfg.Server.Addr = *serverAddr
			cfg.Server.Password = *password
			tmpPath := filepath.Join(os.TempDir(), "shuttle-quick.yaml")
			if err := config.SaveClientConfig(tmpPath, cfg); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create config: %v\n", err)
				os.Exit(1)
			}
			*configPath = tmpPath
			fmt.Fprintf(os.Stderr, "Connecting to %s...\n", *serverAddr)
		}
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
		_ = apiCmd.Parse(os.Args[2:])
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
	fmt.Fprintf(os.Stderr, "Shuttle v%s — Break the impossible triangle\n\n", getVersion())
	fmt.Fprintf(os.Stderr, "Quick start:\n")
	fmt.Fprintf(os.Stderr, "  shuttle run -s server:443 -p password   Connect to a server\n")
	fmt.Fprintf(os.Stderr, "  shuttle import \"shuttle://...\"          Import from URI and run\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  shuttle run -c <config.yaml>            Start with config file\n")
	fmt.Fprintf(os.Stderr, "  shuttle api -c <config.yaml>            Headless API server\n")
	fmt.Fprintf(os.Stderr, "  shuttle import <shuttle://...>          Import server config\n")
	fmt.Fprintf(os.Stderr, "  shuttle genkey                          Generate key pair\n")
	fmt.Fprintf(os.Stderr, "  shuttle version                         Show version\n")
	fmt.Fprintf(os.Stderr, "  shuttle completion <bash|zsh|fish>      Shell completions\n")
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

func run(configPath string) {
	cfg, err := config.LoadClientConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	eng := engine.New(cfg)
	if err := eng.Start(ctx); err != nil {
		cancel()
		fmt.Fprintf(os.Stderr, "Failed to start: %v\n", err)
		os.Exit(1)
	}

	<-ctx.Done()
	_ = eng.Stop()
}
