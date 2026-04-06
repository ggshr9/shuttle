//go:build darwin

package sysproxy

import (
	"fmt"
	"os/exec"
	"strings"
)

// getNetworkServices returns all network services (Wi-Fi, Ethernet, etc.)
func getNetworkServices() ([]string, error) {
	out, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return nil, fmt.Errorf("list network services: %w", err)
	}

	lines := strings.Split(string(out), "\n")
	services := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip header line and empty lines
		if line == "" || strings.HasPrefix(line, "An asterisk") {
			continue
		}
		// Skip disabled services (marked with *)
		if strings.HasPrefix(line, "*") {
			continue
		}
		services = append(services, line)
	}
	return services, nil
}

func set(cfg ProxyConfig) error {
	services, err := getNetworkServices()
	if err != nil {
		return err
	}

	for _, service := range services {
		if err := setForService(service, cfg); err != nil {
			// Log but continue with other services
			continue
		}
	}
	return nil
}

func setForService(service string, cfg ProxyConfig) error {
	if !cfg.Enable {
		// Disable all proxies
		_ = exec.Command("networksetup", "-setwebproxystate", service, "off").Run()
		_ = exec.Command("networksetup", "-setsecurewebproxystate", service, "off").Run()
		_ = exec.Command("networksetup", "-setsocksfirewallproxystate", service, "off").Run()
		return nil
	}

	// Set HTTP proxy
	if cfg.HTTPAddr != "" {
		host, port := splitHostPort(cfg.HTTPAddr)
		if host != "" && port != "" {
			// HTTP proxy
			_ = exec.Command("networksetup", "-setwebproxy", service, host, port).Run()
			_ = exec.Command("networksetup", "-setwebproxystate", service, "on").Run()
			// HTTPS proxy (uses same address)
			_ = exec.Command("networksetup", "-setsecurewebproxy", service, host, port).Run()
			_ = exec.Command("networksetup", "-setsecurewebproxystate", service, "on").Run()
		}
	}

	// Set SOCKS proxy
	if cfg.SOCKSAddr != "" {
		host, port := splitHostPort(cfg.SOCKSAddr)
		if host != "" && port != "" {
			_ = exec.Command("networksetup", "-setsocksfirewallproxy", service, host, port).Run()
			_ = exec.Command("networksetup", "-setsocksfirewallproxystate", service, "on").Run()
		}
	}

	// Set bypass domains
	if len(cfg.Bypass) > 0 {
		bypassList := strings.Join(cfg.Bypass, " ")
		_ = exec.Command("networksetup", "-setproxybypassdomains", service, bypassList).Run()
	}

	return nil
}

func splitHostPort(addr string) (host, port string) {
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return addr, ""
	}
	return addr[:idx], addr[idx+1:]
}
