//go:build freebsd

package procnet

// FreeBSD process/network introspection is not yet implemented.

func ListNetworkProcesses() ([]ProcInfo, error) {
	return nil, nil
}

func PortToPID(port uint16) uint32 {
	return 0
}
