package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const systemdUnit = `[Unit]
Description=Shuttle Server
After=network.target

[Service]
ExecStart=%s run -c %s
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
`

func installAndStartService(configPath string) {
	if runtime.GOOS != "linux" {
		fmt.Fprintf(os.Stderr, "Daemon mode (-d) is only supported on Linux with systemd.\n")
		fmt.Fprintf(os.Stderr, "On other platforms, use a process manager or run in the foreground.\n")
		os.Exit(1)
	}

	// Resolve paths
	binPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot determine binary path: %v\n", err)
		os.Exit(1)
	}
	binPath, _ = filepath.EvalSymlinks(binPath)

	if configPath == "" {
		configPath = "/etc/shuttle/server.yaml"
	}
	configPath, _ = filepath.Abs(configPath)

	// Write systemd unit
	unit := fmt.Sprintf(systemdUnit, binPath, configPath)
	unitPath := "/etc/systemd/system/shuttled.service"
	if err := os.WriteFile(unitPath, []byte(unit), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write %s: %v\nTry with sudo.\n", unitPath, err)
		os.Exit(1)
	}

	// Reload + enable + start
	cmds := [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "shuttled"},
		{"systemctl", "start", "shuttled"},
	}
	for _, args := range cmds {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s failed: %s\n", strings.Join(args, " "), string(out))
			os.Exit(1)
		}
	}

	fmt.Println()
	fmt.Println("  Shuttle server installed as systemd service.")
	fmt.Println()
	fmt.Printf("  Config:   %s\n", configPath)
	fmt.Printf("  Service:  shuttled.service\n")
	fmt.Println()
	fmt.Println("  Commands:")
	fmt.Println("    sudo shuttled status       Check status")
	fmt.Println("    sudo shuttled stop         Stop server")
	fmt.Println("    sudo journalctl -u shuttled -f   View logs")
	fmt.Println("    sudo shuttled uninstall    Remove service")
	fmt.Println()
}

func stopService() {
	if runtime.GOOS != "linux" {
		fmt.Fprintln(os.Stderr, "Service management is only supported on Linux.")
		os.Exit(1)
	}
	out, err := exec.Command("systemctl", "stop", "shuttled").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to stop: %s\n", string(out))
		os.Exit(1)
	}
	fmt.Println("Shuttle server stopped.")
}

func serviceStatus() {
	if runtime.GOOS != "linux" {
		fmt.Fprintln(os.Stderr, "Service management is only supported on Linux.")
		os.Exit(1)
	}
	out, _ := exec.Command("systemctl", "status", "shuttled").CombinedOutput()
	fmt.Print(string(out))
}

func uninstallService() {
	if runtime.GOOS != "linux" {
		fmt.Fprintln(os.Stderr, "Service management is only supported on Linux.")
		os.Exit(1)
	}
	cmds := [][]string{
		{"systemctl", "stop", "shuttled"},
		{"systemctl", "disable", "shuttled"},
	}
	for _, args := range cmds {
		exec.Command(args[0], args[1:]...).Run()
	}
	os.Remove("/etc/systemd/system/shuttled.service")
	exec.Command("systemctl", "daemon-reload").Run()
	fmt.Println("Shuttle server service removed.")
}
