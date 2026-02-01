//go:build darwin

package procnet

import (
	"os/exec"
	"strconv"
	"strings"
)

// ListNetworkProcesses returns processes with active network connections on macOS.
// Uses lsof to enumerate all TCP and UDP connections.
func ListNetworkProcesses() ([]ProcInfo, error) {
	out, err := exec.Command("lsof", "-i", "-nP", "-Fpcn").Output()
	if err != nil {
		return nil, nil
	}
	return parseLsofFpcn(string(out)), nil
}

// PortToPID returns the owning PID for the given local port, or 0 if not found.
func PortToPID(port uint16) uint32 {
	portStr := strconv.FormatUint(uint64(port), 10)
	out, err := exec.Command("lsof", "-iTCP:"+portStr, "-iUDP:"+portStr, "-nP", "-Fp").Output()
	if err != nil {
		// Try TCP-only as fallback
		out, err = exec.Command("lsof", "-iTCP:"+portStr, "-nP", "-Fp").Output()
		if err != nil {
			return 0
		}
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "p") {
			pid, _ := strconv.ParseUint(line[1:], 10, 32)
			if pid > 0 {
				return uint32(pid)
			}
		}
	}
	return 0
}

// parseLsofFpcn parses lsof -Fpcn output (field-mode: p=PID, c=command, n=name).
func parseLsofFpcn(out string) []ProcInfo {
	pidConns := make(map[uint32]int)
	pidNames := make(map[uint32]string)

	var curPID uint32
	var curName string

	for _, line := range strings.Split(out, "\n") {
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ := strconv.ParseUint(line[1:], 10, 32)
			curPID = uint32(pid)
		case 'c':
			curName = line[1:]
			if curPID > 0 && curName != "" {
				pidNames[curPID] = curName
			}
		case 'n':
			// Each 'n' line is a connection name; count it
			if curPID > 0 {
				pidConns[curPID]++
			}
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
	return result
}
