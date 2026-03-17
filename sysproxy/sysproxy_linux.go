//go:build linux

package sysproxy

import (
	"os/exec"
	"strings"
)

func set(cfg ProxyConfig) error {
	// Try GNOME/gsettings first
	if err := setGNOME(cfg); err == nil {
		return nil
	}

	// Try KDE/kwriteconfig5
	if err := setKDE(cfg); err == nil {
		return nil
	}

	// No supported desktop environment found
	// User will need to set environment variables manually
	return nil
}

func setGNOME(cfg ProxyConfig) error {
	// Check if gsettings is available
	if _, err := exec.LookPath("gsettings"); err != nil {
		return err
	}

	if !cfg.Enable {
		// Disable proxy
		_ = exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "none").Run()
		return nil
	}

	// Set manual proxy mode
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "manual").Run()

	// Set HTTP proxy
	if cfg.HTTPAddr != "" {
		host, port := splitHostPort(cfg.HTTPAddr)
		_ = exec.Command("gsettings", "set", "org.gnome.system.proxy.http", "host", host).Run()
		_ = exec.Command("gsettings", "set", "org.gnome.system.proxy.http", "port", port).Run()
		_ = exec.Command("gsettings", "set", "org.gnome.system.proxy.https", "host", host).Run()
		_ = exec.Command("gsettings", "set", "org.gnome.system.proxy.https", "port", port).Run()
	}

	// Set SOCKS proxy
	if cfg.SOCKSAddr != "" {
		host, port := splitHostPort(cfg.SOCKSAddr)
		_ = exec.Command("gsettings", "set", "org.gnome.system.proxy.socks", "host", host).Run()
		_ = exec.Command("gsettings", "set", "org.gnome.system.proxy.socks", "port", port).Run()
	}

	// Set bypass list
	if len(cfg.Bypass) > 0 {
		bypassArg := "['" + strings.Join(cfg.Bypass, "','") + "']"
		_ = exec.Command("gsettings", "set", "org.gnome.system.proxy", "ignore-hosts", bypassArg).Run()
	}

	return nil
}

func setKDE(cfg ProxyConfig) error {
	// Check if kwriteconfig5 is available
	if _, err := exec.LookPath("kwriteconfig5"); err != nil {
		return err
	}

	if !cfg.Enable {
		// Disable proxy
		_ = exec.Command("kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "ProxyType", "0").Run()
		return nil
	}

	// Enable manual proxy
	_ = exec.Command("kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "ProxyType", "1").Run()

	// Set HTTP proxy
	if cfg.HTTPAddr != "" {
		_ = exec.Command("kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "httpProxy", "http://"+cfg.HTTPAddr).Run()
		_ = exec.Command("kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "httpsProxy", "http://"+cfg.HTTPAddr).Run()
	}

	// Set SOCKS proxy
	if cfg.SOCKSAddr != "" {
		_ = exec.Command("kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "socksProxy", "socks://"+cfg.SOCKSAddr).Run()
	}

	// Set bypass list
	if len(cfg.Bypass) > 0 {
		_ = exec.Command("kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "NoProxyFor", strings.Join(cfg.Bypass, ",")).Run()
	}

	// Notify KDE to reload settings
	_ = exec.Command("dbus-send", "--type=signal", "/KIO/Scheduler", "org.kde.KIO.Scheduler.reparseSlaveConfiguration", "string:").Run()

	return nil
}

func splitHostPort(addr string) (host, port string) {
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return addr, ""
	}
	return addr[:idx], addr[idx+1:]
}
