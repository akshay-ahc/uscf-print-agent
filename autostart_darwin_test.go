//go:build darwin

package main

import (
	"strings"
	"testing"
)

// Quit in the menu bar must stay quit: macOS restarts the agent only after a crash.
func TestLaunchAgentRestartsOnlyAfterACrash(t *testing.T) {
	plist := launchAgentPlist("/usr/local/bin/uscf-print-agent")
	if !strings.Contains(plist, "<key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>") {
		t.Errorf("KeepAlive must only cover unclean exits:\n%s", plist)
	}
	if !strings.Contains(plist, "<key>RunAtLoad</key><true/>") {
		t.Errorf("the agent must still start at login:\n%s", plist)
	}
}
