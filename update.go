package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Self-update: the agent looks at the latest GitHub release of its own repo
// and, where the OS allows a silent install (Windows), installs it by itself.
// Every file is checked against the SHA256SUMS.txt published with the release.
// Deleting a bad release on GitHub makes the previous one "latest" again.

const (
	releaseAPI      = "https://api.github.com/repos/akshay-ahc/uscf-print-agent/releases/latest"
	checksumsAsset  = "SHA256SUMS.txt"
	updateFirstWait = 2 * time.Minute
	updateEvery     = 6 * time.Hour
)

type release struct {
	Tag    string            // "v0.4.1"
	Assets map[string]string // file name -> download URL
}

// semverParts reads "v1.2.3" or "1.2.3" (anything after a '-' or '+' is ignored).
func semverParts(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// isNewer reports whether latest is a higher version than current. A "dev"
// build or an unreadable version never updates.
func isNewer(latest, current string) bool {
	l, ok1 := semverParts(latest)
	c, ok2 := semverParts(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// parseChecksums reads "sha256  filename" lines (sha256sum output).
func parseChecksums(r io.Reader) map[string]string {
	sums := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) >= 2 && len(f[0]) == 64 {
			sums[strings.TrimPrefix(strings.Join(f[1:], " "), "*")] = strings.ToLower(f[0])
		}
	}
	return sums
}

// Downloads may only come from GitHub (release files redirect to its storage host).
func allowedDownloadHost(u *url.URL) bool {
	if u.Scheme != "https" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "github.com" || h == "api.github.com" || strings.HasSuffix(h, ".githubusercontent.com")
}

var updateHTTP = &http.Client{
	Timeout: 5 * time.Minute,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || !allowedDownloadHost(req.URL) {
			return errors.New("download redirected somewhere unexpected")
		}
		return nil
	},
}

func fetchLatestRelease(ctx context.Context) (release, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, releaseAPI, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "uscf-print-agent")
	res, err := updateHTTP.Do(req)
	if err != nil {
		return release{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("GitHub answered %d", res.StatusCode)
	}
	var body struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body); err != nil {
		return release{}, err
	}
	rel := release{Tag: body.TagName, Assets: map[string]string{}}
	for _, a := range body.Assets {
		rel.Assets[a.Name] = a.URL
	}
	return rel, nil
}

func download(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || !allowedDownloadHost(u) {
		return nil, errors.New("download URL is not on GitHub")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	req.Header.Set("User-Agent", "uscf-print-agent")
	res, err := updateHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download answered %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("download is larger than expected")
	}
	return data, nil
}
