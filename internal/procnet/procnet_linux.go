//go:build linux

package procnet

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ListNetworkProcesses returns processes with active TCP and UDP connections.
func ListNetworkProcesses() ([]ProcInfo, error) {
	return listLinux()
}

// PortToPID returns the owning PID for the given local port (TCP then UDP fallback), or 0 if not found.
func PortToPID(port uint16) uint32 {
	if pid := portToPIDFromFile("/proc/net/tcp", port); pid > 0 {
		return pid
	}
	return portToPIDFromFile("/proc/net/udp", port)
}

func portToPIDFromFile(path string, port uint16) uint32 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	hexPort := fmt.Sprintf("%04X", port)
	for _, line := range strings.Split(string(data), "\n")[1:] {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		parts := strings.SplitN(fields[1], ":", 2)
		if len(parts) == 2 && strings.EqualFold(parts[1], hexPort) {
			inode, _ := strconv.ParseUint(fields[9], 10, 64)
			if inode > 0 {
				pid := inodeToPID(inode)
				if pid > 0 {
					return pid
				}
			}
		}
	}
	return 0
}

func listLinux() ([]ProcInfo, error) {
	inodeSet := make(map[uint64]struct{})

	// Parse both /proc/net/tcp and /proc/net/udp
	for _, path := range []string{"/proc/net/tcp", "/proc/net/udp"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n")[1:] {
			fields := strings.Fields(line)
			if len(fields) < 10 {
				continue
			}
			inode, _ := strconv.ParseUint(fields[9], 10, 64)
			if inode > 0 {
				inodeSet[inode] = struct{}{}
			}
		}
	}

	if len(inodeSet) == 0 {
		return nil, nil
	}

	// Walk /proc to find PID -> inode mapping
	pidConns := make(map[uint32]int)
	pidNames := make(map[uint32]string)

	entries, _ := os.ReadDir("/proc")
	for _, entry := range entries {
		pid, err := strconv.ParseUint(entry.Name(), 10, 32)
		if err != nil {
			continue
		}
		fdPath := filepath.Join("/proc", entry.Name(), "fd")
		fds, _ := os.ReadDir(fdPath)
		for _, fd := range fds {
			link, _ := os.Readlink(filepath.Join(fdPath, fd.Name()))
			if !strings.HasPrefix(link, "socket:[") {
				continue
			}
			inodeStr := link[8 : len(link)-1]
			inode, _ := strconv.ParseUint(inodeStr, 10, 64)
			if _, ok := inodeSet[inode]; ok {
				pidConns[uint32(pid)]++
			}
		}
		if pidConns[uint32(pid)] > 0 {
			comm, _ := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm")) //nolint:gocritic // /proc is a real filesystem path
			pidNames[uint32(pid)] = strings.TrimSpace(string(comm))
		}
	}

	result := make([]ProcInfo, 0, len(pidConns))
	for pid, conns := range pidConns {
		name := pidNames[pid]
		if name == "" {
			continue
		}
		result = append(result, ProcInfo{
			PID:   pid,
			Name:  name,
			Conns: conns,
		})
	}
	return result, nil
}

func inodeToPID(targetInode uint64) uint32 {
	entries, _ := os.ReadDir("/proc")
	for _, entry := range entries {
		pid, err := strconv.ParseUint(entry.Name(), 10, 32)
		if err != nil {
			continue
		}
		fdPath := filepath.Join("/proc", entry.Name(), "fd") //nolint:gocritic // /proc is a real filesystem path
		fds, _ := os.ReadDir(fdPath)
		for _, fd := range fds {
			link, _ := os.Readlink(filepath.Join(fdPath, fd.Name())) //nolint:gocritic // /proc is a real filesystem path
			if !strings.HasPrefix(link, "socket:[") {
				continue
			}
			inodeStr := link[8 : len(link)-1]
			inode, _ := strconv.ParseUint(inodeStr, 10, 64)
			if inode == targetInode {
				return uint32(pid)
			}
		}
	}
	return 0
}
