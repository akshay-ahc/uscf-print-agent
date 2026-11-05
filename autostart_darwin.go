//go:build darwin

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// macOS: a per-user LaunchAgent starts the agent at login and restarts it after
// a crash. A clean exit (Quit in the menu bar) stays quit until the next login.
const launchAgentLabel = "com.uscompoundingfactory.print-agent"

func launchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, launchAgentLabel+".plist"), nil
}

func installAutostart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	path, err := launchAgentPath()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(launchAgentPlist(exe)), 0o644)
}

func launchAgentPlist(exe string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key><array><string>%s</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
  <key>ProcessType</key><string>Interactive</string>
</dict>
</plist>
`, launchAgentLabel, exe)
}

func removeAutostart() error {
	path, err := launchAgentPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
