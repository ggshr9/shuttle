package geodata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewDownloader(t *testing.T) {
	dir := t.TempDir()
	d := NewDownloader(dir, nil)
	if d == nil {
		t.Fatal("NewDownloader returned nil")
	}
	if d.dataDir != dir {
		t.Fatalf("dataDir = %q, want %q", d.dataDir, dir)
	}
	if d.client == nil {
		t.Fatal("expected non-nil HTTP client")
	}
}

func TestNewDownloaderNilLogger(t *testing.T) {
	dir := t.TempDir()
	d := NewDownloader(dir, nil)
	if d.logger == nil {
		t.Fatal("expected default logger when nil is passed")
	}
}

func TestDownloadSuccess(t *testing.T) {
	body := "1.0.0.0/8\n2.0.0.0/8\n"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"abc123"`)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	}))
	defer ts.Close()

	dir := t.TempDir()
	d := NewDownloader(dir, nil)

	updated, err := d.Download(context.Background(), ts.URL+"/cn.txt", "cn-cidr.txt")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true for first download")
	}

	// Verify file was written
	data, err := os.ReadFile(filepath.Join(dir, "cn-cidr.txt"))
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(data) != body {
		t.Fatalf("file content = %q, want %q", string(data), body)
	}

	// Verify ETag was saved
	d.mu.Lock()
	etag := d.etag["cn-cidr.txt"]
	d.mu.Unlock()
	if etag != `"abc123"` {
		t.Fatalf("ETag = %q, want %q", etag, `"abc123"`)
	}
}

func TestDownloadETagCaching(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Header.Get("If-None-Match") == `"etag-v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"etag-v1"`)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	d := NewDownloader(dir, nil)

	// First download should fetch the data
	updated, err := d.Download(context.Background(), ts.URL+"/test.txt", "test.txt")
	if err != nil {
		t.Fatalf("first download: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true on first download")
	}

	// Second download should get 304 Not Modified
	updated, err = d.Download(context.Background(), ts.URL+"/test.txt", "test.txt")
	if err != nil {
		t.Fatalf("second download: %v", err)
	}
	if updated {
		t.Fatal("expected updated=false on 304 response")
	}

	if callCount != 2 {
		t.Fatalf("expected 2 HTTP requests, got %d", callCount)
	}
}

func TestDownloadIfNoneMatchHeaderSent(t *testing.T) {
	var receivedINM string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedINM = r.Header.Get("If-None-Match")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer ts.Close()

	dir := t.TempDir()
	d := NewDownloader(dir, nil)

	// Pre-set an ETag
	d.mu.Lock()
	d.etag["test.txt"] = `"pre-existing-etag"`
	d.mu.Unlock()

	d.Download(context.Background(), ts.URL+"/test.txt", "test.txt")

	if receivedINM != `"pre-existing-etag"` {
		t.Fatalf("If-None-Match header = %q, want %q", receivedINM, `"pre-existing-etag"`)
	}
}

func TestDownloadHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	dir := t.TempDir()
	d := NewDownloader(dir, nil)

	updated, err := d.Download(context.Background(), ts.URL+"/fail", "fail.txt")
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
	if updated {
		t.Fatal("expected updated=false on error")
	}
}

func TestDownloadHTTP404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	dir := t.TempDir()
	d := NewDownloader(dir, nil)

	updated, err := d.Download(context.Background(), ts.URL+"/notfound", "missing.txt")
	if err == nil {
		t.Fatal("expected error for HTTP 404")
	}
	if updated {
		t.Fatal("expected updated=false on 404")
	}
}

func TestDownloadNetworkFailure(t *testing.T) {
	dir := t.TempDir()
	d := NewDownloader(dir, nil)

	// Use an address that will fail to connect
	updated, err := d.Download(context.Background(), "http://127.0.0.1:1/fail", "fail.txt")
	if err == nil {
		t.Fatal("expected error for network failure")
	}
	if updated {
		t.Fatal("expected updated=false on network failure")
	}
}

