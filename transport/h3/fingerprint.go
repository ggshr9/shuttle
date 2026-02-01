package h3

// This file defines Chrome-compatible QUIC and TLS fingerprint parameters
// to make Shuttle connections indistinguishable from real Chrome HTTP/3 traffic.

import (
	"crypto/tls"
)

// ChromeQUICVersions are QUIC versions Chrome supports.
var ChromeQUICVersions = []uint32{
	0x00000001, // QUIC v1 (RFC 9000)
}

// ChromeALPN is the ALPN list Chrome sends for HTTP/3.
var ChromeALPN = []string{"h3"}

// ChromeTLSALPN is the full ALPN for TLS-over-TCP fallback.
var ChromeTLSALPN = []string{"h3", "h2", "http/1.1"}

// ChromeCipherSuites mirrors Chrome's TLS 1.3 cipher suite preferences.
var ChromeCipherSuites = []uint16{
	tls.TLS_AES_128_GCM_SHA256,
	tls.TLS_AES_256_GCM_SHA384,
	tls.TLS_CHACHA20_POLY1305_SHA256,
}

// ChromeCurvePreferences mirrors Chrome's elliptic curve preferences.
var ChromeCurvePreferences = []tls.CurveID{
	tls.X25519,
	tls.CurveP256,
	tls.CurveP384,
}

// QUICTransportParams mirrors Chrome's default QUIC transport parameters.
type QUICTransportParams struct {
	MaxIdleTimeout                 uint64 // 30s default
	MaxUDPPayloadSize              uint64 // 1350
	InitialMaxData                 uint64 // 15MB
	InitialMaxStreamDataBidiLocal  uint64 // 6MB
	InitialMaxStreamDataBidiRemote uint64 // 6MB
	InitialMaxStreamDataUni        uint64 // 6MB
	InitialMaxStreamsBidi          uint64 // 100
	InitialMaxStreamsUni           uint64 // 100
	ActiveConnectionIDLimit        uint64 // 8
}

// DefaultChromeTransportParams returns transport params matching Chrome defaults.
func DefaultChromeTransportParams() *QUICTransportParams {
	return &QUICTransportParams{
		MaxIdleTimeout:                 30_000, // 30 seconds in ms
		MaxUDPPayloadSize:              1350,
		InitialMaxData:                 15_728_640, // 15 MB
		InitialMaxStreamDataBidiLocal:  6_291_456,  // 6 MB
		InitialMaxStreamDataBidiRemote: 6_291_456,
		InitialMaxStreamDataUni:        6_291_456,
		InitialMaxStreamsBidi:          100,
		InitialMaxStreamsUni:           100,
		ActiveConnectionIDLimit:        8,
	}
}

// FingerprintConfig controls what browser fingerprint to emulate.
type FingerprintConfig struct {
	Browser  string // "chrome", "firefox", "safari"
	Platform string // "windows", "macos", "linux"
}

// DefaultFingerprint returns Chrome on Windows config.
func DefaultFingerprint() *FingerprintConfig {
	return &FingerprintConfig{
		Browser:  "chrome",
		Platform: "windows",
	}
}
