//go:build !windows

package main

// macOS needs an administrator password to install a package, so the agent
// cannot update itself there: the station asks for the update instead.
const autoUpdateSupported = false

func startAutoUpdate(string) {}