func TestDownloadInvalidURL(t *testing.T) {
	dir := t.TempDir()
	d := NewDownloader(dir, nil)

	_, err := d.Download(context.Background(), "://invalid-url", "fail.txt")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestDownloadContextCancelled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	dir := t.TempDir()
	d := NewDownloader(dir, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := d.Download(ctx, ts.URL+"/slow", "slow.txt")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestDownloadAllSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data for " + r.URL.Path))
	}))
	defer ts.Close()

	dir := t.TempDir()
	d := NewDownloader(dir, nil)

	sources := map[string]string{
		"file1.txt": ts.URL + "/file1",
		"file2.txt": ts.URL + "/file2",
	}

	updated, err := d.DownloadAll(context.Background(), sources)
	if err != nil {
		t.Fatalf("DownloadAll: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true")
	}

	// Verify files exist
	for filename := range sources {
		if !d.FileExists(filename) {
			t.Fatalf("expected file %s to exist", filename)
		}
	}
}

func TestDownloadAllPartialFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	d := NewDownloader(dir, nil)

	sources := map[string]string{
		"good.txt": ts.URL + "/ok",
		"bad.txt":  ts.URL + "/fail",
	}

	// Partial failure should not return an error (only all-fail does)
	updated, err := d.DownloadAll(context.Background(), sources)
	if err != nil {
		t.Fatalf("DownloadAll partial failure should not error: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true when at least one file succeeded")
	}
}

func TestDownloadAllTotalFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	dir := t.TempDir()
	d := NewDownloader(dir, nil)

	sources := map[string]string{
		"fail1.txt": ts.URL + "/fail1",
		"fail2.txt": ts.URL + "/fail2",
	}

	_, err := d.DownloadAll(context.Background(), sources)
	if err == nil {
		t.Fatal("expected error when all downloads fail")
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	d := NewDownloader(dir, nil)

	if d.FileExists("nonexistent.txt") {
		t.Fatal("expected FileExists=false for nonexistent file")
	}

	if err := os.WriteFile(filepath.Join(dir, "exists.txt"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !d.FileExists("exists.txt") {
		t.Fatal("expected FileExists=true for existing file")
	}
}

func TestFilePath(t *testing.T) {
	dir := t.TempDir()
	d := NewDownloader(dir, nil)

	expected := filepath.Join(dir, "test.txt")
	got := d.FilePath("test.txt")
	if got != expected {
		t.Fatalf("FilePath = %q, want %q", got, expected)
	}
}

func TestETagPersistence(t *testing.T) {
	dir := t.TempDir()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"persist-etag"`)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data"))
	}))
	defer ts.Close()

	// Download a file to save an ETag
	d1 := NewDownloader(dir, nil)
	d1.Download(context.Background(), ts.URL+"/test", "test.txt")

	// Create a new downloader from the same directory -- should load persisted ETags
	d2 := NewDownloader(dir, nil)
	d2.mu.Lock()
	etag := d2.etag["test.txt"]
	d2.mu.Unlock()

	if etag != `"persist-etag"` {
		t.Fatalf("persisted ETag = %q, want %q", etag, `"persist-etag"`)
	}
}

func TestDownloadNoETagHeader(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No ETag header
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("no etag"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	d := NewDownloader(dir, nil)

	updated, err := d.Download(context.Background(), ts.URL+"/test", "test.txt")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true")
	}

	// ETag should not be set
	d.mu.Lock()
	etag := d.etag["test.txt"]
	d.mu.Unlock()
	if etag != "" {
		t.Fatalf("expected empty ETag, got %q", etag)
	}
}

func TestDownloadCreatesDataDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "dir")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data"))
	}))
	defer ts.Close()

	d := NewDownloader(dir, nil)
	updated, err := d.Download(context.Background(), ts.URL+"/test", "test.txt")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true")
	}

	// Verify the nested directory was created
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("expected data directory to be created")
	}
}

func TestDownloadAtomicWrite(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("new content"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "test.txt")

	// Pre-create the file with old content
	if err := os.WriteFile(destPath, []byte("old content"), 0o600); err != nil {
		t.Fatal(err)
	}

	d := NewDownloader(dir, nil)
	updated, err := d.Download(context.Background(), ts.URL+"/test", "test.txt")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if !updated {
		t.Fatal("expected updated=true")
	}

	// Verify content was replaced
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new content" {
		t.Fatalf("file content = %q, want %q", string(data), "new content")
	}

	// Verify no temp file remains
	tmpPath := destPath + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatal("temp file should not remain after successful download")
	}
}
