//go:build darwin

package proxy

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

func (t *TUNServer) createTUN() (*os.File, error) {
	const (
		afSystem       = 32
		afSysControl   = 2
		sysprotoControl = 2
		utunControlName = "com.apple.net.utun_control"
		ctliocginfo     = 0xc0644e03
	)

	fd, err := syscall.Socket(afSystem, syscall.SOCK_DGRAM, sysprotoControl)
	if err != nil {
		return nil, fmt.Errorf("socket(AF_SYSTEM): %w", err)
	}

	var ctlInfo [100]byte
	copy(ctlInfo[4:], utunControlName)

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(ctliocginfo),
		uintptr(unsafe.Pointer(&ctlInfo[0])),
	)
	if errno != 0 {
		syscall.Close(fd)
		return nil, fmt.Errorf("ioctl CTLIOCGINFO: %w", errno)
	}

	ctlID := binary.LittleEndian.Uint32(ctlInfo[0:4])

	var sa [32]byte
	sa[0] = 32
	sa[1] = afSystem
	binary.LittleEndian.PutUint16(sa[2:4], afSysControl)
	binary.LittleEndian.PutUint32(sa[4:8], ctlID)
	binary.LittleEndian.PutUint32(sa[8:12], 0) // unit 0 = auto-assign

	_, _, errno = syscall.Syscall(
		syscall.SYS_CONNECT,
		uintptr(fd),
		uintptr(unsafe.Pointer(&sa[0])),
		uintptr(32),
	)
	if errno != 0 {
		syscall.Close(fd)
		return nil, fmt.Errorf("connect utun: %w", errno)
	}

	return os.NewFile(uintptr(fd), "utun"), nil
}

func (t *TUNServer) configureTUN() error {
	ip, _, err := net.ParseCIDR(t.config.CIDR)
	if err != nil {
		return fmt.Errorf("parse CIDR %q: %w", t.config.CIDR, err)
	}

	localIP := make(net.IP, len(ip))
	copy(localIP, ip)
	localIP[len(localIP)-1]++

	peerIP := make(net.IP, len(localIP))
	copy(peerIP, localIP)
	peerIP[len(peerIP)-1]++

	dev := t.config.DeviceName
	cmds := [][]string{
		{"ifconfig", dev, "inet", localIP.String(), peerIP.String(), "up"},
		{"ifconfig", dev, "mtu", fmt.Sprintf("%d", t.config.MTU)},
	}
	for _, args := range cmds {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %s: %w", args[0], string(out), err)
		}
	}
	return nil
}

func (t *TUNServer) setupRoutes() error {
	_, ipNet, err := net.ParseCIDR(t.config.CIDR)
	if err != nil {
		return err
	}
	localIP := make(net.IP, 4)
	copy(localIP, ipNet.IP)
	localIP[3]++
	out, err := exec.Command("route", "add", "-net", ipNet.String(), localIP.String()).CombinedOutput()
	if err != nil {
		return fmt.Errorf("route add: %s: %w", string(out), err)
	}
	t.logger.Info("routes configured", "cidr", ipNet.String(), "dev", t.config.DeviceName)
	return nil
}

func (t *TUNServer) teardownRoutes() {
	_, ipNet, err := net.ParseCIDR(t.config.CIDR)
	if err != nil {
		return
	}
	exec.Command("route", "delete", "-net", ipNet.String()).Run()
}
