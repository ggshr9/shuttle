//go:build linux

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
		cIFF_TUN   = 0x0001
		cIFF_NO_PI = 0x1000
		cTUNSETIFF = 0x400454ca
	)

	f, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open /dev/net/tun: %w (are you root?)", err)
	}

	var ifr [40]byte
	copy(ifr[:15], t.config.DeviceName)
	binary.LittleEndian.PutUint16(ifr[16:18], cIFF_TUN|cIFF_NO_PI)

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		f.Fd(),
		uintptr(cTUNSETIFF),
		uintptr(unsafe.Pointer(&ifr[0])),
	)
	if errno != 0 {
		f.Close()
		return nil, fmt.Errorf("ioctl TUNSETIFF: %w", errno)
	}

	return f, nil
}

func (t *TUNServer) configureTUN() error {
	ip, ipNet, err := net.ParseCIDR(t.config.CIDR)
	if err != nil {
		return fmt.Errorf("parse CIDR %q: %w", t.config.CIDR, err)
	}

	localIP := make(net.IP, len(ip))
	copy(localIP, ip)
	localIP[len(localIP)-1]++

	dev := t.config.DeviceName
	cidr := fmt.Sprintf("%s/%d", localIP, maskBits(ipNet.Mask))

	cmds := [][]string{
		{"ip", "addr", "add", cidr, "dev", dev},
		{"ip", "link", "set", dev, "mtu", fmt.Sprintf("%d", t.config.MTU)},
		{"ip", "link", "set", dev, "up"},
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
	out, err := exec.Command("ip", "route", "add", ipNet.String(), "dev", t.config.DeviceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip route add: %s: %w", string(out), err)
	}
	t.logger.Info("routes configured", "cidr", ipNet.String(), "dev", t.config.DeviceName)
	return nil
}

func (t *TUNServer) teardownRoutes() {
	if ipNet, err := parseCIDRNet(t.config.CIDR); err == nil {
		exec.Command("ip", "route", "del", ipNet.String(), "dev", t.config.DeviceName).Run()
	}
}

func parseCIDRNet(cidr string) (*net.IPNet, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	return ipNet, err
}
