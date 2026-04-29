//go:build linux

package sysproxy

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// errToolMissing signals that the controlling tool (gsettings/kwriteconfig5)
// is not installed, so the caller may try the next desktop environment.
// Real exec failures use a different error and are surfaced directly.
var errToolMissing = errors.New("sysproxy: tool not installed")

func set(cfg ProxyConfig) error {
	// Try GNOME/gsettings first.
	err := setGNOME(cfg)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errToolMissing) {
		return err
	}

	// Fall through to KDE only if gsettings was missing.
	err = setKDE(cfg)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errToolMissing) {
		return err
	}

	return errors.New("sysproxy: no supported desktop environment found (need gsettings or kwriteconfig5)")
}

// runOrErr runs cmd and wraps any failure with a context label that
// callers can present to the user; ExitError carries stderr if available.
func runOrErr(label string, cmd *exec.Cmd) error {
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w (%s)", label, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func setGNOME(cfg ProxyConfig) error {
	if _, err := exec.LookPath("gsettings"); err != nil {
		return errToolMissing
	}

	if !cfg.Enable {
		return runOrErr("gsettings disable",
			exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "none"))
	}

	if err := runOrErr("gsettings mode",
		exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "manual")); err != nil {
		return err
	}

	if cfg.HTTPAddr != "" {
		host, port := splitHostPort(cfg.HTTPAddr)
		for _, scheme := range []string{"http", "https"} {
			if err := runOrErr("gsettings "+scheme+" host",
				exec.Command("gsettings", "set", "org.gnome.system.proxy."+scheme, "host", host)); err != nil {
				return err
			}
			if err := runOrErr("gsettings "+scheme+" port",
				exec.Command("gsettings", "set", "org.gnome.system.proxy."+scheme, "port", port)); err != nil {
				return err
			}
		}
	}

	if cfg.SOCKSAddr != "" {
		host, port := splitHostPort(cfg.SOCKSAddr)
		if err := runOrErr("gsettings socks host",
			exec.Command("gsettings", "set", "org.gnome.system.proxy.socks", "host", host)); err != nil {
			return err
		}
		if err := runOrErr("gsettings socks port",
			exec.Command("gsettings", "set", "org.gnome.system.proxy.socks", "port", port)); err != nil {
			return err
		}
	}

	if len(cfg.Bypass) > 0 {
		bypassArg := "['" + strings.Join(cfg.Bypass, "','") + "']"
		if err := runOrErr("gsettings bypass",
			exec.Command("gsettings", "set", "org.gnome.system.proxy", "ignore-hosts", bypassArg)); err != nil {
			return err
		}
	}

	return nil
}

func setKDE(cfg ProxyConfig) error {
	if _, err := exec.LookPath("kwriteconfig5"); err != nil {
		return errToolMissing
	}

	if !cfg.Enable {
		return runOrErr("kwriteconfig5 disable",
			exec.Command("kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "ProxyType", "0"))
	}

	if err := runOrErr("kwriteconfig5 mode",
		exec.Command("kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "ProxyType", "1")); err != nil {
		return err
	}

	if cfg.HTTPAddr != "" {
		if err := runOrErr("kwriteconfig5 httpProxy",
			//nolint:gosec // G204: input is from validated ProxyConfig, not user-tainted
			exec.Command("kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "httpProxy", "http://"+cfg.HTTPAddr)); err != nil {
			return err
		}
		if err := runOrErr("kwriteconfig5 httpsProxy",
			//nolint:gosec // G204: input is from validated ProxyConfig, not user-tainted
			exec.Command("kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "httpsProxy", "http://"+cfg.HTTPAddr)); err != nil {
			return err
		}
	}

	if cfg.SOCKSAddr != "" {
		if err := runOrErr("kwriteconfig5 socksProxy",
			//nolint:gosec // G204: input is from validated ProxyConfig, not user-tainted
			exec.Command("kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "socksProxy", "socks://"+cfg.SOCKSAddr)); err != nil {
			return err
		}
	}

	if len(cfg.Bypass) > 0 {
		if err := runOrErr("kwriteconfig5 NoProxyFor",
			//nolint:gosec // G204: input is from validated ProxyConfig bypass list, not user-tainted
			exec.Command("kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "NoProxyFor", strings.Join(cfg.Bypass, ","))); err != nil {
			return err
		}
	}

	// dbus-send is best-effort: KDE picks up file changes anyway, the
	// signal just speeds up reload. Don't fail set() on a missing
	// dbus-send (e.g. KDE-without-dbus is rare but possible).
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
