package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverHandlerDefault(t *testing.T) {
	handler := NewCoverHandler(&CoverConfig{Mode: ""}, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "<html>") {
		t.Fatal("default handler should return an HTML page")
	}
	if !strings.Contains(body, "Welcome") {
		t.Fatal("default handler should contain welcome text")
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", ct)
	}

	srv := rec.Header().Get("Server")
	if srv == "" {
		t.Fatal("default handler should set a Server header")
	}
}

func TestCoverHandlerStatic(t *testing.T) {
	dir := t.TempDir()
	indexContent := "<html><body>static test page</body></html>"
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(indexContent), 0o600); err != nil {
		t.Fatal(err)
	}

	handler := NewCoverHandler(&CoverConfig{
		Mode:      "static",
		StaticDir: dir,
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "static test page") {
		t.Fatalf("expected static content, got %q", body)
	}
}
