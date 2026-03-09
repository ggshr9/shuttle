package geodata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const maxBodySize = 50 << 20 // 50 MB

// Downloader fetches geo data files from remote URLs with ETag caching.
type Downloader struct {
	client  *http.Client
	dataDir string
	logger  *slog.Logger

	mu   sync.Mutex
	etag map[string]string // filename -> ETag
}

// NewDownloader creates a new geo data downloader.
func NewDownloader(dataDir string, logger *slog.Logger) *Downloader {
	if logger == nil {
		logger = slog.Default()
	}
	d := &Downloader{
		client: &http.Client{
			Timeout: 2 * time.Minute,
		},
		dataDir: dataDir,
		logger:  logger,
		etag:    make(map[string]string),
	}
	d.loadETags()
	return d
}

// Download fetches a single URL to dataDir/filename. Returns true if the file was updated.
func (d *Downloader) Download(ctx context.Context, url, filename string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("create request: %w", err)
	}

	d.mu.Lock()
	if etag := d.etag[filename]; etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	d.mu.Unlock()

	resp, err := d.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("fetch %s: %w", filename, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		d.logger.Debug("geodata not modified", "file", filename)
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("fetch %s: HTTP %d", filename, resp.StatusCode)
	}

	// Ensure data dir exists
	if err := os.MkdirAll(d.dataDir, 0o755); err != nil {
		return false, fmt.Errorf("create data dir: %w", err)
	}

	// Write to temp file, then atomic rename
	tmpFile := filepath.Join(d.dataDir, filename+".tmp")
	f, err := os.Create(tmpFile)
	if err != nil {
		return false, fmt.Errorf("create temp file: %w", err)
	}

	n, err := io.Copy(f, io.LimitReader(resp.Body, maxBodySize))
	f.Close()
	if err != nil {
		os.Remove(tmpFile)
		return false, fmt.Errorf("write %s: %w", filename, err)
	}

	destPath := filepath.Join(d.dataDir, filename)
	if err := os.Rename(tmpFile, destPath); err != nil {
		os.Remove(tmpFile)
		return false, fmt.Errorf("rename %s: %w", filename, err)
	}

	d.logger.Info("geodata updated", "file", filename, "bytes", n)

	// Save ETag
	if etag := resp.Header.Get("ETag"); etag != "" {
		d.mu.Lock()
		d.etag[filename] = etag
		d.mu.Unlock()
		d.saveETags()
	}

	return true, nil
}

// DownloadAll fetches multiple files concurrently. Returns true if any file was updated.
func (d *Downloader) DownloadAll(ctx context.Context, sources map[string]string) (bool, error) {
	type result struct {
		file    string
		updated bool
		err     error
	}

	ch := make(chan result, len(sources))
	sem := make(chan struct{}, 4) // max 4 concurrent downloads

	for filename, url := range sources {
		go func(fn, u string) {
			sem <- struct{}{}
			defer func() { <-sem }()
			updated, err := d.Download(ctx, u, fn)
			ch <- result{file: fn, updated: updated, err: err}
		}(filename, url)
	}

	var anyUpdated bool
	var errs []error
	for range sources {
		r := <-ch
		if r.err != nil {
			d.logger.Warn("geodata download failed", "file", r.file, "err", r.err)
			errs = append(errs, r.err)
		} else if r.updated {
			anyUpdated = true
		}
	}

	if len(errs) == len(sources) {
		return false, fmt.Errorf("all downloads failed: %v", errs[0])
	}
	return anyUpdated, nil
}

// FileExists checks if a cached file exists.
func (d *Downloader) FileExists(filename string) bool {
	_, err := os.Stat(filepath.Join(d.dataDir, filename))
	return err == nil
}

// FilePath returns the full path to a cached file.
func (d *Downloader) FilePath(filename string) string {
	return filepath.Join(d.dataDir, filename)
}

func (d *Downloader) etagPath() string {
	return filepath.Join(d.dataDir, "etags.json")
}

func (d *Downloader) loadETags() {
	data, err := os.ReadFile(d.etagPath())
	if err != nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_ = json.Unmarshal(data, &d.etag)
}

func (d *Downloader) saveETags() {
	d.mu.Lock()
	data, _ := json.Marshal(d.etag)
	d.mu.Unlock()
	_ = os.MkdirAll(d.dataDir, 0o755)
	_ = os.WriteFile(d.etagPath(), data, 0o644)
}
