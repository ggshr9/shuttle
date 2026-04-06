package engine

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"testing"

	"github.com/shuttleX/shuttle/config"
)

func newTestEngine() *Engine {
	cfg := config.DefaultClientConfig()
	cfg.Server.Addr = "test.example.com:443"
	cfg.Server.Password = "super-secret-password"
	cfg.Transport.H3.Enabled = true
	return New(cfg)
}

func TestCollectDiagnostics(t *testing.T) {
	eng := newTestEngine()
	bundle := eng.CollectDiagnostics()

	if bundle == nil {
		t.Fatal("CollectDiagnostics returned nil")
	}

	// Check timestamp is set
	if bundle.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}

	// Check version is set
	if bundle.Version == "" {
		t.Error("expected non-empty version")
	}

	// Check system info
	if bundle.System.OS == "" {
		t.Error("expected non-empty OS")
	}
	if bundle.System.Arch == "" {
		t.Error("expected non-empty Arch")
	}
	if bundle.System.NumCPU == 0 {
		t.Error("expected non-zero NumCPU")
	}
	if bundle.System.GoVer == "" {
		t.Error("expected non-empty GoVer")
	}
	if bundle.System.NumGR == 0 {
		t.Error("expected non-zero goroutine count")
	}

	// Check status is populated
	if bundle.Status == nil {
		t.Error("expected non-nil Status")
	}

	// Check config is populated (engine not started, but cfg exists)
	if bundle.Config == nil {
		t.Error("expected non-nil Config")
	}

	// DNS.Servers may be nil if no DNS configured — just ensure no panic on access.
	_ = bundle.DNS.Servers

	// Check Router section
	if bundle.Router.Stats == nil {
		t.Error("expected non-nil Router.Stats map")
	}
}

func TestExportDiagnosticsZIP(t *testing.T) {
	eng := newTestEngine()

	var buf bytes.Buffer
	if err := eng.ExportDiagnosticsZIP(&buf); err != nil {
		t.Fatalf("ExportDiagnosticsZIP failed: %v", err)
	}

	// Open as ZIP
	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("failed to open ZIP: %v", err)
	}

	expectedFiles := map[string]bool{
		"diagnostics.json": false,
		"goroutines.txt":   false,
		"config.json":      false,
	}

	for _, f := range reader.File {
		if _, ok := expectedFiles[f.Name]; ok {
			expectedFiles[f.Name] = true

			rc, err := f.Open()
			if err != nil {
				t.Errorf("failed to open %s: %v", f.Name, err)
				continue
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Errorf("failed to read %s: %v", f.Name, err)
				continue
			}
			if len(data) == 0 {
				t.Errorf("%s is empty", f.Name)
			}

			// Verify diagnostics.json is valid JSON
			if f.Name == "diagnostics.json" {
				var bundle DiagnosticsBundle
				if err := json.Unmarshal(data, &bundle); err != nil {
					t.Errorf("diagnostics.json is not valid JSON: %v", err)
				}
			}
		}
	}

	for name, found := range expectedFiles {
		if !found {
			t.Errorf("expected file %s not found in ZIP", name)
		}
	}
}

func TestRedactConfig(t *testing.T) {
	cfg := config.DefaultClientConfig()
	cfg.Server.Password = "my-secret-pass"

	redacted := redactConfig(cfg)

	// Check that the server section exists
	server, ok := redacted["server"].(map[string]interface{})
	if !ok {
		t.Fatal("expected server section in redacted config")
	}

	// Password should be redacted
	if pw, ok := server["password"].(string); ok {
		if pw != "***" {
			t.Errorf("expected password to be '***', got %q", pw)
		}
	}

	// Walk entire map looking for any unredacted sensitive values
	checkNoSecrets(t, redacted, "")
}

// checkNoSecrets recursively checks that no sensitive keys have their original values.
func checkNoSecrets(t *testing.T, m map[string]interface{}, path string) {
	t.Helper()
	for k, v := range m {
		fullPath := path + "." + k
		if isSensitiveKey(k) {
			if s, ok := v.(string); ok && s != "" && s != "***" {
				t.Errorf("sensitive key %s has unredacted value: %q", fullPath, s)
			}
		}
		switch val := v.(type) {
		case map[string]interface{}:
			checkNoSecrets(t, val, fullPath)
		case []interface{}:
			for i, item := range val {
				if sub, ok := item.(map[string]interface{}); ok {
					checkNoSecrets(t, sub, fmt.Sprintf("%s[%d]", fullPath, i))
				}
			}
		}
	}
}

func TestSystemInfo(t *testing.T) {
	eng := newTestEngine()
	bundle := eng.CollectDiagnostics()

	sys := bundle.System
	if sys.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", sys.OS, runtime.GOOS)
	}
	if sys.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", sys.Arch, runtime.GOARCH)
	}
	if sys.NumCPU != runtime.NumCPU() {
		t.Errorf("NumCPU = %d, want %d", sys.NumCPU, runtime.NumCPU())
	}
	if sys.GoVer != runtime.Version() {
		t.Errorf("GoVer = %q, want %q", sys.GoVer, runtime.Version())
	}
	if sys.MemAlloc == 0 {
		t.Error("expected non-zero MemAlloc")
	}
	if sys.MemSys == 0 {
		t.Error("expected non-zero MemSys")
	}
}
