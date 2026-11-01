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
	"unsafe"

	"golang.org/x/sys/windows"
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
		// A running copy keeps its .exe locked: ask it to quit (needs the
		// pairing token), stop it if it is still there (it is not paired yet,
		// or did not answer), then try once more.
		askRunningAgentToQuit()
		stopRunningCopies(target)
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
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// stopRunningCopies ends every process running the installed exe (this user's
// own agent), and waits for each to exit so its .exe and port are free.
func stopRunningCopies(target string) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err := windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if pe.ProcessID == uint32(os.Getpid()) || !strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), filepath.Base(target)) {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pe.ProcessID)
		if err != nil {
			continue
		}
		buf := make([]uint16, windows.MAX_LONG_PATH)
		n := uint32(len(buf))
		if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) == nil && strings.EqualFold(windows.UTF16ToString(buf[:n]), target) {
			if err := windows.TerminateProcess(h, 0); err != nil {
				log.Printf("install: stop running copy %d: %v", pe.ProcessID, err)
			} else {
				_, _ = windows.WaitForSingleObject(h, 3000)
			}
		}
		windows.CloseHandle(h)
	}
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
