//go:build windows

package sysproxy

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

const (
	internetOptionRefresh         = 37
	internetOptionSettingsChanged = 39
	internetOptionPerConnFlags    = 75
	internetOptionProxyServer     = 38

	proxyTypeDirect = 0x00000001
	proxyTypeProxy  = 0x00000002
)

var (
	wininet                       = syscall.NewLazyDLL("wininet.dll")
	internetSetOptionW            = wininet.NewProc("InternetSetOptionW")
	internetQueryOptionW          = wininet.NewProc("InternetQueryOptionW")
)

// internetPerConnOptionList is the structure for proxy settings
type internetPerConnOptionList struct {
	dwSize        uint32
	pszConnection *uint16
	dwOptionCount uint32
	dwOptionError uint32
	pOptions      *internetPerConnOption
}

type internetPerConnOption struct {
	dwOption uint32
	value    uint64 // Union: can be dwValue, pszValue, ftValue
}

const (
	internetPerConnFlags                  = 1
	internetPerConnProxyServer            = 2
	internetPerConnProxyBypass            = 3
	internetPerConnAutoconfigUrl          = 4
	internetPerConnAutoDiscovery          = 5
	internetPerConnAutoconfigSecondaryUrl = 6
	internetPerConnAutoconfigReloadDelay  = 7
	internetPerConnAutoconfgFlag          = 8
)

const (
	proxyTypeAutoProxy = 0x00000004
	proxyTypeAutoConf  = 0x00000008
)

func set(cfg ProxyConfig) error {
	if !cfg.Enable {
		return setWindowsProxy("", "")
	}

	// Build proxy string
	var parts []string
	if cfg.HTTPAddr != "" {
		parts = append(parts, "http="+cfg.HTTPAddr)
		parts = append(parts, "https="+cfg.HTTPAddr)
	}
	if cfg.SOCKSAddr != "" {
		parts = append(parts, "socks="+cfg.SOCKSAddr)
	}

	proxyServer := strings.Join(parts, ";")
	bypass := strings.Join(cfg.Bypass, ";")

	return setWindowsProxy(proxyServer, bypass)
}

func setWindowsProxy(proxyServer, bypass string) error {
	// Use registry approach for reliability
	return setProxyViaRegistry(proxyServer, bypass)
}

func setProxyViaRegistry(proxyServer, bypass string) error {
	key, _, err := openRegKey(`Software\Microsoft\Windows\CurrentVersion\Internet Settings`)
	if err != nil {
		return fmt.Errorf("open registry key: %w", err)
	}
	defer closeRegKey(key)

	if proxyServer == "" {
		// Disable proxy
		if err := setRegDWORD(key, "ProxyEnable", 0); err != nil {
			return err
		}
	} else {
		// Enable proxy
		if err := setRegDWORD(key, "ProxyEnable", 1); err != nil {
			return err
		}
		if err := setRegString(key, "ProxyServer", proxyServer); err != nil {
			return err
		}
		if bypass != "" {
			if err := setRegString(key, "ProxyOverride", bypass); err != nil {
				return err
			}
		}
	}

	// Notify system of settings change
	notifyProxyChange()
	return nil
}

var (
	advapi32       = syscall.NewLazyDLL("advapi32.dll")
	regOpenKeyExW  = advapi32.NewProc("RegOpenKeyExW")
	regCloseKey    = advapi32.NewProc("RegCloseKey")
	regSetValueExW = advapi32.NewProc("RegSetValueExW")
)

const (
	hkeyCurrentUser = 0x80000001
	keyWrite        = 0x20006
	regSz           = 1
	regDword        = 4
)

func openRegKey(path string) (syscall.Handle, bool, error) {
	pathPtr, _ := syscall.UTF16PtrFromString(path)
	var key syscall.Handle
	ret, _, _ := regOpenKeyExW.Call(
		uintptr(hkeyCurrentUser),
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		uintptr(keyWrite),
		uintptr(unsafe.Pointer(&key)),
	)
	if ret != 0 {
		return 0, false, fmt.Errorf("RegOpenKeyExW failed: %d", ret)
	}
	return key, false, nil
}

func closeRegKey(key syscall.Handle) {
	regCloseKey.Call(uintptr(key))
}

func setRegDWORD(key syscall.Handle, name string, value uint32) error {
	namePtr, _ := syscall.UTF16PtrFromString(name)
	ret, _, _ := regSetValueExW.Call(
		uintptr(key),
		uintptr(unsafe.Pointer(namePtr)),
		0,
		regDword,
		uintptr(unsafe.Pointer(&value)),
		4,
	)
	if ret != 0 {
		return fmt.Errorf("RegSetValueExW DWORD failed: %d", ret)
	}
	return nil
}

func setRegString(key syscall.Handle, name, value string) error {
	namePtr, _ := syscall.UTF16PtrFromString(name)
	valuePtr, _ := syscall.UTF16PtrFromString(value)
	valueBytes := unsafe.Slice((*byte)(unsafe.Pointer(valuePtr)), (len(value)+1)*2)
	ret, _, _ := regSetValueExW.Call(
		uintptr(key),
		uintptr(unsafe.Pointer(namePtr)),
		0,
		regSz,
		uintptr(unsafe.Pointer(&valueBytes[0])),
		uintptr(len(valueBytes)),
	)
	if ret != 0 {
		return fmt.Errorf("RegSetValueExW string failed: %d", ret)
	}
	return nil
}

func notifyProxyChange() {
	internetSetOptionW.Call(0, internetOptionSettingsChanged, 0, 0)
	internetSetOptionW.Call(0, internetOptionRefresh, 0, 0)
}
