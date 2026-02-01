package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/shuttle-proxy/shuttle/config"
	"github.com/shuttle-proxy/shuttle/crypto"
	"github.com/shuttle-proxy/shuttle/engine"
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
	fmt.Fprintf(os.Stderr, "  shuttle run -c <config.yaml>    Start the client\n")
	fmt.Fprintf(os.Stderr, "  shuttle genkey                  Generate key pair\n")
	fmt.Fprintf(os.Stderr, "  shuttle version                 Show version\n")
	fmt.Fprintf(os.Stderr, "  shuttle help                    Show this help\n")
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
