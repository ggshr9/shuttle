package procnet

import (
	"syscall"
	"unsafe"
)

var (
	modiphlpapi              = syscall.NewLazyDLL("iphlpapi.dll")
	modkernel32              = syscall.NewLazyDLL("kernel32.dll")
	procGetExtendedTcpTable  = modiphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdpTable  = modiphlpapi.NewProc("GetExtendedUdpTable")
	procOpenProcess          = modkernel32.NewProc("OpenProcess")
	procCloseHandle          = modkernel32.NewProc("CloseHandle")
	procQueryFullProcessName = modkernel32.NewProc("QueryFullProcessImageNameW")
)

const (
	tcpTableOwnerPidAll = 5
	udpTableOwnerPid    = 1
	afINET              = 2
	processQueryInfo    = 0x0400
	processVMRead       = 0x0010
)

// MIB_TCPROW_OWNER_PID from iphlpapi
type tcpRowOwnerPID struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32
	RemoteAddr uint32
	RemotePort uint32
	OwningPID  uint32
}

// MIB_UDPROW_OWNER_PID from iphlpapi
type udpRowOwnerPID struct {
	LocalAddr uint32
	LocalPort uint32
	OwningPID uint32
}

// ListNetworkProcesses returns processes with active TCP and UDP connections on Windows.
func ListNetworkProcesses() ([]ProcInfo, error) {
	pidConns := make(map[uint32]int)

	tcpRows, err := getTCPTable()
	if err == nil {
		for _, row := range tcpRows {
			pidConns[row.OwningPID]++
		}
	}

	udpRows, _ := getUDPTable()
	for _, row := range udpRows {
		pidConns[row.OwningPID]++
	}

	// Resolve PID -> process name
	result := make([]ProcInfo, 0, len(pidConns))
	for pid, conns := range pidConns {
		if pid == 0 {
			continue
		}
		name := getProcessName(pid)
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

func getTCPTable() ([]tcpRowOwnerPID, error) {
	var size uint32
	// First call to get required buffer size
	procGetExtendedTcpTable.Call(0, uintptr(unsafe.Pointer(&size)), 1, afINET, tcpTableOwnerPidAll, 0)

	buf := make([]byte, size)
	ret, _, err := procGetExtendedTcpTable.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		1, afINET, tcpTableOwnerPidAll, 0,
	)
	if ret != 0 {
		return nil, err
	}

	count := *(*uint32)(unsafe.Pointer(&buf[0]))
	rows := make([]tcpRowOwnerPID, count)
	rowSize := unsafe.Sizeof(tcpRowOwnerPID{})
	for i := uint32(0); i < count; i++ {
		offset := 4 + uintptr(i)*rowSize
		rows[i] = *(*tcpRowOwnerPID)(unsafe.Pointer(&buf[offset]))
	}
	return rows, nil
}

// PortToPID returns the owning PID for the given local port (TCP then UDP fallback), or 0 if not found.
func PortToPID(port uint16) uint32 {
	rows, err := getTCPTable()
	if err == nil {
		for _, row := range rows {
			// LocalPort is in network byte order (big-endian) stored in uint32
			lp := uint16((row.LocalPort>>8)&0xff) | uint16((row.LocalPort&0xff)<<8)
			if lp == port {
				return row.OwningPID
			}
		}
	}
	// Fallback to UDP table
	udpRows, err := getUDPTable()
	if err == nil {
		for _, row := range udpRows {
			lp := uint16((row.LocalPort>>8)&0xff) | uint16((row.LocalPort&0xff)<<8)
			if lp == port {
				return row.OwningPID
			}
		}
	}
	return 0
}

func getUDPTable() ([]udpRowOwnerPID, error) {
	var size uint32
	procGetExtendedUdpTable.Call(0, uintptr(unsafe.Pointer(&size)), 1, afINET, udpTableOwnerPid, 0)

	buf := make([]byte, size)
	ret, _, err := procGetExtendedUdpTable.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		1, afINET, udpTableOwnerPid, 0,
	)
	if ret != 0 {
		return nil, err
	}

	count := *(*uint32)(unsafe.Pointer(&buf[0]))
	rows := make([]udpRowOwnerPID, count)
	rowSize := unsafe.Sizeof(udpRowOwnerPID{})
	for i := uint32(0); i < count; i++ {
		offset := 4 + uintptr(i)*rowSize
		rows[i] = *(*udpRowOwnerPID)(unsafe.Pointer(&buf[offset]))
	}
	return rows, nil
}

func getProcessName(pid uint32) string {
	handle, _, _ := procOpenProcess.Call(processQueryInfo|processVMRead, 0, uintptr(pid))
	if handle == 0 {
		return ""
	}
	defer procCloseHandle.Call(handle)

	var buf [260]uint16
	size := uint32(len(buf))
	ret, _, _ := procQueryFullProcessName.Call(handle, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if ret == 0 {
		return ""
	}
	fullPath := syscall.UTF16ToString(buf[:size])
	// Extract just the filename
	for i := len(fullPath) - 1; i >= 0; i-- {
		if fullPath[i] == '\\' || fullPath[i] == '/' {
			return fullPath[i+1:]
		}
	}
	return fullPath
}
