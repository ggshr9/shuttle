package fakeip

import (
	"testing"
)

func TestFilter_ExactMatch(t *testing.T) {
	f := NewFilter([]string{
		"stun.l.google.com",
		"time.windows.com",
	})

	cases := []struct {
		domain string
		want   bool
	}{
		{"stun.l.google.com", true},
		{"STUN.L.GOOGLE.COM", true},   // case-insensitive
		{"time.windows.com", true},
		{"other.google.com", false},
		{"google.com", false},
		{"example.com", false},
	}

	for _, tc := range cases {
		got := f.ShouldSkip(tc.domain)
		if got != tc.want {
			t.Errorf("ShouldSkip(%q) = %v, want %v", tc.domain, got, tc.want)
		}
	}
}

func TestFilter_WildcardSuffix(t *testing.T) {
	f := NewFilter([]string{
		"+.lan",
		"+.local",
	})

	cases := []struct {
		domain string
		want   bool
	}{
		{"printer.lan", true},
		{"nas.home.lan", true},
		{"mydevice.local", true},
		{"gateway.local", true},
		{"LAN", false},           // no dot prefix, not a suffix match
		{"example.com", false},
		{"notlan", false},
		{"lan", false},           // plain "lan" does not end in ".lan"
	}

	for _, tc := range cases {
		got := f.ShouldSkip(tc.domain)
		if got != tc.want {
			t.Errorf("ShouldSkip(%q) = %v, want %v", tc.domain, got, tc.want)
		}
	}
}

func TestFilter_GlobPattern(t *testing.T) {
	f := NewFilter([]string{
		"stun.*",
		"time.*.com",
		"*.ntp.org",
	})

	cases := []struct {
		domain string
		want   bool
	}{
		// "stun.*" — * matches any chars including dots
		{"stun.example", true},
		{"stun.servers.net", true},  // * spans dots in filepath.Match
		// "time.*.com"
		{"time.apple.com", true},
		{"time.windows.com", true},
		// "*.ntp.org" — * matches any chars including dots
		{"pool.ntp.org", true},
		{"0.pool.ntp.org", true},    // * spans dots
		{"ntp.org", false},          // no wildcard segment before .ntp.org
		{"example.com", false},
	}

	for _, tc := range cases {
		got := f.ShouldSkip(tc.domain)
		if got != tc.want {
			t.Errorf("ShouldSkip(%q) = %v, want %v", tc.domain, got, tc.want)
		}
	}
}

func TestFilter_Empty(t *testing.T) {
	cases := []*Filter{
		NewFilter(nil),
		NewFilter([]string{}),
		NewFilter([]string{"", "  "}), // blank entries only
	}

	domains := []string{
		"example.com",
		"stun.l.google.com",
		"printer.lan",
		"",
	}

	for _, f := range cases {
		for _, d := range domains {
			if f.ShouldSkip(d) {
				t.Errorf("empty filter: ShouldSkip(%q) = true, want false", d)
			}
		}
	}
}

func TestFilter_CaseInsensitivePatterns(t *testing.T) {
	// Patterns themselves can be mixed-case; matching is always lowercased
	f := NewFilter([]string{
		"EXAMPLE.COM",
		"+.LAN",
		"Stun.*",
	})

	cases := []struct {
		domain string
		want   bool
	}{
		{"example.com", true},
		{"EXAMPLE.COM", true},
		{"Example.Com", true},
		{"printer.lan", true},
		{"PRINTER.LAN", true},
		{"stun.test", true},
		{"STUN.TEST", true},
		{"other.com", false},
	}

	for _, tc := range cases {
		got := f.ShouldSkip(tc.domain)
		if got != tc.want {
			t.Errorf("ShouldSkip(%q) = %v, want %v", tc.domain, got, tc.want)
		}
	}
}
