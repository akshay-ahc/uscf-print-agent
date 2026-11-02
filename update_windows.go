//go:build windows

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// Windows installs silently: the new .exe copies itself into Programs, stops
// this copy (see installSelf) and starts, keeping the pairing and settings.
const autoUpdateSupported = true

// The release file built for this PC's processor.
func windowsAssetName() string {
	switch runtime.GOARCH {
	case "arm64":
		return "USCF-Portal-Print-Agent-ARM64.exe"
	case "386":
		return "USCF-Portal-Print-Agent-32bit.exe"
	default:
		return "USCF-Portal-Print-Agent.exe"
	}
}

func startAutoUpdate(version string) {
	if _, ok := semverParts(version); !ok {
		return // a "dev" build never updates itself
	}
	go func() {
		time.Sleep(updateFirstWait)
		for {
			if err := updateOnce(version); err != nil {
				log.Printf("update: %v", err)
			}
			time.Sleep(updateEvery)
		}
	}()
}

func updateOnce(version string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	rel, err := fetchLatestRelease(ctx)
	if err != nil {
		return err
	}
	if !isNewer(rel.Tag, version) {
		return nil
	}
	name := windowsAssetName()
	exeURL, sumsURL := rel.Assets[name], rel.Assets[checksumsAsset]
	if exeURL == "" || sumsURL == "" {
		return fmt.Errorf("release %s has no %s or %s; not updating", rel.Tag, name, checksumsAsset)
	}
	sumsData, err := download(ctx, sumsURL, 64<<10)
	if err != nil {
		return err
	}
	want := parseChecksums(bytes.NewReader(sumsData))[name]
	if want == "" {
		return fmt.Errorf("%s is not listed in %s; not updating", name, checksumsAsset)
	}
	exe, err := download(ctx, exeURL, 64<<20)
	if err != nil {
		return err
	}
	got := sha256.Sum256(exe)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("%s does not match its checksum; not updating", name)
	}
	dir := filepath.Join(os.TempDir(), "uscf-print-agent")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, fmt.Sprintf("update-%s.exe", rel.Tag))
	if err := os.WriteFile(path, exe, 0o700); err != nil {
		return err
	}
	log.Printf("update: installing %s (was %s)", rel.Tag, version)
	// The new copy installs itself into Programs, stops this one and starts.
	return exec.Command(path).Start()
}
