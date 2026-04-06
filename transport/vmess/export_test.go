package vmess

// ComputeAuthIDForTest exposes computeAuthID for testing.
func ComputeAuthIDForTest(uuid [16]byte, timestamp int64) [AuthIDLen]byte {
	return computeAuthID(uuid, timestamp)
}

// ParseUUIDForTest exposes parseUUID for testing.
func ParseUUIDForTest(s string) ([16]byte, error) {
	return parseUUID(s)
}
