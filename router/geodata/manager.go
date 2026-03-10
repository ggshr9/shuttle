package geodata

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ManagerConfig mirrors config.GeoDataConfig to avoid circular imports.
type ManagerConfig struct {
	Enabled        bool
	DataDir        string
	AutoUpdate     bool
	UpdateInterval string
	DirectListURL  string
	ProxyListURL   string
	RejectListURL  string
	GFWListURL     string
	CNCidrURL      string
	PrivateCidrURL string
}

// Standard file names for cached geo data.
const (
	FileDirectList  = "direct-list.txt"
	FileProxyList   = "proxy-list.txt"
	FileRejectList  = "reject-list.txt"
	FileGFWList     = "gfw.txt"
	FileCNCidr      = "cn-cidr.txt"
	FilePrivateCidr = "private-cidr.txt"
)

// Category names used in routing rules.
const (
	CategoryCN     = "cn"
	CategoryProxy  = "geolocation-!cn"
	CategoryReject = "category-ads-all"
	CategoryGFW    = "gfw"
)

// Status reports the current state of geo data management.
type Status struct {
	Enabled      bool      `json:"enabled"`
	LastUpdate   time.Time `json:"last_update"`
	LastError    string    `json:"last_error,omitempty"`
	Updating     bool      `json:"updating"`
	FilesPresent []string  `json:"files_present"`
	NextUpdate   time.Time `json:"next_update,omitempty"`
}

// Manager orchestrates downloading, caching, and auto-updating geo data.
type Manager struct {
	mu         sync.RWMutex
	cfg        ManagerConfig
	downloader *Downloader
	logger     *slog.Logger

	lastUpdate time.Time
	lastError  string
	updating   bool
	cancel     context.CancelFunc
}

// persistedStatus is the on-disk representation of manager state.
type persistedStatus struct {
	LastUpdate time.Time `json:"last_update"`
}

// NewManager creates a new geo data manager.
func NewManager(cfg ManagerConfig, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	m := &Manager{
		cfg:        cfg,
		downloader: NewDownloader(cfg.DataDir, logger),
		logger:     logger,
	}
	m.loadStatus()
	return m
}

// Start begins the auto-update loop if configured.
func (m *Manager) Start(ctx context.Context) {
	if !m.cfg.AutoUpdate {
		return
	}

	ctx, m.cancel = context.WithCancel(ctx)

	interval, err := time.ParseDuration(m.cfg.UpdateInterval)
	if err != nil || interval < time.Hour {
		interval = 24 * time.Hour
	}

	go func() {
		// Download on first start if files are missing
		if !m.hasAllFiles() {
			_ = m.Update(ctx)
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = m.Update(ctx)
			}
		}
	}()
}

// Stop cancels the auto-update loop.
func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// Update downloads the latest geo data files.
func (m *Manager) Update(ctx context.Context) error {
	m.mu.Lock()
	if m.updating {
		m.mu.Unlock()
		return fmt.Errorf("update already in progress")
	}
	m.updating = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.updating = false
		m.mu.Unlock()
	}()

	sources := m.buildSources()
	m.logger.Info("updating geodata", "files", len(sources))

	_, err := m.downloader.DownloadAll(ctx, sources)
	m.mu.Lock()
	if err != nil {
		m.lastError = err.Error()
	} else {
		m.lastUpdate = time.Now()
		m.lastError = ""
	}
	m.mu.Unlock()

	if err == nil {
		m.saveStatus()
	}

	return err
}

// LoadGeoIPEntries reads cached CIDR files into GeoIPEntry slices.
func (m *Manager) LoadGeoIPEntries() ([]GeoIPEntry, error) {
	var entries []GeoIPEntry

	if cidrs, err := loadBareCIDRs(m.downloader.FilePath(FileCNCidr)); err == nil && len(cidrs) > 0 {
		entries = append(entries, GeoIPEntry{CountryCode: "CN", CIDRs: cidrs})
	}

	if cidrs, err := loadBareCIDRs(m.downloader.FilePath(FilePrivateCidr)); err == nil && len(cidrs) > 0 {
		entries = append(entries, GeoIPEntry{CountryCode: "PRIVATE", CIDRs: cidrs})
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no geodata IP files found")
	}
	return entries, nil
}

// LoadGeoSiteEntries reads cached domain list files into GeoSiteEntry slices.
func (m *Manager) LoadGeoSiteEntries() ([]GeoSiteEntry, error) {
	fileMap := map[string]string{
		FileDirectList: CategoryCN,
		FileProxyList:  CategoryProxy,
		FileRejectList: CategoryReject,
		FileGFWList:    CategoryGFW,
	}

	var entries []GeoSiteEntry
	for filename, category := range fileMap {
		path := m.downloader.FilePath(filename)
		entry, err := LoadGeoSiteFromFile(path, category)
		if err != nil {
			continue
		}
		if len(entry.Domains) > 0 {
			entries = append(entries, *entry)
		}
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no geodata site files found")
	}
	return entries, nil
}

// Status returns the current manager state.
func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	interval, _ := time.ParseDuration(m.cfg.UpdateInterval)
	if interval < time.Hour {
		interval = 24 * time.Hour
	}

	s := Status{
		Enabled:    m.cfg.Enabled,
		LastUpdate: m.lastUpdate,
		LastError:  m.lastError,
		Updating:   m.updating,
	}

	for _, f := range []string{FileDirectList, FileProxyList, FileRejectList, FileGFWList, FileCNCidr, FilePrivateCidr} {
		if m.downloader.FileExists(f) {
			s.FilesPresent = append(s.FilesPresent, f)
		}
	}

	if m.cfg.AutoUpdate && !m.lastUpdate.IsZero() {
		s.NextUpdate = m.lastUpdate.Add(interval)
	}

	return s
}

func (m *Manager) buildSources() map[string]string {
	sources := map[string]string{
		FileDirectList: m.cfg.DirectListURL,
		FileProxyList:  m.cfg.ProxyListURL,
		FileRejectList: m.cfg.RejectListURL,
		FileGFWList:    m.cfg.GFWListURL,
	}
	if m.cfg.CNCidrURL != "" {
		sources[FileCNCidr] = m.cfg.CNCidrURL
	}
	if m.cfg.PrivateCidrURL != "" {
		sources[FilePrivateCidr] = m.cfg.PrivateCidrURL
	}
	return sources
}

func (m *Manager) hasAllFiles() bool {
	for _, f := range []string{FileDirectList, FileCNCidr} {
		if !m.downloader.FileExists(f) {
			return false
		}
	}
	return true
}

func (m *Manager) statusPath() string {
	return filepath.Join(m.cfg.DataDir, "status.json")
}

func (m *Manager) loadStatus() {
	data, err := os.ReadFile(m.statusPath())
	if err != nil {
		return
	}
	var ps persistedStatus
	if err := json.Unmarshal(data, &ps); err != nil {
		return
	}
	m.mu.Lock()
	m.lastUpdate = ps.LastUpdate
	m.mu.Unlock()
}

func (m *Manager) saveStatus() {
	m.mu.RLock()
	ps := persistedStatus{LastUpdate: m.lastUpdate}
	m.mu.RUnlock()
	data, err := json.Marshal(ps)
	if err != nil {
		return
	}
	_ = os.MkdirAll(m.cfg.DataDir, 0o755)
	_ = os.WriteFile(m.statusPath(), data, 0o644)
}

// loadBareCIDRs reads a file with one CIDR per line (no country code prefix).
func loadBareCIDRs(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cidrs []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cidrs = append(cidrs, line)
	}
	return cidrs, scanner.Err()
}
