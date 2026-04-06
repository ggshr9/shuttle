// Package update provides auto-update functionality for Shuttle.
package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Version is the current application version.
// This should be set at build time using -ldflags.
var Version = "dev"

const (
	// GitHubRepo is the repository for releases
	GitHubRepo = "shuttleX/shuttle"
	// CheckInterval is the minimum time between update checks
	CheckInterval = 4 * time.Hour
)

// Release represents a GitHub release.
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
	Assets      []Asset   `json:"assets"`
}

// Asset represents a release asset (downloadable file).
type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// UpdateInfo contains information about an available update.
type UpdateInfo struct {
	Available       bool      `json:"available"`
	CurrentVersion  string    `json:"current_version"`
	LatestVersion   string    `json:"latest_version"`
	ReleaseDate     time.Time `json:"release_date,omitempty"`
	ReleaseURL      string    `json:"release_url,omitempty"`
	DownloadURL     string    `json:"download_url,omitempty"`
	Changelog       string    `json:"changelog,omitempty"`
	AssetName       string    `json:"asset_name,omitempty"`
	AssetSize       int64     `json:"asset_size,omitempty"`
}

// Checker handles update checking.
type Checker struct {
	client    *http.Client
	lastCheck time.Time
	cached    *UpdateInfo
}

// NewChecker creates a new update checker.
func NewChecker() *Checker {
	return &Checker{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// Check checks for updates.
func (c *Checker) Check(force bool) (*UpdateInfo, error) {
	// Return cached result if recent
	if !force && c.cached != nil && time.Since(c.lastCheck) < CheckInterval {
		return c.cached, nil
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", GitHubRepo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "shuttleX/"+Version)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("check update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// No releases yet
		info := &UpdateInfo{
			Available:      false,
			CurrentVersion: Version,
		}
		c.cached = info
		c.lastCheck = time.Now()
		return info, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("check update: unexpected status %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("parse release: %w", err)
	}

	info := c.buildUpdateInfo(&release)
	c.cached = info
	c.lastCheck = time.Now()
	return info, nil
}

func (c *Checker) buildUpdateInfo(release *Release) *UpdateInfo {
	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(Version, "v")

	info := &UpdateInfo{
		CurrentVersion: Version,
		LatestVersion:  release.TagName,
		ReleaseDate:    release.PublishedAt,
		ReleaseURL:     release.HTMLURL,
		Changelog:      release.Body,
	}

	// Check if update is available using proper version comparison
	info.Available = currentVersion != "dev" && compareVersions(latestVersion, currentVersion) > 0

	// Find appropriate asset for current platform
	assetName := c.getAssetName()
	for _, asset := range release.Assets {
		if strings.Contains(strings.ToLower(asset.Name), assetName) {
			info.DownloadURL = asset.BrowserDownloadURL
			info.AssetName = asset.Name
			info.AssetSize = asset.Size
			break
		}
	}

	return info
}

func (c *Checker) getAssetName() string {
	os := runtime.GOOS
	arch := runtime.GOARCH

	// Map common architectures
	if arch == "amd64" {
		arch = "x86_64"
	} else if arch == "arm64" {
		arch = "aarch64"
	}

	return fmt.Sprintf("%s_%s", os, arch)
}

// GetCurrentVersion returns the current version.
func GetCurrentVersion() string {
	return Version
}

// compareVersions compares two semantic versions.
// Returns 1 if a > b, -1 if a < b, 0 if equal.
func compareVersions(a, b string) int {
	aParts := parseVersion(a)
	bParts := parseVersion(b)

	for i := 0; i < 3; i++ {
		if aParts[i] > bParts[i] {
			return 1
		}
		if aParts[i] < bParts[i] {
			return -1
		}
	}
	return 0
}

// parseVersion extracts major, minor, patch from version string.
func parseVersion(v string) [3]int {
	var parts [3]int
	v = strings.TrimPrefix(v, "v")

	// Split by dot and parse
	segments := strings.Split(v, ".")
	for i := 0; i < len(segments) && i < 3; i++ {
		// Remove any suffix (e.g., "1.0.0-beta" -> "1.0.0")
		num := strings.Split(segments[i], "-")[0]
		if n, err := strconv.Atoi(num); err == nil {
			parts[i] = n
		}
	}
	return parts
}
