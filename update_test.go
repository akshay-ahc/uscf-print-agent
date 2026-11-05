package main

import (
	"net/url"
	"strings"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.4.0", "0.3.3", true},
		{"v0.3.10", "0.3.9", true},
		{"v1.0.0", "0.9.9", true},
		{"v0.4.0", "0.4.0", false},
		{"v0.3.3", "0.4.0", false}, // never downgrade
		{"v0.4.0", "dev", false},   // dev builds never update
		{"latest", "0.3.3", false},
		{"v0.4.1-beta", "0.4.0", true},
	}
	for _, c := range cases {
		if got := isNewer(c.latest, c.current); got != c.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestParseChecksums(t *testing.T) {
	a := strings.Repeat("a", 64)
	b := strings.Repeat("B", 64)
	sums := parseChecksums(strings.NewReader(a + "  USCF-Portal-Print-Agent.exe\n" + b + " *USCF-Portal-Print-Agent.pkg\nnot a line\n"))
	if sums["USCF-Portal-Print-Agent.exe"] != a || sums["USCF-Portal-Print-Agent.pkg"] != strings.ToLower(b) || len(sums) != 2 {
		t.Errorf("checksums: %v", sums)
	}
}

func TestAllowedDownloadHost(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://github.com/akshay-ahc/uscf-print-agent/releases/download/v0.4.0/x.exe": true,
		"https://objects.githubusercontent.com/github-production-release-asset/1":       true,
		"https://api.github.com/repos/x/y/releases/latest":                              true,
		"http://github.com/x":                       false,
		"https://evil.example.com/x.exe":            false,
		"https://github.com.evil.example.com/x.exe": false,
	} {
		u, _ := url.Parse(raw)
		if got := allowedDownloadHost(u); got != want {
			t.Errorf("%s: got %v want %v", raw, got, want)
		}
	}
}
