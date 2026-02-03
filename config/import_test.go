package config

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestImportConfigShuttleURI(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		wantErr bool
		check   func(t *testing.T, result *ImportResult)
	}{
		{
			name: "basic URI",
			uri:  "shuttle://password@example.com:443",
			check: func(t *testing.T, result *ImportResult) {
				if len(result.Servers) != 1 {
					t.Fatalf("Expected 1 server, got %d", len(result.Servers))
				}
				srv := result.Servers[0]
				if srv.Addr != "example.com:443" {
					t.Errorf("Addr = %q, want %q", srv.Addr, "example.com:443")
				}
				if srv.Password != "password" {
					t.Errorf("Password = %q, want %q", srv.Password, "password")
				}
			},
		},
		{
			name: "URI with name and SNI",
			uri:  "shuttle://pass@server.com:443?name=My%20Server&sni=www.google.com",
			check: func(t *testing.T, result *ImportResult) {
				srv := result.Servers[0]
				if srv.Name != "My Server" {
					t.Errorf("Name = %q, want %q", srv.Name, "My Server")
				}
				if srv.SNI != "www.google.com" {
					t.Errorf("SNI = %q, want %q", srv.SNI, "www.google.com")
				}
			},
		},
		{
			name: "multi-line URIs",
			uri: `shuttle://pass1@server1.com:443?name=Server1
shuttle://pass2@server2.com:443?name=Server2
# comment line
shuttle://pass3@server3.com:443?name=Server3`,
			check: func(t *testing.T, result *ImportResult) {
				if len(result.Servers) != 3 {
					t.Fatalf("Expected 3 servers, got %d", len(result.Servers))
				}
			},
		},
		{
			name:    "empty URI",
			uri:     "",
			wantErr: true,
		},
		{
			name:    "invalid scheme",
			uri:     "http://example.com:443",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ImportConfig(tt.uri)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ImportConfig() error = %v", err)
			}
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

func TestImportConfigJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantLen int
		wantErr bool
	}{
		{
			name:    "single server object",
			json:    `{"addr":"server.com:443","password":"pass","name":"Test"}`,
			wantLen: 1,
		},
		{
			name:    "array of servers",
			json:    `[{"addr":"s1.com:443"},{"addr":"s2.com:443"}]`,
			wantLen: 2,
		},
		{
			name:    "wrapped servers object",
			json:    `{"servers":[{"addr":"s1.com:443"},{"addr":"s2.com:443"}]}`,
			wantLen: 2,
		},
		{
			name:    "filter empty addresses",
			json:    `[{"addr":"s1.com:443"},{"name":"no addr"}]`,
			wantLen: 1,
		},
		{
			name:    "invalid JSON",
			json:    `{invalid}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ImportConfig(tt.json)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ImportConfig() error = %v", err)
			}
			if len(result.Servers) != tt.wantLen {
				t.Errorf("len(Servers) = %d, want %d", len(result.Servers), tt.wantLen)
			}
		})
	}
}

func TestImportConfigBase64(t *testing.T) {
	// Create JSON servers
	servers := []ServerEndpoint{
		{Addr: "server1.com:443", Name: "Server 1"},
		{Addr: "server2.com:443", Name: "Server 2"},
	}
	jsonData, _ := json.Marshal(servers)

	tests := []struct {
		name    string
		encode  func([]byte) string
		wantLen int
	}{
		{
			name:    "standard base64",
			encode:  func(b []byte) string { return base64.StdEncoding.EncodeToString(b) },
			wantLen: 2,
		},
		{
			name:    "URL-safe base64",
			encode:  func(b []byte) string { return base64.URLEncoding.EncodeToString(b) },
			wantLen: 2,
		},
		{
			name:    "raw base64 (no padding)",
			encode:  func(b []byte) string { return base64.RawStdEncoding.EncodeToString(b) },
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := tt.encode(jsonData)
			result, err := ImportConfig(encoded)
			if err != nil {
				t.Fatalf("ImportConfig() error = %v", err)
			}
			if len(result.Servers) != tt.wantLen {
				t.Errorf("len(Servers) = %d, want %d", len(result.Servers), tt.wantLen)
			}
		})
	}
}

func TestParseShuttleURI(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		want    *ServerEndpoint
		wantErr bool
	}{
		{
			name: "full URI",
			uri:  "shuttle://mypassword@example.com:443?name=Test&sni=www.google.com",
			want: &ServerEndpoint{
				Addr:     "example.com:443",
				Password: "mypassword",
				Name:     "Test",
				SNI:      "www.google.com",
			},
		},
		{
			name: "no password",
			uri:  "shuttle://example.com:443?name=NoPass",
			want: &ServerEndpoint{
				Addr: "example.com:443",
				Name: "NoPass",
			},
		},
		{
			name: "default name to addr",
			uri:  "shuttle://pass@example.com:443",
			want: &ServerEndpoint{
				Addr:     "example.com:443",
				Password: "pass",
				Name:     "example.com:443",
			},
		},
		{
			name:    "missing host",
			uri:     "shuttle://?name=Test",
			wantErr: true,
		},
		{
			name:    "wrong scheme",
			uri:     "http://example.com:443",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseShuttleURI(tt.uri)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseShuttleURI() error = %v", err)
			}
			if got.Addr != tt.want.Addr {
				t.Errorf("Addr = %q, want %q", got.Addr, tt.want.Addr)
			}
			if got.Password != tt.want.Password {
				t.Errorf("Password = %q, want %q", got.Password, tt.want.Password)
			}
			if got.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.want.Name)
			}
			if got.SNI != tt.want.SNI {
				t.Errorf("SNI = %q, want %q", got.SNI, tt.want.SNI)
			}
		})
	}
}

func TestExportConfig(t *testing.T) {
	cfg := &ClientConfig{
		Server: ServerEndpoint{
			Addr:     "active.com:443",
			Password: "pass",
			Name:     "Active",
		},
		Servers: []ServerEndpoint{
			{Addr: "saved1.com:443", Name: "Saved 1"},
			{Addr: "saved2.com:443", Name: "Saved 2", SNI: "google.com"},
		},
	}

	t.Run("JSON export", func(t *testing.T) {
		data, err := ExportConfig(cfg, "json")
		if err != nil {
			t.Fatalf("ExportConfig() error = %v", err)
		}
		if !strings.Contains(string(data), "active.com:443") {
			t.Error("JSON should contain server address")
		}
	})

	t.Run("YAML export", func(t *testing.T) {
		data, err := ExportConfig(cfg, "yaml")
		if err != nil {
			t.Fatalf("ExportConfig() error = %v", err)
		}
		if !strings.Contains(string(data), "active.com:443") {
			t.Error("YAML should contain server address")
		}
	})

	t.Run("URI export", func(t *testing.T) {
		data, err := ExportConfig(cfg, "uri")
		if err != nil {
			t.Fatalf("ExportConfig() error = %v", err)
		}
		lines := strings.Split(string(data), "\n")
		if len(lines) != 3 {
			t.Errorf("URI export should have 3 lines, got %d", len(lines))
		}
		if !strings.HasPrefix(lines[0], "shuttle://") {
			t.Error("URI should start with shuttle://")
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		_, err := ExportConfig(cfg, "invalid")
		if err == nil {
			t.Error("Expected error for invalid format")
		}
	})
}

func TestTryBase64Decode(t *testing.T) {
	original := "hello world"

	tests := []struct {
		name   string
		encode func(string) string
	}{
		{"standard", func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }},
		{"URL-safe", func(s string) string { return base64.URLEncoding.EncodeToString([]byte(s)) }},
		{"raw standard", func(s string) string { return base64.RawStdEncoding.EncodeToString([]byte(s)) }},
		{"raw URL-safe", func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := tt.encode(original)
			decoded, err := tryBase64Decode(encoded)
			if err != nil {
				t.Fatalf("tryBase64Decode() error = %v", err)
			}
			if decoded != original {
				t.Errorf("decoded = %q, want %q", decoded, original)
			}
		})
	}

	t.Run("not base64", func(t *testing.T) {
		_, err := tryBase64Decode("not valid base64!!!")
		if err == nil {
			t.Error("Expected error for non-base64 input")
		}
	})
}

func TestFilterValidServers(t *testing.T) {
	servers := []ServerEndpoint{
		{Addr: "valid1.com:443"},
		{Name: "no addr"},
		{Addr: "valid2.com:443"},
		{Addr: ""},
	}

	filtered := filterValidServers(servers)
	if len(filtered) != 2 {
		t.Errorf("filterValidServers() len = %d, want 2", len(filtered))
	}
}
