package update

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"2.0.0", "1.9.9", 1},
		{"1.10.0", "1.9.0", 1},
		{"0.0.1", "0.0.0", 1},
		{"v1.0.0", "1.0.0", 0},
		{"v1.0.1", "v1.0.0", 1},
		{"1.0.0-beta", "1.0.0", 0}, // Pre-release treated same as release
		{"1.2.3", "1.2", 1},        // Missing patch = 0
		{"1.2", "1.2.0", 0},
	}

	for _, tt := range tests {
		got := compareVersions(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		v    string
		want [3]int
	}{
		{"1.2.3", [3]int{1, 2, 3}},
		{"v1.2.3", [3]int{1, 2, 3}},
		{"0.0.1", [3]int{0, 0, 1}},
		{"10.20.30", [3]int{10, 20, 30}},
		{"1.2.3-beta", [3]int{1, 2, 3}},
		{"1.2.3-rc1", [3]int{1, 2, 3}},
		{"1.2", [3]int{1, 2, 0}},
		{"1", [3]int{1, 0, 0}},
		{"", [3]int{0, 0, 0}},
		{"invalid", [3]int{0, 0, 0}},
	}

	for _, tt := range tests {
		got := parseVersion(tt.v)
		if got != tt.want {
			t.Errorf("parseVersion(%q) = %v, want %v", tt.v, got, tt.want)
		}
	}
}

func TestCheckerGetAssetName(t *testing.T) {
	c := NewChecker()
	name := c.getAssetName()

	// Should contain OS and arch
	if name == "" {
		t.Error("getAssetName() returned empty string")
	}
	// Format should be os_arch
	if len(name) < 3 {
		t.Errorf("getAssetName() = %q, expected longer string", name)
	}
}

func TestBuildUpdateInfo(t *testing.T) {
	c := NewChecker()

	// Test with newer version available
	release := Release{
		TagName: "v2.0.0",
		Name:    "Version 2.0.0",
		Body:    "Release notes here",
		HTMLURL: "https://github.com/test/test/releases/v2.0.0",
		Assets: []Asset{
			{Name: "shuttle_darwin_aarch64.tar.gz", BrowserDownloadURL: "https://example.com/download"},
		},
	}

	// Temporarily set version for testing
	oldVersion := Version
	Version = "1.0.0"
	defer func() { Version = oldVersion }()

	info := c.buildUpdateInfo(release)

	if !info.Available {
		t.Error("Expected update to be available")
	}
	if info.LatestVersion != "v2.0.0" {
		t.Errorf("LatestVersion = %q, want %q", info.LatestVersion, "v2.0.0")
	}
	if info.CurrentVersion != "1.0.0" {
		t.Errorf("CurrentVersion = %q, want %q", info.CurrentVersion, "1.0.0")
	}
}

func TestBuildUpdateInfoDevVersion(t *testing.T) {
	c := NewChecker()

	release := Release{TagName: "v2.0.0"}

	// Dev version should never show updates
	oldVersion := Version
	Version = "dev"
	defer func() { Version = oldVersion }()

	info := c.buildUpdateInfo(release)

	if info.Available {
		t.Error("Dev version should not show updates available")
	}
}
