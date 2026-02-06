//go:build windows

package autostart

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

const (
	registryPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	appKey       = "Shuttle"
)

var (
	advapi32         = syscall.NewLazyDLL("advapi32.dll")
	regOpenKeyExW    = advapi32.NewProc("RegOpenKeyExW")
	regCloseKey      = advapi32.NewProc("RegCloseKey")
	regSetValueExW   = advapi32.NewProc("RegSetValueExW")
	regDeleteValueW  = advapi32.NewProc("RegDeleteValueW")
	regQueryValueExW = advapi32.NewProc("RegQueryValueExW")
)

const (
	hkeyCurrentUser = 0x80000001
	keyRead         = 0x20019
	keyWrite        = 0x20006
	regSz           = 1
)

func isEnabled(cfg *Config) (bool, error) {
	key, err := openKey(keyRead)
	if err != nil {
		return false, nil // Key doesn't exist, not enabled
	}
	defer closeKey(key)

	// Check if value exists
	namePtr, _ := syscall.UTF16PtrFromString(appKey)
	var dataType uint32
	var dataSize uint32

	ret, _, _ := regQueryValueExW.Call(
		uintptr(key),
		uintptr(unsafe.Pointer(namePtr)),
		0,
		uintptr(unsafe.Pointer(&dataType)),
		0,
		uintptr(unsafe.Pointer(&dataSize)),
	)

	return ret == 0, nil
}

func enable(cfg *Config) error {
	key, err := openKey(keyWrite)
	if err != nil {
		return fmt.Errorf("open registry key: %w", err)
	}
	defer closeKey(key)

	// Build command line
	cmd := fmt.Sprintf(`"%s"`, cfg.AppPath)
	for _, arg := range cfg.Args {
		cmd += fmt.Sprintf(` "%s"`, arg)
	}
	if cfg.Hidden {
		cmd += " --hidden"
	}

	// Set registry value
	namePtr, _ := syscall.UTF16PtrFromString(appKey)
	valuePtr, _ := syscall.UTF16PtrFromString(cmd)
	valueBytes := unsafe.Slice((*byte)(unsafe.Pointer(valuePtr)), (len(cmd)+1)*2)

	ret, _, _ := regSetValueExW.Call(
		uintptr(key),
		uintptr(unsafe.Pointer(namePtr)),
		0,
		regSz,
		uintptr(unsafe.Pointer(&valueBytes[0])),
		uintptr(len(valueBytes)),
	)

	if ret != 0 {
		return fmt.Errorf("RegSetValueExW failed: %d", ret)
	}
	return nil
}

func disable(cfg *Config) error {
	key, err := openKey(keyWrite)
	if err != nil {
		return nil // Key doesn't exist, already disabled
	}
	defer closeKey(key)

	namePtr, _ := syscall.UTF16PtrFromString(appKey)
	regDeleteValueW.Call(
		uintptr(key),
		uintptr(unsafe.Pointer(namePtr)),
	)
	return nil
}

func openKey(access uint32) (syscall.Handle, error) {
	pathPtr, _ := syscall.UTF16PtrFromString(registryPath)
	var key syscall.Handle

	ret, _, _ := regOpenKeyExW.Call(
		hkeyCurrentUser,
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		uintptr(access),
		uintptr(unsafe.Pointer(&key)),
	)

	if ret != 0 {
		return 0, fmt.Errorf("RegOpenKeyExW failed: %d", ret)
	}
	return key, nil
}

func closeKey(key syscall.Handle) {
	regCloseKey.Call(uintptr(key))
}

// GetAutoStartArgs returns args that indicate the app was auto-started.
func GetAutoStartArgs() []string {
	for _, arg := range os.Args[1:] {
		if strings.Contains(arg, "--hidden") {
			return []string{"--hidden"}
		}
	}
	return nil
}
