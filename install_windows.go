//go:build windows

package main

import (
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// installSelf copies the downloaded .exe into the user's Programs folder
// (%LocalAppData%\Programs\USCF Portal Print Agent\) and starts that copy, so
// the login item does not point at the Downloads folder and a newer download
// replaces the old version. Returns true when the caller should exit.
func installSelf() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return false
	}
	dir := filepath.Join(base, "Programs", ProductName)
	target := filepath.Join(dir, ProductName+".exe")
	if strings.EqualFold(exe, target) {
		return false // already the installed copy
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("install: %v", err)
		return false
	}
	if err := copyFile(exe, target); err != nil {
		// A running copy keeps its .exe locked: ask it to quit, then try once more.
		askRunningAgentToQuit()
		if err := copyFile(exe, target); err != nil {
			log.Printf("install: %v (running from %s instead)", err, exe)
			return false
		}
	}
	cmd := exec.Command(target)
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		log.Printf("install: start %s: %v", target, err)
		return false
	}
	return true
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func askRunningAgentToQuit() {
	cfg, err := LoadConfig()
	if err != nil || cfg.Token == "" {
		return
	}
	req, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+itoa(cfg.Port)+"/v1/quit", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	client := &http.Client{Timeout: 3 * time.Second}
	if resp, err := client.Do(req); err == nil {
		resp.Body.Close()
		time.Sleep(1500 * time.Millisecond)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
