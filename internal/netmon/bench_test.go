package netmon

import "testing"

// BenchmarkNetworkClassify benchmarks ClassifyInterface with various interface names.
func BenchmarkNetworkClassify(b *testing.B) {
	names := []string{
		"wlan0",      // WiFi
		"eth0",       // Ethernet
		"rmnet0",     // Cellular
		"en0",        // macOS WiFi
		"enp0s3",     // Linux Ethernet
		"lo",         // Unknown (loopback)
		"docker0",    // Unknown
		"wwan0",      // Cellular
		"Wi-Fi",      // WiFi (Windows)
		"en2",        // macOS Ethernet
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := names[i%len(names)]
		_ = ClassifyInterface(name)
	}
}
