package h3

import (
	"crypto/tls"
	"testing"
)

func TestChromeQUICVersions(t *testing.T) {
	if len(ChromeQUICVersions) == 0 {
		t.Fatal("ChromeQUICVersions must not be empty")
	}
	// QUIC v1 (RFC 9000) must be present
	found := false
	for _, v := range ChromeQUICVersions {
		if v == 0x00000001 {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("ChromeQUICVersions must contain QUIC v1 (0x00000001)")
	}
}

func TestChromeALPNTokens(t *testing.T) {
	// ChromeALPN should contain exactly "h3"
	if len(ChromeALPN) != 1 {
		t.Fatalf("expected ChromeALPN length 1, got %d", len(ChromeALPN))
	}
	if ChromeALPN[0] != "h3" {
		t.Fatalf("expected ChromeALPN[0] = \"h3\", got %q", ChromeALPN[0])
	}
}

func TestChromeTLSALPNTokens(t *testing.T) {
	// ChromeTLSALPN should contain h3, h2, http/1.1 in that order
	expected := []string{"h3", "h2", "http/1.1"}
	if len(ChromeTLSALPN) != len(expected) {
		t.Fatalf("expected ChromeTLSALPN length %d, got %d", len(expected), len(ChromeTLSALPN))
	}
	for i, want := range expected {
		if ChromeTLSALPN[i] != want {
			t.Fatalf("ChromeTLSALPN[%d] = %q, want %q", i, ChromeTLSALPN[i], want)
		}
	}
}

func TestChromeCipherSuitesValidity(t *testing.T) {
	if len(ChromeCipherSuites) != 3 {
		t.Fatalf("expected 3 cipher suites, got %d", len(ChromeCipherSuites))
	}

	expectedSuites := []uint16{
		tls.TLS_AES_128_GCM_SHA256,
		tls.TLS_AES_256_GCM_SHA384,
		tls.TLS_CHACHA20_POLY1305_SHA256,
	}
	for i, want := range expectedSuites {
		if ChromeCipherSuites[i] != want {
			t.Fatalf("ChromeCipherSuites[%d] = %d, want %d", i, ChromeCipherSuites[i], want)
		}
	}
}

func TestChromeCurvePreferences(t *testing.T) {
	if len(ChromeCurvePreferences) != 3 {
		t.Fatalf("expected 3 curve preferences, got %d", len(ChromeCurvePreferences))
	}

	expectedCurves := []tls.CurveID{
		tls.X25519,
		tls.CurveP256,
		tls.CurveP384,
	}
	for i, want := range expectedCurves {
		if ChromeCurvePreferences[i] != want {
			t.Fatalf("ChromeCurvePreferences[%d] = %v, want %v", i, ChromeCurvePreferences[i], want)
		}
	}
}

func TestDefaultChromeTransportParamsValues(t *testing.T) {
	params := DefaultChromeTransportParams()
	if params == nil {
		t.Fatal("DefaultChromeTransportParams returned nil")
	}

	tests := []struct {
		name string
		got  uint64
		want uint64
	}{
		{"MaxIdleTimeout", params.MaxIdleTimeout, 30_000},
		{"MaxUDPPayloadSize", params.MaxUDPPayloadSize, 1350},
		{"InitialMaxData", params.InitialMaxData, 15_728_640},
		{"InitialMaxStreamDataBidiLocal", params.InitialMaxStreamDataBidiLocal, 6_291_456},
		{"InitialMaxStreamDataBidiRemote", params.InitialMaxStreamDataBidiRemote, 6_291_456},
		{"InitialMaxStreamDataUni", params.InitialMaxStreamDataUni, 6_291_456},
		{"InitialMaxStreamsBidi", params.InitialMaxStreamsBidi, 100},
		{"InitialMaxStreamsUni", params.InitialMaxStreamsUni, 100},
		{"ActiveConnectionIDLimit", params.ActiveConnectionIDLimit, 8},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}

func TestDefaultChromeTransportParamsReturnsNewInstance(t *testing.T) {
	p1 := DefaultChromeTransportParams()
	p2 := DefaultChromeTransportParams()
	if p1 == p2 {
		t.Fatal("DefaultChromeTransportParams should return a new instance each call")
	}
}

func TestDefaultFingerprintValues(t *testing.T) {
	fp := DefaultFingerprint()
	if fp == nil {
		t.Fatal("DefaultFingerprint returned nil")
	}
	if fp.Browser != "chrome" {
		t.Fatalf("Browser = %q, want \"chrome\"", fp.Browser)
	}
	if fp.Platform != "windows" {
		t.Fatalf("Platform = %q, want \"windows\"", fp.Platform)
	}
}

func TestDefaultFingerprintReturnsNewInstance(t *testing.T) {
	fp1 := DefaultFingerprint()
	fp2 := DefaultFingerprint()
	if fp1 == fp2 {
		t.Fatal("DefaultFingerprint should return a new instance each call")
	}
}

func TestFingerprintConfigFields(t *testing.T) {
	fc := &FingerprintConfig{
		Browser:  "firefox",
		Platform: "linux",
	}
	if fc.Browser != "firefox" {
		t.Fatalf("Browser = %q, want \"firefox\"", fc.Browser)
	}
	if fc.Platform != "linux" {
		t.Fatalf("Platform = %q, want \"linux\"", fc.Platform)
	}
}

func TestQUICTransportParamsZeroValue(t *testing.T) {
	var params QUICTransportParams
	// Zero value should have all fields as 0
	if params.MaxIdleTimeout != 0 {
		t.Fatalf("zero value MaxIdleTimeout should be 0, got %d", params.MaxIdleTimeout)
	}
	if params.InitialMaxStreamsBidi != 0 {
		t.Fatalf("zero value InitialMaxStreamsBidi should be 0, got %d", params.InitialMaxStreamsBidi)
	}
}

func TestFingerprintApplyToTLSConfig(t *testing.T) {
	// Verify that Chrome fingerprint values can be applied to a tls.Config
	tlsCfg := &tls.Config{
		CipherSuites:     ChromeCipherSuites,
		CurvePreferences: ChromeCurvePreferences,
		NextProtos:       ChromeTLSALPN,
		MinVersion:       tls.VersionTLS13,
	}

	if len(tlsCfg.CipherSuites) != 3 {
		t.Fatalf("expected 3 cipher suites in tls.Config, got %d", len(tlsCfg.CipherSuites))
	}
	if len(tlsCfg.CurvePreferences) != 3 {
		t.Fatalf("expected 3 curve preferences in tls.Config, got %d", len(tlsCfg.CurvePreferences))
	}
	if len(tlsCfg.NextProtos) != 3 {
		t.Fatalf("expected 3 ALPN protos in tls.Config, got %d", len(tlsCfg.NextProtos))
	}
	if tlsCfg.MinVersion != tls.VersionTLS13 {
		t.Fatal("expected TLS 1.3 minimum version")
	}
}
