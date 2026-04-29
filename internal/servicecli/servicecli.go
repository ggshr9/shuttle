// Package servicecli provides shared subcommand handlers for shuttle and shuttled.
package servicecli

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/ggshr9/shuttle/internal/paths"
	"github.com/ggshr9/shuttle/service"
)

// Options identifies which binary is calling in.
type Options struct {
	Name         string        // "shuttled" or "shuttle"
	DisplayName  string        // "Shuttle Server" / "Shuttle Client"
	DefaultScope service.Scope // System for shuttled, User for shuttle
}

// Install runs the install subcommand.
func Install(opts Options, args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	configPath := fs.String("c", "", "config file path (required)")
	scopeFlag := fs.String("scope", scopeToString(opts.DefaultScope), "service scope: system|user")
	ui := fs.String("ui", "", "Web UI listen addr (e.g. :9090)")
	_ = fs.Parse(args)

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "install requires -c <config>")
		os.Exit(1)
	}
	abs, _ := filepath.Abs(*configPath)
	if _, err := os.Stat(abs); err != nil {
		fmt.Fprintf(os.Stderr, "config not found: %s\n", abs)
		os.Exit(1)
	}

	scope := parseScope(*scopeFlag)
	bin := currentBinary()

	mgr := mustManager(opts.Name, scope)
	svcArgs := []string{"run", "-c", abs}
	// For shuttled, ensureUIToken already writes Admin.{Enabled,Listen,Token}
	// into the config file, so the service reads it automatically at startup.
	// Only shuttle's run subcommand accepts --ui; shuttled's does not.
	if *ui != "" && opts.Name != "shuttled" {
		svcArgs = append(svcArgs, "--ui", *ui)
	}

	cfg := service.Config{
		Name:        opts.Name,
		DisplayName: opts.DisplayName,
		Description: opts.DisplayName,
		BinaryPath:  bin,
		Args:        svcArgs,
		Scope:       scope,
		Restart:     true,
		RestartSec:  5,
		LimitNOFILE: 65535,
		LogDir:      paths.Resolve(toPathsScope(scope)).LogDir,
	}
	var uiURL string
	if *ui != "" {
		tok, err := ensureUIToken(abs, *ui, opts.Name == "shuttled")
		if err != nil {
			exit("ui token: %v", err)
		}
		uiURL = fmt.Sprintf("http://%s/?token=%s", displayHost(*ui), tok)
	}

	if err := mgr.Install(&cfg); err != nil {
		exit("install: %v", err)
	}
	if err := mgr.Start(); err != nil {
		exit("start: %v", err)
	}
	fmt.Printf("%s installed and started.\n  Config: %s\n  Stop:   %s stop\n  Logs:   %s logs -f\n",
		opts.DisplayName, abs, opts.Name, opts.Name)
	if uiURL != "" {
		fmt.Printf("  Web UI: %s\n", uiURL)
	}
}

// Uninstall runs the uninstall subcommand.
func Uninstall(opts Options, args []string) {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	scopeFlag := fs.String("scope", scopeToString(opts.DefaultScope), "service scope")
	purge := fs.Bool("purge", false, "also remove config and log directories")
	_ = fs.Parse(args)
	scope := parseScope(*scopeFlag)
	mgr := mustManager(opts.Name, scope)
	if err := mgr.Uninstall(*purge); err != nil {
		exit("uninstall: %v", err)
	}
	fmt.Printf("%s removed.\n", opts.DisplayName)
}

// Start runs the start subcommand.
func Start(opts Options, args []string) {
	simple(opts, args, "started", func(m service.Manager) error { return m.Start() })
}

// Stop runs the stop subcommand.
func Stop(opts Options, args []string) {
	simple(opts, args, "stopped", func(m service.Manager) error { return m.Stop() })
}

// Restart runs the restart subcommand.
func Restart(opts Options, args []string) {
	simple(opts, args, "restarted", func(m service.Manager) error { return m.Restart() })
}

// Status prints the service status.
func Status(opts Options, args []string) {
	scope := scopeFromArgs(opts, args)
	mgr := mustManager(opts.Name, scope)
	s, err := mgr.Status()
	if err != nil {
		exit("status: %v", err)
	}
	fmt.Println(s)
}

// Logs tails the service logs.
func Logs(opts Options, args []string) {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	scopeFlag := fs.String("scope", scopeToString(opts.DefaultScope), "service scope")
	follow := fs.Bool("f", false, "follow")
	_ = fs.Parse(args)
	scope := parseScope(*scopeFlag)
	mgr := mustManager(opts.Name, scope)
	if err := mgr.Logs(*follow); err != nil {
		exit("logs: %v", err)
	}
}

// --- helpers ---

func simple(opts Options, args []string, pastTense string, fn func(service.Manager) error) {
	scope := scopeFromArgs(opts, args)
	mgr := mustManager(opts.Name, scope)
	if err := fn(mgr); err != nil {
		exit("%v", err)
	}
	fmt.Printf("%s %s.\n", opts.DisplayName, pastTense)
}

func scopeFromArgs(opts Options, args []string) service.Scope {
	fs := flag.NewFlagSet("scope", flag.ContinueOnError)
	fs.SetOutput(nopWriter{})
	scopeFlag := fs.String("scope", scopeToString(opts.DefaultScope), "")
	_ = fs.Parse(args)
	return parseScope(*scopeFlag)
}

func mustManager(name string, scope service.Scope) service.Manager {
	mgr, err := service.New(name, scope)
	if err != nil {
		exit("service: %v", err)
	}
	return mgr
}

func currentBinary() string {
	bin, _ := os.Executable()
	if r, err := filepath.EvalSymlinks(bin); err == nil {
		bin = r
	}
	return bin
}

func parseScope(s string) service.Scope {
	switch s {
	case "user":
		return service.ScopeUser
	default:
		return service.ScopeSystem
	}
}

func scopeToString(s service.Scope) string {
	if s == service.ScopeUser {
		return "user"
	}
	return "system"
}

// toPathsScope maps service.Scope to paths.Scope. Duplicate of
// service.scopeToPaths which is unexported. Accept the small duplication
// to keep service's helper unexported.
func toPathsScope(s service.Scope) paths.Scope {
	if s == service.ScopeUser {
		return paths.ScopeUser
	}
	return paths.ScopeSystem
}

func exit(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// displayHost normalizes a listen address for user display by replacing empty
// or wildcard hosts with 127.0.0.1. The service still binds on the original
// address; this is for UI URL printing only.
func displayHost(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
